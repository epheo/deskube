/*
Copyright 2025 Thibaut Lapierre.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// VDI instance phases
const (
	PhasePending   = "Pending"
	PhaseRunning   = "Running"
	PhaseFailed    = "Failed"
	PhaseSucceeded = "Succeeded"
	PhaseUnknown   = "Unknown"
)

// Default values for Sunshine and Moonlight
const (
	DefaultSunshineImage  = "lizardbyte/sunshine:latest"
	DefaultMoonlightImage = "lizardbyte/moonlight:latest"
	DefaultStreamingPort  = 47989
	DefaultWebAdminPort   = 47990
	DefaultFPS            = 60
	DefaultBitrate        = 50 // Mbps
	DefaultResolution     = "1920x1080"
	DefaultServiceType    = "ClusterIP"
	DefaultExposureMethod = "Route"
)

// VdiInstanceReconciler reconciles a VdiInstance object
type VdiInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vditemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vditemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims;pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines;virtualmachineinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VdiInstance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *VdiInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling VdiInstance", "namespace", req.Namespace, "name", req.Name)

	// Fetch the VdiInstance
	vdi := &vdiv1alpha1.VdiInstance{}
	if err := r.Get(ctx, req.NamespacedName, vdi); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			logger.Info("VdiInstance resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get VdiInstance")
		return ctrl.Result{}, err
	}

	// Initialize status if it's a new resource
	if vdi.Status.Phase == "" {
		vdi.Status.Phase = PhasePending
		if err := r.Status().Update(ctx, vdi); err != nil {
			logger.Error(err, "Failed to update VdiInstance status")
			return ctrl.Result{}, err
		}
	}

	// If this is a template-based provisioning, process the template first
	if vdi.Spec.ProvisionSource == vdiv1alpha1.ProvisionFromTemplate && vdi.Spec.TemplateRef != nil {
		if err := r.processTemplateBasedInstance(ctx, vdi); err != nil {
			logger.Error(err, "Failed to process template-based instance")
			vdi.Status.Phase = PhaseFailed
			if updateErr := r.Status().Update(ctx, vdi); updateErr != nil {
				logger.Error(updateErr, "Failed to update VdiInstance status after template processing error")
			}
			return ctrl.Result{}, err
		}

		// Requeue to handle the updated instance with populated values from template
		logger.Info("Template successfully applied to instance, requeuing")
		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer for cleanup if not present
	if !controllerutil.ContainsFinalizer(vdi, "vdi.desku.be/finalizer") {
		controllerutil.AddFinalizer(vdi, "vdi.desku.be/finalizer")
		if err := r.Update(ctx, vdi); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Handle deletion if needed
	if !vdi.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, vdi)
	}

	// Check KubeVirt VM status
	vmStatus, err := r.checkKubeVirtVM(ctx, vdi)
	if err != nil {
		vdi.Status.Phase = PhaseFailed
		vdi.Status.VMStatus = fmt.Sprintf("Error checking VM: %v", err)
		if updateErr := r.Status().Update(ctx, vdi); updateErr != nil {
			logger.Error(updateErr, "Failed to update VdiInstance status after VM check error")
		}
		return ctrl.Result{}, err
	}
	vdi.Status.VMStatus = vmStatus

	// Set up Sunshine components
	if err := r.reconcileSunshineComponents(ctx, vdi); err != nil {
		logger.Error(err, "Failed to reconcile Sunshine components")
		vdi.Status.Phase = PhaseFailed
		if updateErr := r.Status().Update(ctx, vdi); updateErr != nil {
			logger.Error(updateErr, "Failed to update VdiInstance status after Sunshine reconcile error")
		}
		return ctrl.Result{}, err
	}

	// Set up networking
	if err := r.reconcileNetworking(ctx, vdi); err != nil {
		logger.Error(err, "Failed to reconcile networking")
		return ctrl.Result{}, err
	}

	// Set up RBAC for access control
	if err := r.reconcileRBAC(ctx, vdi); err != nil {
		logger.Error(err, "Failed to reconcile RBAC for VDI instance")
		vdi.Status.Phase = PhaseFailed
		if updateErr := r.Status().Update(ctx, vdi); updateErr != nil {
			logger.Error(updateErr, "Failed to update VdiInstance status after RBAC reconcile error")
		}
		return ctrl.Result{}, err
	}

	// Set up RBAC
	if err := r.reconcileRBAC(ctx, vdi); err != nil {
		logger.Error(err, "Failed to reconcile RBAC")
		return ctrl.Result{}, err
	}

	// Update status
	vdi.Status.Phase = PhaseRunning
	if err := r.Status().Update(ctx, vdi); err != nil {
		logger.Error(err, "Failed to update VdiInstance status")
		return ctrl.Result{}, err
	}

	logger.Info("VdiInstance reconciliation completed successfully")
	// Requeue after 1 minute to check status periodically
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VdiInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vdiv1alpha1.VdiInstance{}).
		Complete(r)
}

// handleDeletion handles the cleanup of resources when a VdiInstance is deleted
func (r *VdiInstanceReconciler) handleDeletion(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up resources for deleted VdiInstance", "namespace", vdi.Namespace, "name", vdi.Name)

	// Check if the finalizer is still present
	if controllerutil.ContainsFinalizer(vdi, "vdi.desku.be/finalizer") {
		// Clean up Sunshine resources
		if err := r.cleanupSunshineResources(ctx, vdi); err != nil {
			logger.Error(err, "Failed to clean up Sunshine resources")
			return ctrl.Result{}, err
		}

		// Clean up networking resources
		if err := r.cleanupNetworkingResources(ctx, vdi); err != nil {
			logger.Error(err, "Failed to clean up networking resources")
			return ctrl.Result{}, err
		}

		// Clean up RBAC resources
		if err := r.cleanupRBACResources(ctx, vdi); err != nil {
			logger.Error(err, "Failed to clean up RBAC resources")
			return ctrl.Result{}, err
		}

		// Remove our finalizer
		controllerutil.RemoveFinalizer(vdi, "vdi.desku.be/finalizer")
		if err := r.Update(ctx, vdi); err != nil {
			logger.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	logger.Info("Successfully cleaned up all resources")
	return ctrl.Result{}, nil
}

// checkKubeVirtVM verifies the KubeVirt VM exists and returns its status
func (r *VdiInstanceReconciler) checkKubeVirtVM(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (string, error) {
	logger := log.FromContext(ctx)

	// Check if KubeVirtVMRef is provided
	if vdi.Spec.KubeVirtVMRef == nil {
		return "", fmt.Errorf("KubeVirtVMRef is not specified in VDI instance")
	}

	// Determine the namespace to look for the VM
	vmNamespace := vdi.Spec.KubeVirtVMRef.Namespace
	if vmNamespace == "" {
		vmNamespace = vdi.Namespace
	}

	vmName := vdi.Spec.KubeVirtVMRef.Name
	if vmName == "" {
		return "", fmt.Errorf("VM name is empty in KubeVirtVMRef")
	}

	// Get the KubeVirt VM using the explicit API type
	vm := &kubevirtv1.VirtualMachine{}
	vmKey := types.NamespacedName{Name: vmName, Namespace: vmNamespace}

	if err := r.Get(ctx, vmKey, vm); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("KubeVirt VM not found", "vmName", vmName, "vmNamespace", vmNamespace)
			return "NotFound", err
		}
		logger.Error(err, "Failed to get KubeVirt VM", "vmName", vmName, "vmNamespace", vmNamespace)
		return "Error", err
	}

	// Check VM status - more comprehensive status check
	logger.Info("Found KubeVirt VM", "name", vm.Name, "status", vm.Status.PrintableStatus)

	// Check if VM is running based on the conditions and printable status
	if vm.Status.Ready {
		return "Ready", nil
	}

	if vm.Status.PrintableStatus == "Running" || vm.Status.PrintableStatus == "Starting" {
		return "Starting", nil
	}

	if vm.Status.PrintableStatus == "Stopped" || vm.Status.PrintableStatus == "Stopping" {
		return "Stopped", nil
	}

	return string(vm.Status.PrintableStatus), nil
}

// reconcileSunshineComponents creates or updates the Sunshine streaming components
func (r *VdiInstanceReconciler) reconcileSunshineComponents(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)
	logger.Info("Setting up Sunshine components", "namespace", vdi.Namespace, "name", vdi.Name)

	// Create Sunshine deployment
	if err := r.reconcileSunshineDeployment(ctx, vdi); err != nil {
		return fmt.Errorf("failed to reconcile Sunshine deployment: %w", err)
	}

	// Create Sunshine configuration ConfigMap
	if err := r.reconcileSunshineConfigMap(ctx, vdi); err != nil {
		return fmt.Errorf("failed to reconcile Sunshine ConfigMap: %w", err)
	}

	// Update Sunshine status
	sunshineStatus := vdiv1alpha1.ComponentStatus{
		Ready:   true,
		PodName: fmt.Sprintf("%s-sunshine", vdi.Name),
		Message: "Sunshine components created successfully",
	}

	vdi.Status.SunshineStatus = sunshineStatus

	logger.Info("Sunshine components set up successfully")
	return nil
}

// reconcileSunshineDeployment creates or updates the Sunshine deployment
func (r *VdiInstanceReconciler) reconcileSunshineDeployment(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)

	// Determine image to use
	image := DefaultSunshineImage
	if vdi.Spec.SunshineConfig.Image != "" {
		image = vdi.Spec.SunshineConfig.Image
	}

	// Create deployment spec
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sunshine", vdi.Name),
			Namespace: vdi.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("%s-sunshine", vdi.Name),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": fmt.Sprintf("%s-sunshine", vdi.Name),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "sunshine",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "streaming",
									ContainerPort: DefaultStreamingPort,
								},
								{
									Name:          "webadmin",
									ContainerPort: DefaultWebAdminPort,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "SUNSHINE_FPS",
									Value: fmt.Sprintf("%d", getOrDefaultFPS(vdi)),
								},
								{
									Name:  "SUNSHINE_BITRATE",
									Value: fmt.Sprintf("%d", getOrDefaultBitrate(vdi)),
								},
								{
									Name:  "SUNSHINE_RESOLUTION",
									Value: getOrDefaultResolution(vdi),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sunshine-config",
									MountPath: "/etc/sunshine",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sunshine-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-sunshine-config", vdi.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference so deployment will be deleted when VDI instance is deleted
	if err := controllerutil.SetControllerReference(vdi, deployment, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on Sunshine deployment")
		return err
	}

	// Create or update the deployment
	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating Sunshine deployment", "name", deployment.Name)
		return r.Create(ctx, deployment)
	} else if err != nil {
		logger.Error(err, "Failed to get existing Sunshine deployment")
		return err
	}

	// Update existing deployment
	existingDeployment.Spec = deployment.Spec
	logger.Info("Updating Sunshine deployment", "name", deployment.Name)
	return r.Update(ctx, existingDeployment)
}

// reconcileSunshineConfigMap creates or updates the Sunshine configuration ConfigMap
func (r *VdiInstanceReconciler) reconcileSunshineConfigMap(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)

	// Create basic Sunshine configuration
	configData := map[string]string{
		"sunshine.conf": generateSunshineConfig(vdi),
	}

	// Create ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sunshine-config", vdi.Name),
			Namespace: vdi.Namespace,
		},
		Data: configData,
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(vdi, cm, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on Sunshine ConfigMap")
		return err
	}

	// Create or update the ConfigMap
	existingCM := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existingCM)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating Sunshine ConfigMap", "name", cm.Name)
		return r.Create(ctx, cm)
	} else if err != nil {
		logger.Error(err, "Failed to get existing Sunshine ConfigMap")
		return err
	}

	// Update existing ConfigMap
	existingCM.Data = cm.Data
	logger.Info("Updating Sunshine ConfigMap", "name", cm.Name)
	return r.Update(ctx, existingCM)
}

// reconcileNetworking creates or updates the networking resources for VDI access
func (r *VdiInstanceReconciler) reconcileNetworking(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)
	logger.Info("Setting up networking for VDI instance", "namespace", vdi.Namespace, "name", vdi.Name)

	// Create service for Sunshine streaming server
	if err := r.reconcileService(ctx, vdi); err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}

	// Create route or ingress based on the exposure method
	exposureMethod := getExposureMethod(vdi)
	switch exposureMethod {
	case "Route":
		// This is specific to OpenShift
		logger.Info("Using OpenShift Route for VDI exposure")
		// In a full implementation, this would create an OpenShift Route
		// Since we're in a basic implementation, we'll skip this step
		// but update the connection info as if we did it
		baseURL := fmt.Sprintf("%s-vdi.apps.example.com", vdi.Name)
		vdi.Status.ConnectionDetails.AccessURL = "https://" + baseURL
		vdi.Status.ConnectionDetails.Port = DefaultStreamingPort
	case "Ingress":
		logger.Info("Using Kubernetes Ingress for VDI exposure")
		if err := r.reconcileIngress(ctx, vdi); err != nil {
			return fmt.Errorf("failed to reconcile ingress: %w", err)
		}
	default:
		// Just use the service
		logger.Info("Using direct service access for VDI")
		vdi.Status.ConnectionDetails.AccessURL = fmt.Sprintf("%s-sunshine.%s.svc", vdi.Name, vdi.Namespace)
		vdi.Status.ConnectionDetails.Port = DefaultStreamingPort
	}

	// Update connection parameters
	vdi.Status.ConnectionDetails.ConnectionParams = map[string]string{
		"protocol": "sunshine",
		"username": "sunshine",
	}

	return nil
}

// reconcileService creates or updates the Kubernetes Service for VDI access
func (r *VdiInstanceReconciler) reconcileService(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)

	// Determine service type
	serviceType := getServiceType(vdi)

	// Create service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sunshine", vdi.Name),
			Namespace: vdi.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceType(serviceType),
			Ports: []corev1.ServicePort{
				{
					Name:       "streaming",
					Port:       DefaultStreamingPort,
					TargetPort: intstr.FromInt(DefaultStreamingPort),
				},
				{
					Name:       "webadmin",
					Port:       DefaultWebAdminPort,
					TargetPort: intstr.FromInt(DefaultWebAdminPort),
				},
			},
			Selector: map[string]string{
				"app": fmt.Sprintf("%s-sunshine", vdi.Name),
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(vdi, service, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on service")
		return err
	}

	// Create or update the service
	existingService := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, existingService)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		logger.Error(err, "Failed to get existing service")
		return err
	}

	// Update existing service
	existingService.Spec.Ports = service.Spec.Ports
	existingService.Spec.Selector = service.Spec.Selector
	// Don't change the service type as it might cause issues
	logger.Info("Updating service", "name", service.Name)
	return r.Update(ctx, existingService)
}

// reconcileIngress creates or updates the Kubernetes Ingress for VDI access
func (r *VdiInstanceReconciler) reconcileIngress(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)

	// Determine hostname
	hostname := getHostname(vdi)

	// Default backend path for streaming
	pathType := networkingv1.PathTypeImplementationSpecific

	// Create ingress
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sunshine", vdi.Name),
			Namespace: vdi.Namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: hostname,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: fmt.Sprintf("%s-sunshine", vdi.Name),
											Port: networkingv1.ServiceBackendPort{
												Number: DefaultStreamingPort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(vdi, ingress, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on ingress")
		return err
	}

	// Create or update the ingress
	existingIngress := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, existingIngress)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating ingress", "name", ingress.Name)
		return r.Create(ctx, ingress)
	} else if err != nil {
		logger.Error(err, "Failed to get existing ingress")
		return err
	}

	// Update existing ingress
	existingIngress.Spec = ingress.Spec
	logger.Info("Updating ingress", "name", ingress.Name)
	return r.Update(ctx, existingIngress)
}

// Helper functions for Sunshine configuration

// getOrDefaultFPS returns the configured FPS or the default value
func getOrDefaultFPS(vdi *vdiv1alpha1.VdiInstance) int32 {
	if vdi.Spec.SunshineConfig.FPS != 0 {
		return vdi.Spec.SunshineConfig.FPS
	}
	return DefaultFPS
}

// getOrDefaultBitrate returns the configured bitrate or the default value
func getOrDefaultBitrate(vdi *vdiv1alpha1.VdiInstance) int32 {
	if vdi.Spec.SunshineConfig.Bitrate != 0 {
		return vdi.Spec.SunshineConfig.Bitrate
	}
	return DefaultBitrate
}

// getOrDefaultResolution returns the configured resolution or the default value
func getOrDefaultResolution(vdi *vdiv1alpha1.VdiInstance) string {
	if vdi.Spec.SunshineConfig.Resolution != "" {
		return vdi.Spec.SunshineConfig.Resolution
	}
	return DefaultResolution
}

// generateSunshineConfig generates the Sunshine configuration file content
func generateSunshineConfig(vdi *vdiv1alpha1.VdiInstance) string {
	// Basic configuration template
	config := fmt.Sprintf(`
# Sunshine configuration for %s
[streaming]
fps = %d
bitrate = %d
resolution = %s

[input]
gamepad = true

[host]
port = %d
`,
		vdi.Name,
		getOrDefaultFPS(vdi),
		getOrDefaultBitrate(vdi),
		getOrDefaultResolution(vdi),
		DefaultStreamingPort,
	)

	// Add any additional custom options
	if vdi.Spec.SunshineConfig.AdditionalOptions != nil {
		config += "\n# Additional custom options\n"
		for key, value := range vdi.Spec.SunshineConfig.AdditionalOptions {
			config += fmt.Sprintf("%s = %s\n", key, value)
		}
	}

	return config
}

// getExposureMethod returns the configured exposure method or the default
func getExposureMethod(vdi *vdiv1alpha1.VdiInstance) string {
	if vdi.Spec.NetworkConfig.ExposureMethod != "" {
		return vdi.Spec.NetworkConfig.ExposureMethod
	}
	return DefaultExposureMethod
}

// getServiceType returns the configured service type or the default
func getServiceType(vdi *vdiv1alpha1.VdiInstance) string {
	if vdi.Spec.NetworkConfig.ServiceType != "" {
		return vdi.Spec.NetworkConfig.ServiceType
	}
	return DefaultServiceType
}

// getHostname returns the configured hostname or a generated one
func getHostname(vdi *vdiv1alpha1.VdiInstance) string {
	if vdi.Spec.NetworkConfig.Hostname != "" {
		return vdi.Spec.NetworkConfig.Hostname
	}
	return fmt.Sprintf("%s-vdi.%s.example.com", vdi.Name, vdi.Namespace)
}

// cleanupSunshineResources removes all Sunshine-related resources
func (r *VdiInstanceReconciler) cleanupSunshineResources(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up Sunshine resources", "namespace", vdi.Namespace, "name", vdi.Name)

	// Delete deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sunshine", vdi.Name),
			Namespace: vdi.Namespace,
		},
	}
	if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete Sunshine deployment")
		return err
	}

	// Delete ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sunshine-config", vdi.Name),
			Namespace: vdi.Namespace,
		},
	}
	if err := r.Delete(ctx, cm); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete Sunshine ConfigMap")
		return err
	}

	logger.Info("Successfully cleaned up Sunshine resources")
	return nil
}

// cleanupNetworkingResources removes all networking-related resources
func (r *VdiInstanceReconciler) cleanupNetworkingResources(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up networking resources", "namespace", vdi.Namespace, "name", vdi.Name)

	// Delete service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sunshine", vdi.Name),
			Namespace: vdi.Namespace,
		},
	}
	if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete service")
		return err
	}

	// Delete ingress if exposure method is Ingress
	if getExposureMethod(vdi) == "Ingress" {
		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-sunshine", vdi.Name),
				Namespace: vdi.Namespace,
			},
		}
		if err := r.Delete(ctx, ingress); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete ingress")
			return err
		}
	}

	logger.Info("Successfully cleaned up networking resources")
	return nil
}

// reconcileRBAC creates or updates the RBAC resources for VDI instances
func (r *VdiInstanceReconciler) reconcileRBAC(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)

	// Skip RBAC setup if access control is not explicitly enabled
	if vdi.Spec.AccessControl == nil || !vdi.Spec.AccessControl.Enabled {
		logger.Info("RBAC integration not enabled, skipping RBAC reconciliation")
		return nil
	}

	logger.Info("Setting up RBAC for VDI instance", "namespace", vdi.Namespace, "name", vdi.Name)

	// Create default roles if needed
	if vdi.Spec.AccessControl.CreateRoles {
		if err := r.createVDIRoles(ctx, vdi); err != nil {
			logger.Error(err, "Failed to create VDI roles")
			return err
		}
	}

	// Create role bindings for each user access policy that has a role reference
	for _, access := range vdi.Spec.UserAccess {
		if access.RoleRef != nil {
			if err := r.createRoleBinding(ctx, vdi, access); err != nil {
				logger.Error(err, "Failed to create role binding", "userAccess", access)
				return err
			}
		}
	}

	return nil
}

// createVDIRoles creates standard roles for VDI access
func (r *VdiInstanceReconciler) createVDIRoles(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)

	// Determine role prefix
	rolePrefix := "vdi"
	if vdi.Spec.AccessControl.RolePrefix != "" {
		rolePrefix = vdi.Spec.AccessControl.RolePrefix
	}

	// Create viewer role
	viewerRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-viewer", rolePrefix, vdi.Name),
			Namespace: vdi.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"vdi.desku.be"},
				Resources: []string{"vdiinstances"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(vdi, viewerRole, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on viewer role")
		return err
	}

	// Create or update the viewer role
	if err := r.createOrUpdateRole(ctx, viewerRole); err != nil {
		return err
	}

	// Create user role
	userRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-user", rolePrefix, vdi.Name),
			Namespace: vdi.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"vdi.desku.be"},
				Resources: []string{"vdiinstances"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"vdi.desku.be"},
				Resources: []string{"vdiinstances/status"},
				Verbs:     []string{"get"},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(vdi, userRole, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on user role")
		return err
	}

	// Create or update the user role
	if err := r.createOrUpdateRole(ctx, userRole); err != nil {
		return err
	}

	// Create admin role
	adminRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-admin", rolePrefix, vdi.Name),
			Namespace: vdi.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"vdi.desku.be"},
				Resources: []string{"vdiinstances"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{"vdi.desku.be"},
				Resources: []string{"vdiinstances/status"},
				Verbs:     []string{"get"},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(vdi, adminRole, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on admin role")
		return err
	}

	// Create or update the admin role
	if err := r.createOrUpdateRole(ctx, adminRole); err != nil {
		return err
	}

	logger.Info("Successfully created standard VDI roles")
	return nil
}

// createOrUpdateRole creates or updates a Role
func (r *VdiInstanceReconciler) createOrUpdateRole(ctx context.Context, role *rbacv1.Role) error {
	logger := log.FromContext(ctx)

	// Check if role already exists
	existingRole := &rbacv1.Role{}
	err := r.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, existingRole)

	if err != nil && errors.IsNotFound(err) {
		// Role doesn't exist, create it
		logger.Info("Creating Role", "namespace", role.Namespace, "name", role.Name)
		return r.Create(ctx, role)
	} else if err != nil {
		// Error retrieving role
		logger.Error(err, "Failed to get Role")
		return err
	}

	// Role exists, update it
	existingRole.Rules = role.Rules
	logger.Info("Updating Role", "namespace", role.Namespace, "name", role.Name)
	return r.Update(ctx, existingRole)
}

// createRoleBinding creates or updates a RoleBinding for a user access policy
// createRoleBinding creates a RoleBinding for user access policy
func (r *VdiInstanceReconciler) createRoleBinding(
	ctx context.Context,
	vdi *vdiv1alpha1.VdiInstance,
	access vdiv1alpha1.UserAccessPolicy,
) error {
	logger := log.FromContext(ctx)

	// Determine subject type and name
	var subjectKind string
	var subjectName string

	if access.Type != "" {
		// Use the new fields
		switch access.Type {
		case "user":
			subjectKind = "User"
			subjectName = access.Name
		case "group":
			subjectKind = "Group"
			subjectName = access.Name
		case "serviceaccount":
			subjectKind = "ServiceAccount"
			subjectName = access.Name
		default:
			return fmt.Errorf("unknown subject type: %s", access.Type)
		}
	} else {
		// Fallback to legacy fields
		if access.Username != "" {
			subjectKind = "User"
			subjectName = access.Username
		} else if len(access.Groups) > 0 {
			// For backward compatibility, just use the first group
			subjectKind = "Group"
			subjectName = access.Groups[0]
		} else {
			return fmt.Errorf("user access policy does not specify a subject")
		}
	}

	// Determine role reference
	var roleRef rbacv1.RoleRef
	if access.RoleRef != nil {
		roleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     access.RoleRef.Kind,
			Name:     access.RoleRef.Name,
		}
	} else if vdi.Spec.AccessControl.DefaultRoleRef != nil {
		// Use default role reference
		roleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     vdi.Spec.AccessControl.DefaultRoleRef.Kind,
			Name:     vdi.Spec.AccessControl.DefaultRoleRef.Name,
		}
	} else {
		// Use the standard user role
		rolePrefix := "vdi"
		if vdi.Spec.AccessControl.RolePrefix != "" {
			rolePrefix = vdi.Spec.AccessControl.RolePrefix
		}
		roleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     fmt.Sprintf("%s-%s-user", rolePrefix, vdi.Name),
		}
	}

	// Create role binding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", roleRef.Name, subjectKind, subjectName),
			Namespace: vdi.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     subjectKind,
				APIGroup: "rbac.authorization.k8s.io",
				Name:     subjectName,
			},
		},
		RoleRef: roleRef,
	}

	// Handle ServiceAccount which doesn't have an APIGroup
	if subjectKind == "ServiceAccount" {
		roleBinding.Subjects[0].APIGroup = ""
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(vdi, roleBinding, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on role binding")
		return err
	}

	// Create or update the role binding
	existingRoleBinding := &rbacv1.RoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, existingRoleBinding)

	if err != nil && errors.IsNotFound(err) {
		// RoleBinding doesn't exist, create it
		logger.Info("Creating RoleBinding",
			"namespace", roleBinding.Namespace,
			"name", roleBinding.Name,
			"subject", fmt.Sprintf("%s:%s", subjectKind, subjectName),
			"role", fmt.Sprintf("%s:%s", roleRef.Kind, roleRef.Name))
		return r.Create(ctx, roleBinding)
	} else if err != nil {
		// Error retrieving role binding
		logger.Error(err, "Failed to get RoleBinding")
		return err
	}

	// RoleBinding exists, update it
	existingRoleBinding.Subjects = roleBinding.Subjects
	existingRoleBinding.RoleRef = roleBinding.RoleRef
	logger.Info("Updating RoleBinding",
		"namespace", roleBinding.Namespace,
		"name", roleBinding.Name,
		"subject", fmt.Sprintf("%s:%s", subjectKind, subjectName),
		"role", fmt.Sprintf("%s:%s", roleRef.Kind, roleRef.Name))
	return r.Update(ctx, existingRoleBinding)
}

// cleanupRBACResources removes RBAC resources created for a VDI instance
func (r *VdiInstanceReconciler) cleanupRBACResources(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)
	logger.Info("Cleaning up RBAC resources", "namespace", vdi.Namespace, "name", vdi.Name)

	// Skip if RBAC was never enabled for this VDI instance
	if vdi.Spec.AccessControl == nil || !vdi.Spec.AccessControl.Enabled {
		return nil
	}

	// List role bindings in the namespace
	roleBindingList := &rbacv1.RoleBindingList{}
	if err := r.List(ctx, roleBindingList, client.InNamespace(vdi.Namespace)); err != nil {
		logger.Error(err, "Failed to list role bindings")
		return err
	}

	// Delete role bindings owned by this VDI instance
	for i := range roleBindingList.Items {
		rb := &roleBindingList.Items[i]
		if isOwnedBy(rb, vdi) {
			if err := r.Delete(ctx, rb); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete role binding", "name", rb.Name)
				return err
			}
			logger.Info("Deleted role binding", "name", rb.Name)
		}
	}

	// List roles in the namespace
	roleList := &rbacv1.RoleList{}
	if err := r.List(ctx, roleList, client.InNamespace(vdi.Namespace)); err != nil {
		logger.Error(err, "Failed to list roles")
		return err
	}

	// Delete roles owned by this VDI instance
	for i := range roleList.Items {
		role := &roleList.Items[i]
		if isOwnedBy(role, vdi) {
			if err := r.Delete(ctx, role); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete role", "name", role.Name)
				return err
			}
			logger.Info("Deleted role", "name", role.Name)
		}
	}

	return nil
}

// isOwnedBy checks if an object is owned by a VDI instance
func isOwnedBy(obj metav1.Object, vdi *vdiv1alpha1.VdiInstance) bool {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.Kind == "VdiInstance" && owner.Name == vdi.Name && owner.UID == vdi.UID {
			return true
		}
	}
	return false
}
