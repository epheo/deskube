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
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

// Condition types on a VdiInstance. Ready aggregates the other three.
const (
	ConditionPodReady       = "PodReady"
	ConditionNetworkReady   = "NetworkReady"
	ConditionStreamingReady = "StreamingReady"
	ConditionReady          = "Ready"
	// ConditionSeccompFiltered is not part of Ready: it says whether the
	// desktop runs under a seccomp filter, and which.
	ConditionSeccompFiltered = "SeccompFiltered"

	labelInstance = "vdi.desku.be/instance"
	adminUser     = "admin"
	containerName = "desktop"
	// The image's user and home; an arbitrary UID gets the same home.
	homePath = "/home/desk"
	// What a sandbox desktop runs as inside its own user namespace.
	desktopUID = int64(1000)
	// OpenShift's ClusterRole that grants use of the nested-container SCC.
	nestedSCCRole = "system:openshift:scc:nested-container"

	requeueStarting = 10 * time.Second
	requeueReady    = 30 * time.Second

	// The manager's cache holds no object it made without this label.
	labelManagedBy = "app.kubernetes.io/managed-by"
	managedBy      = "deskube"
)

// ProbeFunc reads Sunshine serverinfo; tests replace it.
type ProbeFunc func(ctx context.Context, host string, basePort int32) (*sunshine.ServerInfo, error)

// VdiInstanceReconciler turns a VdiInstance into a desktop pod with a
// home volume and a Service, and pairs Moonlight clients with it.
type VdiInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// ProbeExternal probes through the client address instead of the
	// pod IP, for a manager running outside the cluster.
	ProbeExternal bool
	Probe         ProbeFunc
	// AdminClient builds the Sunshine admin API client; tests inject a fake.
	AdminClient NewPairingClient
	// APIReader reads without the cache: the manager may get one
	// ClusterRole by name, not list and watch them all, and port
	// allocation must see every Service in the cluster, fresh.
	APIReader client.Reader
	// SandboxSeccompProfile names a seccomp profile already on the
	// nodes, relative to the kubelet's seccomp directory, for sandbox
	// desktops. It overrides the profile the manager would otherwise
	// have the Security Profiles Operator install.
	SandboxSeccompProfile string
}

// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets;services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes;pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,resourceNames="system:openshift:scc:nested-container",verbs=get;bind
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
	result, err := r.reconcile(ctx, vdi)
	if isTransient(err) {
		// The cache lags one write behind; the next pass sees it.
		log.V(1).Info("retrying after a stale read", "cause", err.Error())
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}
	return result, err
}

func (r *VdiInstanceReconciler) reconcile(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if vdi.Status.BasePort == 0 {
		base, err := r.chooseBasePort(ctx, vdi)
		if err != nil {
			reason := "PortAllocation"
			if _, ok := errors.AsType[*portConflictError](err); ok {
				reason = "PortConflict"
			}
			return r.fail(ctx, vdi, ConditionNetworkReady, reason, err)
		}
		vdi.Status.BasePort = base
		vdi.Status.Phase = vdiv1alpha1.PhaseProvisioning
		// Persist before creating children so a retry reuses the same window.
		if err := r.Status().Update(ctx, vdi); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("allocated base port", "basePort", base)
	}

	// The Service claims the node ports before a pod starts on them.
	svc, err := r.ensureService(ctx, vdi)
	if err != nil {
		if svc.UID == "" && nodePortTaken(err) {
			return r.releaseWindow(ctx, vdi, err)
		}
		return r.fail(ctx, vdi, ConditionNetworkReady, "ServiceApply", err)
	}

	if err := r.ensureAdminSecret(ctx, vdi); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureHome(ctx, vdi); err != nil {
		return r.fail(ctx, vdi, ConditionPodReady, "HomeApply", err)
	}
	pullSecrets, err := r.ensureSandboxIdentity(ctx, vdi)
	if err != nil {
		return r.fail(ctx, vdi, ConditionPodReady, "SandboxIdentity", err)
	}
	seccomp, err := r.chooseSeccomp(ctx, vdi)
	if err != nil {
		return r.fail(ctx, vdi, ConditionPodReady, "SeccompProfile", err)
	}
	setCondition(vdi, ConditionSeccompFiltered,
		seccomp.Reason != seccompNone && seccomp.Reason != seccompPending, seccomp.Reason, seccomp.Message)
	if seccomp.Profile == nil {
		// Starting unfiltered now and filtered later would restart the
		// desktop under its user; a few seconds of waiting does not.
		setCondition(vdi, ConditionPodReady, false, seccomp.Reason, seccomp.Message)
		setCondition(vdi, ConditionReady, false, seccomp.Reason, "")
		vdi.Status.Phase = vdiv1alpha1.PhaseProvisioning
		if err := r.Status().Update(ctx, vdi); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}
	dep, err := r.ensureDeployment(ctx, vdi, pullSecrets, seccomp.Profile)
	if err != nil {
		return r.fail(ctx, vdi, ConditionPodReady, "DeploymentApply", err)
	}
	if err := r.ensureOwnerRBAC(ctx, vdi); err != nil {
		return ctrl.Result{}, err
	}
	result, err := r.updateStatus(ctx, vdi, dep, svc)
	if err != nil {
		return result, err
	}
	return r.reconcilePairing(ctx, vdi, result)
}

