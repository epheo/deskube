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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	"github.com/deskube/deskube/internal/guest"
	"github.com/deskube/deskube/internal/sunshine"
)

const (
	ConditionVMReady        = "VMReady"
	ConditionNetworkReady   = "NetworkReady"
	ConditionStreamingReady = "StreamingReady"
	ConditionReady          = "Ready"

	labelInstance = "vdi.desku.be/instance"
	adminUser     = "admin"

	requeueStarting = 10 * time.Second
	requeueReady    = 30 * time.Second
)

// ProbeFunc reads Sunshine serverinfo; tests replace it.
type ProbeFunc func(ctx context.Context, host string, basePort int32) (*sunshine.ServerInfo, error)

// VdiInstanceReconciler turns a VdiInstance into a VM, a Service, and
// the secrets the guest and the pairing controller need.
type VdiInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// ProbeExternal probes through the client address instead of the
	// ClusterIP, for a manager running outside the cluster.
	ProbeExternal bool
	Probe         ProbeFunc
}

// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile converges the children, then reports how to connect.
func (r *VdiInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	vdi := &vdiv1alpha1.VdiInstance{}
	if err := r.Get(ctx, req.NamespacedName, vdi); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Children carry owner references, garbage collection removes them.
	if !vdi.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if vdi.Status.BasePort == 0 {
		base, err := r.chooseBasePort(ctx, vdi)
		if err != nil {
			return r.fail(ctx, vdi, ConditionNetworkReady, "PortAllocation", err)
		}
		vdi.Status.BasePort = base
		vdi.Status.Phase = vdiv1alpha1.PhaseProvisioning
		// Persist before creating children so a retry reuses the same window.
		if err := r.Status().Update(ctx, vdi); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("allocated base port", "basePort", base)
	}

	password, err := r.ensureAdminSecret(ctx, vdi)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureCloudInitSecret(ctx, vdi, password); err != nil {
		return ctrl.Result{}, err
	}
	vm, err := r.ensureVM(ctx, vdi)
	if err != nil {
		return r.fail(ctx, vdi, ConditionVMReady, "VMApply", err)
	}
	svc, err := r.ensureService(ctx, vdi)
	if err != nil {
		return r.fail(ctx, vdi, ConditionNetworkReady, "ServiceApply", err)
	}
	if err := r.ensureOwnerRBAC(ctx, vdi); err != nil {
		return ctrl.Result{}, err
	}
	return r.updateStatus(ctx, vdi, vm, svc)
}

// chooseBasePort honours a pinned port, else takes the lowest free window.
func (r *VdiInstanceReconciler) chooseBasePort(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (int32, error) {
	if vdi.Spec.Expose.Type == vdiv1alpha1.ExposeLoadBalancer {
		if vdi.Spec.Expose.BasePort != 0 {
			return vdi.Spec.Expose.BasePort, nil
		}
		return loadBalancerBasePort, nil
	}
	if pinned := vdi.Spec.Expose.BasePort; pinned != 0 {
		if !nodePortBaseValid(pinned) {
			return 0, fmt.Errorf("basePort %d leaves the NodePort range %d-%d", pinned, nodePortMin, nodePortMax)
		}
		return pinned, nil
	}
	all := &vdiv1alpha1.VdiInstanceList{}
	if err := r.List(ctx, all); err != nil {
		return 0, err
	}
	used := map[int32]bool{}
	for i := range all.Items {
		other := &all.Items[i]
		if other.UID == vdi.UID {
			continue
		}
		if other.Status.BasePort != 0 {
			used[other.Status.BasePort] = true
		}
		if other.Spec.Expose.BasePort != 0 {
			used[other.Spec.Expose.BasePort] = true
		}
	}
	return allocateNodePortBase(used)
}

func (r *VdiInstanceReconciler) ensureAdminSecret(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (string, error) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: adminSecretName(vdi), Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		if len(secret.Data["password"]) == 0 {
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				return err
			}
			secret.Data["password"] = []byte(hex.EncodeToString(raw))
		}
		secret.Data["username"] = []byte(adminUser)
		return controllerutil.SetControllerReference(vdi, secret, r.Scheme)
	})
	if err != nil {
		return "", err
	}
	return string(secret.Data["password"]), nil
}