// chooseBasePort honours a pinned port, else takes the lowest free window.
func (r *VdiInstanceReconciler) chooseBasePort(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (int32, error) {
	if vdi.Spec.Expose.Type == vdiv1alpha1.ExposeLoadBalancer {
		if vdi.Spec.Expose.BasePort != 0 {
			return vdi.Spec.Expose.BasePort, nil
		}
		return loadBalancerBasePort, nil
	}
	pinned := vdi.Spec.Expose.BasePort
	if pinned != 0 && !nodePortBaseValid(pinned) {
		return 0, fmt.Errorf("basePort %d leaves the NodePort range %d-%d", pinned, nodePortMin, nodePortMax)
	}
	// Two desktops pinned to one window would refuse each other forever
	// if pins counted here; the first to hold it keeps it.
	held, err := r.heldNodePorts(ctx, vdi, pinned == 0)
	if err != nil {
		return 0, err
	}
	if pinned == 0 {
		return allocateNodePortBase(held)
	}
	if port, holder, taken := held.conflict(pinned); taken {
		return 0, &portConflictError{base: pinned, port: port, holder: holder}
	}
	return pinned, nil
}

// heldNodePorts lists the node ports other desktops hold or, with pins,
// have asked for, and those any Service in the cluster holds. Both are
// read uncached: allocation is rare, a stale read hands out a window
// twice, and the cache does not carry foreign Services.
func (r *VdiInstanceReconciler) heldNodePorts(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, pins bool,
) (heldPorts, error) {
	held := heldPorts{}
	desktops := &vdiv1alpha1.VdiInstanceList{}
	if err := r.reader().List(ctx, desktops); err != nil {
		return nil, err
	}
	for i := range desktops.Items {
		other := &desktops.Items[i]
		if other.UID == vdi.UID || other.Spec.Expose.Type == vdiv1alpha1.ExposeLoadBalancer {
			continue
		}
		holder := "VdiInstance " + other.Namespace + "/" + other.Name
		if other.Status.BasePort != 0 {
			held.holdWindow(other.Status.BasePort, holder)
		}
		if pins && other.Spec.Expose.BasePort != 0 {
			held.holdWindow(other.Spec.Expose.BasePort, holder)
		}
	}
	services := &corev1.ServiceList{}
	if err := r.reader().List(ctx, services); err != nil {
		return nil, err
	}
	for i := range services.Items {
		svc := &services.Items[i]
		if metav1.IsControlledBy(svc, vdi) {
			continue
		}
		for _, p := range svc.Spec.Ports {
			if p.NodePort != 0 {
				held.hold(p.NodePort, "Service "+svc.Namespace+"/"+svc.Name)
			}
		}
	}
	return held, nil
}

// releaseWindow drops a window the API server refused node ports for, so
// the next pass allocates another or, for a pinned base, names the holder.
func (r *VdiInstanceReconciler) releaseWindow(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, cause error,
) (ctrl.Result, error) {
	// A Service the cache missed holds these ports itself; its window stays.
	err := r.reader().Get(ctx, client.ObjectKey{Name: vdi.Name, Namespace: vdi.Namespace}, &corev1.Service{})
	switch {
	case err == nil:
		logf.FromContext(ctx).V(1).Info("retrying after a stale read", "cause", cause.Error())
		return ctrl.Result{RequeueAfter: time.Second}, nil
	case !apierrors.IsNotFound(err):
		return ctrl.Result{}, err
	}
	logf.FromContext(ctx).Info("node ports taken, giving up the window", "basePort", vdi.Status.BasePort)
	vdi.Status.BasePort = 0
	return r.fail(ctx, vdi, ConditionNetworkReady, "PortConflict", cause)
}

// reader reads past the cache.
func (r *VdiInstanceReconciler) reader() client.Reader {
	if r.APIReader == nil {
		return r.Client
	}
	return r.APIReader
}

func (r *VdiInstanceReconciler) ensureAdminSecret(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: adminSecretName(vdi), Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		labelManaged(secret, vdi.Name)
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
	return err
}

// ensureHome creates the home volume once. It outlives every restart
// of the desktop and goes away with the instance.
func (r *VdiInstanceReconciler) ensureHome(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: homeClaimName(vdi), Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		labelManaged(pvc, vdi.Name)
		// A bound claim accepts next to no change.
		if pvc.UID != "" {
			return nil
		}
		size := vdi.Spec.Home.Size
		if size.IsZero() {
			size = resource.MustParse("20Gi")
		}
		pvc.Spec = corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: vdi.Spec.Home.StorageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
		}
		return controllerutil.SetControllerReference(vdi, pvc, r.Scheme)
	})
	return err
}

// ensureSandboxIdentity gives a sandbox desktop a service account of
// its own and, on OpenShift, binds only that account to the
// nested-container SCC. Other desktops keep the namespace default and
// its restricted SCC. It returns the pull secrets the default account
// carries, which the new account does not inherit.
func (r *VdiInstanceReconciler) ensureSandboxIdentity(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance,
) ([]corev1.LocalObjectReference, error) {
	meta := metav1.ObjectMeta{Name: sandboxAccountName(vdi), Namespace: vdi.Namespace}
	sa := &corev1.ServiceAccount{ObjectMeta: meta}
	binding := &rbacv1.RoleBinding{ObjectMeta: meta}
	if !vdi.Spec.NestedSandboxes {
		for _, obj := range []client.Object{binding, sa} {
			if err := r.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		labelManaged(sa, vdi.Name)
		// The desktop never sees a token of this account.
		sa.AutomountServiceAccountToken = ptr.To(false)
		return controllerutil.SetControllerReference(vdi, sa, r.Scheme)
	}); err != nil {
		return nil, err
	}

	err := r.reader().Get(ctx, client.ObjectKey{Name: nestedSCCRole}, &rbacv1.ClusterRole{})
	switch {
	case apierrors.IsNotFound(err):
		// Not OpenShift: Pod Security Admission decides, nothing to bind.
	case err != nil:
		return nil, err
	default:
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
			labelManaged(binding, vdi.Name)
			binding.Subjects = []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: sa.Name, Namespace: sa.Namespace}}
			binding.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: nestedSCCRole}
			return controllerutil.SetControllerReference(vdi, binding, r.Scheme)
		}); err != nil {
			return nil, err
		}
	}

	def := &corev1.ServiceAccount{}
	if err := r.Get(ctx, client.ObjectKey{Name: "default", Namespace: vdi.Namespace}, def); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	var pullSecrets []corev1.LocalObjectReference
	for _, ref := range def.ImagePullSecrets {
		// OpenShift's own registry credential is tied to the default account.
		if !strings.HasPrefix(ref.Name, "default-dockercfg-") {
			pullSecrets = append(pullSecrets, ref)
		}
	}
	return pullSecrets, nil
}

func (r *VdiInstanceReconciler) ensureDeployment(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, pullSecrets []corev1.LocalObjectReference,
	seccomp *corev1.SeccompProfile,
) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: vdi.Name, Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		// The pod keeps this one label: another would restart every desktop.
		labels := map[string]string{labelInstance: vdi.Name}
		replicas := int32(1)
		if vdi.Spec.Running != nil && !*vdi.Spec.Running {
			replicas = 0
		}
		labelManaged(dep, vdi.Name)
		dep.Spec.Replicas = &replicas
		// The home volume and the ports belong to one pod at a time.
		dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
		if dep.UID == "" {
			dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		}
		dep.Spec.Template.Labels = labels
		fillPodSpec(vdi, &dep.Spec.Template.Spec, pullSecrets, seccomp)
		return controllerutil.SetControllerReference(vdi, dep, r.Scheme)
	})
	return dep, err
}