func (r *VdiInstanceReconciler) ensureCloudInitSecret(ctx context.Context, vdi *vdiv1alpha1.VdiInstance, password string) error {
	userData, err := guest.CloudInit(guest.Params{
		Name:              vdi.Name,
		BasePort:          vdi.Status.BasePort,
		AdminUser:         adminUser,
		AdminPassword:     password,
		SSHAuthorizedKeys: vdi.Spec.SSHAuthorizedKeys,
		Encoder:           vdi.Spec.Stream.Encoder,
		Capture:           vdi.Spec.Stream.Capture,
	})
	if err != nil {
		return err
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cloudInitSecretName(vdi), Namespace: vdi.Namespace}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Data = map[string][]byte{"userdata": []byte(userData)}
		return controllerutil.SetControllerReference(vdi, secret, r.Scheme)
	})
	return err
}

func (r *VdiInstanceReconciler) ensureVM(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (*kubevirtv1.VirtualMachine, error) {
	vm := &kubevirtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: vdi.Name, Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, vm, func() error {
		strategy := kubevirtv1.RunStrategyAlways
		if vdi.Spec.Running != nil && !*vdi.Spec.Running {
			strategy = kubevirtv1.RunStrategyHalted
		}
		// Only the power state follows spec changes. The disk and the
		// cloud-init are read once at first boot, rewriting them is churn.
		if vm.UID != "" {
			vm.Spec.RunStrategy = &strategy
			return nil
		}
		vm.Labels = map[string]string{labelInstance: vdi.Name}
		vm.Spec = desiredVMSpec(vdi, strategy)
		return controllerutil.SetControllerReference(vdi, vm, r.Scheme)
	})
	return vm, err
}

func desiredVMSpec(vdi *vdiv1alpha1.VdiInstance, strategy kubevirtv1.VirtualMachineRunStrategy) kubevirtv1.VirtualMachineSpec {
	instanceType := vdi.Spec.InstanceType
	if instanceType == "" {
		instanceType = "u1.xlarge"
	}
	preference := vdi.Spec.Preference
	if preference == "" {
		preference = "fedora"
	}
	size := vdi.Spec.Image.Size
	if size.IsZero() {
		size = resource.MustParse("30Gi")
	}
	imageNS := vdi.Spec.Image.DataSource.Namespace
	if imageNS == "" {
		imageNS = vdi.Namespace
	}
	rootDV := vdi.Name + "-root"
	return kubevirtv1.VirtualMachineSpec{
		RunStrategy:  &strategy,
		Instancetype: &kubevirtv1.InstancetypeMatcher{Kind: "VirtualMachineClusterInstancetype", Name: instanceType},
		Preference:   &kubevirtv1.PreferenceMatcher{Kind: "VirtualMachineClusterPreference", Name: preference},
		DataVolumeTemplates: []kubevirtv1.DataVolumeTemplateSpec{{
			ObjectMeta: metav1.ObjectMeta{
				Name: rootDV,
				// A thin snapshot clones in seconds where a copy takes minutes.
				Annotations: map[string]string{"cdi.kubevirt.io/cloneType": "snapshot"},
			},
			Spec: cdiv1beta1.DataVolumeSpec{
				SourceRef: &cdiv1beta1.DataVolumeSourceRef{
					Kind:      "DataSource",
					Name:      vdi.Spec.Image.DataSource.Name,
					Namespace: &imageNS,
				},
				Storage: &cdiv1beta1.StorageSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: size},
					},
				},
			},
		}},
		Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{labelInstance: vdi.Name}},
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				Domain: kubevirtv1.DomainSpec{
					Devices: kubevirtv1.Devices{
						Disks: []kubevirtv1.Disk{
							{Name: "rootdisk", DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: kubevirtv1.DiskBusVirtio}}},
							{Name: "cloudinitdisk", DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: kubevirtv1.DiskBusVirtio}}},
						},
						// Masquerade with no port list forwards every port, UDP included.
						Interfaces: []kubevirtv1.Interface{{
							Name:                   "default",
							InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{Masquerade: &kubevirtv1.InterfaceMasquerade{}},
						}},
					},
				},
				Networks: []kubevirtv1.Network{{Name: "default", NetworkSource: kubevirtv1.NetworkSource{Pod: &kubevirtv1.PodNetwork{}}}},
				Volumes: []kubevirtv1.Volume{
					{Name: "rootdisk", VolumeSource: kubevirtv1.VolumeSource{DataVolume: &kubevirtv1.DataVolumeSource{Name: rootDV}}},
					{Name: "cloudinitdisk", VolumeSource: kubevirtv1.VolumeSource{CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
						UserDataSecretRef: &corev1.LocalObjectReference{Name: cloudInitSecretName(vdi)},
					}}},
				},
			},
		},
	}
}