// fillPodSpec fills the pod in place. Fields it does not name keep what
// the API server defaulted, so an unchanged instance writes nothing.
func fillPodSpec(
	vdi *vdiv1alpha1.VdiInstance, pod *corev1.PodSpec, pullSecrets []corev1.LocalObjectReference,
	seccomp *corev1.SeccompProfile,
) {
	base := vdi.Status.BasePort
	web := strconv.Itoa(int(base + sunshine.OffsetWeb))

	pod.Hostname = vdi.Name
	// A desktop runs whatever its user starts; none of it gets an API token.
	pod.AutomountServiceAccountToken = ptr.To(false)
	pod.EnableServiceLinks = ptr.To(false)
	pod.TerminationGracePeriodSeconds = ptr.To(int64(10))
	pod.ImagePullSecrets = append(append([]corev1.LocalObjectReference{}, vdi.Spec.ImagePullSecrets...), pullSecrets...)
	if len(pod.ImagePullSecrets) == 0 {
		pod.ImagePullSecrets = nil
	}
	if pod.SecurityContext == nil {
		pod.SecurityContext = &corev1.PodSecurityContext{}
	}
	psc := pod.SecurityContext
	psc.RunAsNonRoot = ptr.To(true)
	psc.SeccompProfile = seccomp
	sandbox := vdi.Spec.NestedSandboxes
	if sandbox {
		pod.ServiceAccountName = sandboxAccountName(vdi)
		pod.DeprecatedServiceAccount = pod.ServiceAccountName
		// Its own user namespace is what makes the rest safe to grant:
		// every ID in the pod is an unprivileged one on the node.
		pod.HostUsers = ptr.To(false)
		// The SCC would default to root inside the namespace.
		psc.RunAsUser, psc.RunAsGroup, psc.FSGroup = ptr.To(desktopUID), ptr.To(desktopUID), ptr.To(desktopUID)
	} else {
		if pod.ServiceAccountName == sandboxAccountName(vdi) {
			pod.ServiceAccountName, pod.DeprecatedServiceAccount = "", ""
		}
		pod.HostUsers = nil
		psc.RunAsUser, psc.RunAsGroup, psc.FSGroup = nil, nil, nil
	}
	pod.Volumes = []corev1.Volume{
		{Name: "home", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: homeClaimName(vdi)},
		}},
		// The default 64Mi of /dev/shm starves browsers and games.
		{Name: "shm", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium: corev1.StorageMediumMemory, SizeLimit: ptr.To(resource.MustParse("1Gi")),
		}}},
	}

	if len(pod.Containers) != 1 || pod.Containers[0].Name != containerName {
		pod.Containers = []corev1.Container{{Name: containerName}}
	}
	c := &pod.Containers[0]
	c.Image = vdi.Spec.Image
	if vdi.Spec.ImagePullPolicy != "" {
		c.ImagePullPolicy = vdi.Spec.ImagePullPolicy
	}
	c.Resources = vdi.Spec.Resources
	c.Env = []corev1.EnvVar{
		{Name: "DESKUBE_NAME", Value: vdi.Name},
		{Name: "DESKUBE_PORT", Value: strconv.Itoa(int(base))},
		{Name: "DESKUBE_ADMIN_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: adminSecretName(vdi)}, Key: "password",
		}}},
		// The web UI only accepts writes from origins it was told about.
		// The node address is what a NodePort client browses to.
		{Name: "DESKUBE_HOST_IP", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.hostIP"},
		}},
		// Sunshine counts pod IPs as lan, so lan still lets the manager in.
		{Name: "DESKUBE_WEB_UI_ALLOWED", Value: webUIAllowed(vdi)},
	}
	if host := vdi.Spec.Expose.Host; host != "" {
		c.Env = append(c.Env, corev1.EnvVar{Name: "DESKUBE_WEB_ORIGINS", Value: "https://" + net.JoinHostPort(host, web)})
	}
	if vdi.Spec.Stream.Encoder != "" {
		c.Env = append(c.Env, corev1.EnvVar{Name: "DESKUBE_ENCODER", Value: vdi.Spec.Stream.Encoder})
	}
	if vdi.Spec.Stream.KeyboardLayout != "" {
		c.Env = append(c.Env, corev1.EnvVar{Name: "DESKUBE_KEYBOARD_LAYOUT", Value: vdi.Spec.Stream.KeyboardLayout})
	}
	c.Ports = make([]corev1.ContainerPort, 0, len(sunshine.Ports))
	for _, p := range sunshine.Ports {
		c.Ports = append(c.Ports, corev1.ContainerPort{
			Name: p.Name, ContainerPort: base + p.Offset, Protocol: corev1.Protocol(p.Protocol),
		})
	}
	c.VolumeMounts = []corev1.VolumeMount{
		{Name: "home", MountPath: homePath},
		{Name: "shm", MountPath: "/dev/shm"},
	}
	c.SecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
	if sandbox {
		// A sandbox mounts a fresh /proc, which the kernel refuses while
		// parts of the outer one are hidden.
		c.SecurityContext.ProcMount = ptr.To(corev1.UnmaskedProcMount)
	}
	// Every field is spelled out: a defaulted one would read as a change.
	c.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/serverinfo", Port: intstr.FromInt32(base), Scheme: corev1.URISchemeHTTP,
		}},
		PeriodSeconds: 5, TimeoutSeconds: 2, SuccessThreshold: 1, FailureThreshold: 3,
	}
}