func (r *VdiInstanceReconciler) ensureService(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (*corev1.Service, error) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: vdi.Name, Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = map[string]string{labelInstance: vdi.Name}
		svc.Spec.Selector = map[string]string{labelInstance: vdi.Name}
		svc.Spec.Type = corev1.ServiceTypeNodePort
		if vdi.Spec.Expose.Type == vdiv1alpha1.ExposeLoadBalancer {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		}
		svc.Spec.Ports = servicePorts(vdi.Status.BasePort, svc.Spec.Type == corev1.ServiceTypeNodePort)
		return controllerutil.SetControllerReference(vdi, svc, r.Scheme)
	})
	return svc, err
}

// servicePorts maps each Sunshine port 1:1, so the guest and the
// outside agree on the numbers RTSP advertises.
func servicePorts(base int32, pinNodePorts bool) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(sunshine.Ports))
	for _, p := range sunshine.Ports {
		n := base + p.Offset
		sp := corev1.ServicePort{
			Name:       p.Name,
			Protocol:   corev1.Protocol(p.Protocol),
			Port:       n,
			TargetPort: intstr.FromInt32(n),
		}
		if pinNodePorts {
			sp.NodePort = n
		}
		ports = append(ports, sp)
	}
	return ports
}

// ensureOwnerRBAC lets the owner read the instance, pair clients, and
// read the admin credentials for the Sunshine web UI.
func (r *VdiInstanceReconciler) ensureOwnerRBAC(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	name := vdi.Name + "-owner"
	if vdi.Spec.Owner == "" {
		for _, obj := range []client.Object{
			&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vdi.Namespace}},
			&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vdi.Namespace}},
		} {
			if err := r.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
		return nil
	}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vdi.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{APIGroups: []string{vdiv1alpha1.GroupVersion.Group}, Resources: []string{"vdiinstances"}, ResourceNames: []string{vdi.Name}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{vdiv1alpha1.GroupVersion.Group}, Resources: []string{"vdipairings"}, Verbs: []string{"get", "list", "watch", "create", "delete"}},
			{APIGroups: []string{""}, Resources: []string{"secrets"}, ResourceNames: []string{adminSecretName(vdi)}, Verbs: []string{"get"}},
		}
		return controllerutil.SetControllerReference(vdi, role, r.Scheme)
	}); err != nil {
		return err
	}
	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		binding.Subjects = []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: vdi.Spec.Owner}}
		binding.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name}
		return controllerutil.SetControllerReference(vdi, binding, r.Scheme)
	})
	return err
}

func (r *VdiInstanceReconciler) updateStatus(ctx context.Context, vdi *vdiv1alpha1.VdiInstance, vm *kubevirtv1.VirtualMachine, svc *corev1.Service) (ctrl.Result, error) {
	running := vdi.Spec.Running == nil || *vdi.Spec.Running
	vdi.Status.AdminSecret = adminSecretName(vdi)
	vdi.Status.ObservedGeneration = vdi.Generation

	vmReady := vm.Status.Ready
	setCondition(vdi, ConditionVMReady, vmReady, string(vm.Status.PrintableStatus), "")

	host, err := r.clientHost(ctx, vdi, svc)
	if host != "" {
		vdi.Status.Address = net.JoinHostPort(host, strconv.Itoa(int(vdi.Status.BasePort)))
	}
	setCondition(vdi, ConditionNetworkReady, err == nil && host != "", "Address", errString(err))

	streaming := false
	vdi.Status.SessionState = ""
	if vmReady && host != "" {
		probeHost := svc.Spec.ClusterIP
		if r.ProbeExternal || probeHost == "" || probeHost == corev1.ClusterIPNone {
			probeHost = host
		}
		probe := r.Probe
		if probe == nil {
			probe = sunshine.GetServerInfo
		}
		info, perr := probe(ctx, probeHost, vdi.Status.BasePort)
		if perr == nil && info.StatusCode == 200 {
			streaming = true
			vdi.Status.SessionState = info.SessionState()
		}
		setCondition(vdi, ConditionStreamingReady, streaming, "ServerInfo", errString(perr))
	} else {
		setCondition(vdi, ConditionStreamingReady, false, "VMNotReady", "")
	}

	ready := vmReady && streaming && host != ""
	setCondition(vdi, ConditionReady, ready, "Aggregate", "")
	vdi.Status.Phase = phase(running, ready, vm)

	if err := r.Status().Update(ctx, vdi); err != nil {
		return ctrl.Result{}, err
	}
	switch {
	case !running:
		return ctrl.Result{}, nil
	case ready:
		return ctrl.Result{RequeueAfter: requeueReady}, nil
	default:
		return ctrl.Result{RequeueAfter: requeueStarting}, nil
	}
}