func (r *VdiInstanceReconciler) ensureService(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance,
) (*corev1.Service, error) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: vdi.Name, Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		labelManaged(svc, vdi.Name)
		svc.Spec.Selector = map[string]string{labelInstance: vdi.Name}
		svc.Spec.Type = corev1.ServiceTypeNodePort
		if vdi.Spec.Expose.Type == vdiv1alpha1.ExposeLoadBalancer {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		}
		// A manager outside the cluster reaches the admin API through the Service.
		web := vdi.Spec.Expose.WebUI || r.ProbeExternal
		svc.Spec.Ports = servicePorts(vdi.Status.BasePort, svc.Spec.Type == corev1.ServiceTypeNodePort, web)
		return controllerutil.SetControllerReference(vdi, svc, r.Scheme)
	})
	return svc, err
}

// servicePorts maps each Sunshine port 1:1, so the desktop and the
// outside agree on the numbers RTSP advertises.
func servicePorts(base int32, pinNodePorts, web bool) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(sunshine.Ports))
	for _, p := range sunshine.Ports {
		if p.Offset == sunshine.OffsetWeb && !web {
			continue
		}
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

// ensureOwnerRBAC lets the owner read the instance, write to it, and,
// with the web UI published, read its admin password, of no use to the
// owner otherwise. RBAC cannot tell one field from another: config/policy
// narrows the writes to running and pairing.
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
		labelManaged(role, vdi.Name)
		group := vdiv1alpha1.GroupVersion.Group
		// list and watch ignore resourceNames, so the owner sees every
		// desktop name in the namespace and can only change their own.
		role.Rules = []rbacv1.PolicyRule{
			{APIGroups: []string{group}, Resources: []string{"vdiinstances"}, Verbs: verbsRead},
			{
				APIGroups: []string{group}, Resources: []string{"vdiinstances"},
				ResourceNames: []string{vdi.Name}, Verbs: []string{"update", "patch"},
			},
		}
		if vdi.Spec.Expose.WebUI {
			role.Rules = append(role.Rules, rbacv1.PolicyRule{
				APIGroups: []string{""}, Resources: []string{"secrets"},
				ResourceNames: []string{adminSecretName(vdi)}, Verbs: verbsRead[:1],
			})
		}
		return controllerutil.SetControllerReference(vdi, role, r.Scheme)
	}); err != nil {
		return err
	}
	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vdi.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		labelManaged(binding, vdi.Name)
		binding.Subjects = []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: vdi.Spec.Owner}}
		binding.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name}
		return controllerutil.SetControllerReference(vdi, binding, r.Scheme)
	})
	return err
}

var verbsRead = []string{"get", "list", "watch"}

func (r *VdiInstanceReconciler) updateStatus(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, dep *appsv1.Deployment, svc *corev1.Service,
) (ctrl.Result, error) {
	running := vdi.Spec.Running == nil || *vdi.Spec.Running
	vdi.Status.AdminSecret = adminSecretName(vdi)
	vdi.Status.ObservedGeneration = vdi.Generation

	pod, err := r.findPod(ctx, vdi)
	if err != nil {
		return ctrl.Result{}, err
	}
	// The readiness probe is Sunshine answering, so a ready pod streams.
	podReady := dep.Status.ReadyReplicas > 0
	reason, message := podState(pod)
	setCondition(vdi, ConditionPodReady, podReady, reason, message)

	host, err := r.clientHost(ctx, vdi, svc, pod)
	if host != "" {
		vdi.Status.Address = net.JoinHostPort(host, strconv.Itoa(int(vdi.Status.BasePort)))
	}
	setCondition(vdi, ConditionNetworkReady, err == nil && host != "", "Address", errString(err))

	streaming := false
	vdi.Status.SessionState = ""
	if podReady && host != "" {
		probe := r.Probe
		if probe == nil {
			probe = sunshine.GetServerInfo
		}
		probeHost, perr := r.sunshineHost(vdi, pod)
		if perr == nil {
			var info *sunshine.ServerInfo
			info, perr = probe(ctx, probeHost, vdi.Status.BasePort)
			if perr == nil && info.StatusCode == 200 {
				streaming = true
				vdi.Status.SessionState = info.SessionState()
			}
		}
		setCondition(vdi, ConditionStreamingReady, streaming, "ServerInfo", errString(perr))
	} else {
		setCondition(vdi, ConditionStreamingReady, false, "PodNotReady", "")
	}

	ready := podReady && streaming && host != ""
	setCondition(vdi, ConditionReady, ready, "Aggregate", "")

	idleLeft, err := r.trackIdle(ctx, vdi, running, ready)
	if err != nil {
		return ctrl.Result{}, err
	}
	running = vdi.Spec.Running == nil || *vdi.Spec.Running
	vdi.Status.Phase = phase(running, ready, pod)

	if err := r.Status().Update(ctx, vdi); err != nil {
		return ctrl.Result{}, err
	}
	switch {
	case !running && pod == nil:
		return ctrl.Result{}, nil
	case ready && idleLeft > 0 && idleLeft < requeueReady:
		return ctrl.Result{RequeueAfter: idleLeft + time.Second}, nil
	case ready:
		return ctrl.Result{RequeueAfter: requeueReady}, nil
	default:
		return ctrl.Result{RequeueAfter: requeueStarting}, nil
	}
}

// sunshineHost is where the manager reaches Sunshine, for /serverinfo
// and the admin API alike. The pod IP needs no port on the Service. A
// manager outside the cluster cannot route to pods: it takes the client
// address, and the Service must then publish the web port too.
func (r *VdiInstanceReconciler) sunshineHost(vdi *vdiv1alpha1.VdiInstance, pod *corev1.Pod) (string, error) {
	if r.ProbeExternal {
		host, _, err := net.SplitHostPort(vdi.Status.Address)
		if err != nil {
			return "", errors.New("instance has no address yet")
		}
		return host, nil
	}
	if pod == nil || pod.Status.PodIP == "" {
		return "", errors.New("desktop pod has no IP yet")
	}
	return pod.Status.PodIP, nil
}

// findPod returns the instance's pod, nil when there is none.
func (r *VdiInstanceReconciler) findPod(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (*corev1.Pod, error) {
	pods := &corev1.PodList{}
	err := r.List(ctx, pods, client.InNamespace(vdi.Namespace), client.MatchingLabels{labelInstance: vdi.Name})
	if err != nil {
		return nil, err
	}
	var found *corev1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		// Recreate leaves the old pod terminating next to the new one.
		if found == nil || (found.DeletionTimestamp != nil && pod.DeletionTimestamp == nil) {
			found = pod
		}
	}
	return found, nil
}

// podState says why the desktop is not up yet, in the kubelet's words.
func podState(pod *corev1.Pod) (reason, message string) {
	if pod == nil {
		return "NoPod", ""
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && w.Reason != "" {
			return w.Reason, w.Message
		}
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			return cond.Reason, cond.Message
		}
	}
	return string(pod.Status.Phase), ""
}