func phase(running, ready bool, vm *kubevirtv1.VirtualMachine) vdiv1alpha1.VdiInstancePhase {
	status := string(vm.Status.PrintableStatus)
	switch {
	case ready:
		return vdiv1alpha1.PhaseReady
	case !running && !vm.Status.Ready:
		return vdiv1alpha1.PhaseStopped
	case strings.HasPrefix(status, "Error") || status == string(kubevirtv1.VirtualMachineStatusCrashLoopBackOff):
		return vdiv1alpha1.PhaseFailed
	case status == string(kubevirtv1.VirtualMachineStatusProvisioning) ||
		status == string(kubevirtv1.VirtualMachineStatusWaitingForVolumeBinding) ||
		status == "":
		return vdiv1alpha1.PhaseProvisioning
	default:
		return vdiv1alpha1.PhaseStarting
	}
}

// clientHost is the address a Moonlight client dials.
func (r *VdiInstanceReconciler) clientHost(ctx context.Context, vdi *vdiv1alpha1.VdiInstance, svc *corev1.Service) (string, error) {
	if vdi.Spec.Expose.Host != "" {
		return vdi.Spec.Expose.Host, nil
	}
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				return ing.IP, nil
			}
			if ing.Hostname != "" {
				return ing.Hostname, nil
			}
		}
		return "", fmt.Errorf("load balancer address not assigned yet")
	}
	// NodePorts answer on every node, the one running the VM has the
	// shortest path.
	nodeName := ""
	vmi := &kubevirtv1.VirtualMachineInstance{}
	if err := r.Get(ctx, types.NamespacedName{Name: vdi.Name, Namespace: vdi.Namespace}, vmi); err == nil {
		nodeName = vmi.Status.NodeName
	}
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		return "", err
	}
	var fallback string
	for i := range nodes.Items {
		addr := nodeAddress(&nodes.Items[i])
		if addr == "" {
			continue
		}
		if nodes.Items[i].Name == nodeName {
			return addr, nil
		}
		if fallback == "" {
			fallback = addr
		}
	}
	if fallback == "" {
		return "", fmt.Errorf("no node address found")
	}
	return fallback, nil
}

func nodeAddress(node *corev1.Node) string {
	for _, t := range []corev1.NodeAddressType{corev1.NodeExternalIP, corev1.NodeInternalIP} {
		for _, a := range node.Status.Addresses {
			if a.Type == t && a.Address != "" {
				return a.Address
			}
		}
	}
	return ""
}

// fail records the error on a condition and asks for a retry.
func (r *VdiInstanceReconciler) fail(ctx context.Context, vdi *vdiv1alpha1.VdiInstance, condition, reason string, cause error) (ctrl.Result, error) {
	setCondition(vdi, condition, false, reason, cause.Error())
	setCondition(vdi, ConditionReady, false, reason, "")
	vdi.Status.Phase = vdiv1alpha1.PhaseFailed
	if err := r.Status().Update(ctx, vdi); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, cause
}

func setCondition(vdi *vdiv1alpha1.VdiInstance, conditionType string, ok bool, reason, message string) {
	status := metav1.ConditionFalse
	if ok {
		status = metav1.ConditionTrue
	}
	if reason == "" {
		reason = "Unknown"
	}
	meta.SetStatusCondition(&vdi.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             sanitizeReason(reason),
		Message:            message,
		ObservedGeneration: vdi.Generation,
	})
}

// sanitizeReason keeps KubeVirt printable statuses valid as condition reasons.
func sanitizeReason(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	if b.Len() == 0 {
		return "Unknown"
	}
	return b.String()
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func adminSecretName(vdi *vdiv1alpha1.VdiInstance) string     { return vdi.Name + "-sunshine-admin" }
func cloudInitSecretName(vdi *vdiv1alpha1.VdiInstance) string { return vdi.Name + "-cloudinit" }

// SetupWithManager registers the controller and its owned resources.
func (r *VdiInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vdiv1alpha1.VdiInstance{}).
		Owns(&kubevirtv1.VirtualMachine{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Named("vdiinstance").
		Complete(r)
}