// trackIdle keeps lastActive fresh while a session runs and turns the
// desktop off once it sat free for idleTimeout. It returns how long the
// desktop may still idle, zero when no timeout applies.
func (r *VdiInstanceReconciler) trackIdle(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, running, ready bool,
) (time.Duration, error) {
	if !running {
		vdi.Status.LastActive = nil
		return 0, nil
	}
	if !ready {
		return 0, nil
	}
	now := metav1.Now()
	if vdi.Status.SessionState == "busy" || vdi.Status.LastActive == nil {
		vdi.Status.LastActive = &now
	}
	if vdi.Spec.IdleTimeout == nil || vdi.Status.SessionState != "free" {
		return 0, nil
	}
	left := vdi.Spec.IdleTimeout.Duration - now.Sub(vdi.Status.LastActive.Time)
	if left > 0 {
		return left, nil
	}
	logf.FromContext(ctx).Info("idle timeout reached, stopping the desktop", "idleTimeout", vdi.Spec.IdleTimeout.Duration)
	// Update returns the stored status and would wipe the one built here,
	// so the spec change goes through a copy.
	stopped := vdi.DeepCopy()
	stopped.Spec.Running = ptr.To(false)
	if err := r.Update(ctx, stopped); err != nil {
		return 0, err
	}
	vdi.ResourceVersion = stopped.ResourceVersion
	vdi.Spec.Running = ptr.To(false)
	message := "stopped after " + vdi.Spec.IdleTimeout.Duration.String() + " without a session"
	setCondition(vdi, ConditionReady, false, "IdleShutdown", message)
	return 0, nil
}

// Waiting reasons a retry does not cure on its own.
var failedWaiting = map[string]bool{
	"CrashLoopBackOff": true, "ImagePullBackOff": true, "ErrImagePull": true,
	"InvalidImageName": true, "CreateContainerConfigError": true, "CreateContainerError": true,
}

func phase(running, ready bool, pod *corev1.Pod) vdiv1alpha1.VdiInstancePhase {
	switch {
	case !running && pod != nil:
		return vdiv1alpha1.PhaseStopping
	case !running:
		return vdiv1alpha1.PhaseStopped
	case ready:
		return vdiv1alpha1.PhaseReady
	case pod == nil || pod.Spec.NodeName == "":
		return vdiv1alpha1.PhaseProvisioning
	}
	if reason, _ := podState(pod); failedWaiting[reason] {
		return vdiv1alpha1.PhaseFailed
	}
	return vdiv1alpha1.PhaseStarting
}

// clientHost is the address a Moonlight client dials.
func (r *VdiInstanceReconciler) clientHost(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, svc *corev1.Service, pod *corev1.Pod,
) (string, error) {
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
	// NodePorts answer on every node, the one running the desktop has
	// the shortest path.
	nodeName := ""
	if pod != nil {
		nodeName = pod.Spec.NodeName
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
func (r *VdiInstanceReconciler) fail(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, condition, reason string, cause error,
) (ctrl.Result, error) {
	setCondition(vdi, condition, false, reason, cause.Error())
	setCondition(vdi, ConditionReady, false, reason, "")
	vdi.Status.Phase = vdiv1alpha1.PhaseFailed
	if err := r.Status().Update(ctx, vdi); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, cause
}

// isTransient covers the two errors a stale informer cache produces.
func isTransient(err error) bool {
	return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
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

// sanitizeReason keeps free form reasons valid as condition reasons.
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

// webUIAllowed is what desk-session writes to Sunshine's origin_web_ui_allowed.
func webUIAllowed(vdi *vdiv1alpha1.VdiInstance) string {
	if vdi.Spec.Expose.WebUI {
		return "wan"
	}
	return "lan"
}

func adminSecretName(vdi *vdiv1alpha1.VdiInstance) string { return vdi.Name + "-sunshine-admin" }
func homeClaimName(vdi *vdiv1alpha1.VdiInstance) string   { return vdi.Name + "-home" }

func sandboxAccountName(vdi *vdiv1alpha1.VdiInstance) string { return vdi.Name + "-desktop" }

// labelManaged keeps the labels others set. An empty instance names none.
func labelManaged(obj metav1.Object, instance string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[labelManagedBy] = managedBy
	if instance != "" {
		labels[labelInstance] = instance
	}
	obj.SetLabels(labels)
}

// SetupWithManager registers the controller and its owned resources.
func (r *VdiInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vdiv1alpha1.VdiInstance{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Named("vdiinstance").
		Complete(r)
}
