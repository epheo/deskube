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
	"errors"
	"path/filepath"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

const (
	testNS    = "vdi-test"
	testImage = "registry.example/desktop:1"
)

var _ = Describe("VdiInstance controller", Ordered, func() {
	const podIP = "10.128.0.7"
	var (
		r         *VdiInstanceReconciler
		probe     *sunshine.ServerInfo
		probedVia string
	)

	reconcile := func(name string) {
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: testNS}})
		Expect(err).NotTo(HaveOccurred())
	}
	get := func(name string, obj client.Object) {
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNS}, obj)).To(Succeed())
	}
	expectLabeled := func(instance string, objs ...client.Object) {
		for _, obj := range objs {
			Expect(obj.GetLabels()).To(HaveKeyWithValue(labelManagedBy, managedBy), obj.GetName())
			Expect(obj.GetLabels()).To(HaveKeyWithValue(labelInstance, instance), obj.GetName())
		}
	}
	// envtest runs no kubelet and no deployment controller: the test plays both.
	startPod := func(name string) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name + "-0", Namespace: testNS, Labels: map[string]string{labelInstance: name}},
			Spec: corev1.PodSpec{
				NodeName:   "node-a",
				Containers: []corev1.Container{{Name: containerName, Image: testImage}},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status.PodIP = podIP
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		dep := &appsv1.Deployment{}
		get(name, dep)
		dep.Status.Replicas, dep.Status.ReadyReplicas = 1, 1
		Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
	}
	// The ports any source reaches, and those only the manager's pods reach.
	policyPorts := func(name string) (anyone, manager []string) {
		policy := &networkingv1.NetworkPolicy{}
		get(name, policy)
		for _, rule := range policy.Spec.Ingress {
			for _, p := range rule.Ports {
				port := string(*p.Protocol) + "/" + p.Port.String()
				if len(rule.From) == 0 {
					anyone = append(anyone, port)
				} else {
					manager = append(manager, port)
				}
			}
		}
		return anyone, manager
	}
	stopPod := func(name string) {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name + "-0", Namespace: testNS}}
		Expect(k8sClient.Delete(ctx, pod, client.GracePeriodSeconds(0))).To(Succeed())
		dep := &appsv1.Deployment{}
		get(name, dep)
		dep.Status.Replicas, dep.Status.ReadyReplicas = 0, 0
		Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
	}

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNS}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.5"}, {Type: corev1.NodeExternalIP, Address: "203.0.113.10"}}},
		})).To(Succeed())
		r = &VdiInstanceReconciler{
			Client:           k8sClient,
			Scheme:           scheme.Scheme,
			ManagerNamespace: "deskube-system",
			Probe: func(_ context.Context, host string, _ int32) (*sunshine.ServerInfo, error) {
				probedVia = host
				if probe == nil {
					return nil, errors.New("connection refused")
				}
				return probe, nil
			},
		}
	})

	It("creates the children and allocates the first window", func() {
		vdi := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "alice", Namespace: testNS},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				Image: testImage,
				Owner: "alice",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
					Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
				},
				Stream: vdiv1alpha1.StreamSpec{KeyboardLayout: "fr"},
			},
		}
		Expect(k8sClient.Create(ctx, vdi)).To(Succeed())
		reconcile("alice")

		get("alice", vdi)
		Expect(vdi.Status.BasePort).To(Equal(int32(30016)))
		Expect(vdi.Status.Address).To(Equal("203.0.113.10:30016"))
		Expect(vdi.Status.AdminSecret).To(Equal("alice-sunshine-admin"))
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseProvisioning))
		Expect(meta.IsStatusConditionTrue(vdi.Status.Conditions, ConditionSeccompFiltered)).To(BeTrue())

		admin := &corev1.Secret{}
		get("alice-sunshine-admin", admin)
		Expect(admin.Data["username"]).To(Equal([]byte("admin")))
		Expect(admin.Data["password"]).To(HaveLen(32))
		Expect(metav1.IsControlledBy(admin, vdi)).To(BeTrue())

		home := &corev1.PersistentVolumeClaim{}
		get("alice-home", home)
		Expect(home.Spec.Resources.Requests.Storage().String()).To(Equal("20Gi"))
		Expect(home.Spec.StorageClassName).To(BeNil())
		Expect(metav1.IsControlledBy(home, vdi)).To(BeTrue())

		dep := &appsv1.Deployment{}
		get("alice", dep)
		Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		Expect(dep.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))
		// Any other pod label would restart running desktops on upgrade.
		Expect(dep.Spec.Template.Labels).To(Equal(map[string]string{labelInstance: "alice"}))
		pod := dep.Spec.Template.Spec
		Expect(*pod.AutomountServiceAccountToken).To(BeFalse())
		Expect(*pod.SecurityContext.RunAsNonRoot).To(BeTrue())
		// Without placement the pod is what it was before placement existed.
		Expect(pod.NodeSelector).To(BeNil())
		Expect(pod.Tolerations).To(BeNil())
		Expect(pod.Affinity).To(BeNil())
		Expect(pod.PriorityClassName).To(BeEmpty())
		Expect(pod.RuntimeClassName).To(BeNil())
		Expect(pod.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal("alice-home"))
		c := pod.Containers[0]
		Expect(c.Image).To(Equal(testImage))
		Expect(c.Resources.Requests.Cpu().String()).To(Equal("2"))
		Expect(c.Resources.Limits.Memory().String()).To(Equal("8Gi"))
		Expect(*c.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
		Expect(c.SecurityContext.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
		Expect(c.ReadinessProbe.HTTPGet.Port.IntValue()).To(Equal(30016))
		Expect(c.VolumeMounts[0].MountPath).To(Equal("/home/desk"))
		env := map[string]corev1.EnvVar{}
		for _, e := range c.Env {
			env[e.Name] = e
		}
		Expect(env["DESKUBE_PORT"].Value).To(Equal("30016"))
		Expect(env["DESKUBE_NAME"].Value).To(Equal("alice"))
		Expect(env["DESKUBE_KEYBOARD_LAYOUT"].Value).To(Equal("fr"))
		for _, e := range c.Env {
			Expect(e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil).To(BeTrue(), "%s comes from a Secret", e.Name)
		}
		Expect(env["DESKUBE_ADMIN_PASSWORD_FILE"].Value).To(Equal("/run/deskube/admin/password"))
		Expect(pod.Volumes).To(ContainElement(corev1.Volume{Name: "admin", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  "alice-sunshine-admin",
				Items:       []corev1.KeyToPath{{Key: "password", Path: "password"}},
				DefaultMode: ptr.To(int32(0o440)),
			},
		}}))
		Expect(c.VolumeMounts).To(ContainElement(corev1.VolumeMount{Name: "admin", MountPath: "/run/deskube/admin", ReadOnly: true}))
		Expect(env["DESKUBE_HOST_IP"].ValueFrom.FieldRef.FieldPath).To(Equal("status.hostIP"))
		Expect(env["DESKUBE_WEB_UI_ALLOWED"].Value).To(Equal("lan"))
		Expect(env).NotTo(HaveKey("DESKUBE_ENCODER"))
		Expect(env).NotTo(HaveKey("DESKUBE_WEB_ORIGINS"))

		svc := &corev1.Service{}
		get("alice", svc)
		Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
		Expect(svc.Spec.Selector).To(HaveKeyWithValue(labelInstance, "alice"))
		Expect(svc.Spec.Ports).To(HaveLen(6))
		for _, p := range svc.Spec.Ports {
			Expect(p.NodePort).To(Equal(p.Port))
			Expect(p.Name).NotTo(Equal("web"))
		}

		role := &rbacv1.Role{}
		get("alice-owner", role)
		// No web UI published, so no admin password for the owner either.
		Expect(role.Rules).To(HaveLen(2))
		Expect(role.Rules[0].ResourceNames).To(BeEmpty())
		Expect(role.Rules[0].Verbs).To(ConsistOf("get", "list", "watch"))
		Expect(role.Rules[1].ResourceNames).To(ConsistOf("alice"))
		Expect(role.Rules[1].Verbs).To(ConsistOf("update", "patch"))
		binding := &rbacv1.RoleBinding{}
		get("alice-owner", binding)
		Expect(binding.Subjects[0].Name).To(Equal("alice"))

		policy := &networkingv1.NetworkPolicy{}
		get("alice", policy)
		Expect(metav1.IsControlledBy(policy, vdi)).To(BeTrue())
		Expect(policy.Spec.PodSelector.MatchLabels).To(Equal(dep.Spec.Template.Labels))
		// Egress stays open: a desktop browses the internet.
		Expect(policy.Spec.PolicyTypes).To(ConsistOf(networkingv1.PolicyTypeIngress))
		Expect(policy.Spec.Ingress).To(HaveLen(2))
		Expect(policy.Spec.Ingress[1].From).To(ConsistOf(networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "deskube-system"}},
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"control-plane": "controller-manager"}},
		}))
		anyone, manager := policyPorts("alice")
		// The readiness probe's port, 30016, is one of them.
		Expect(anyone).To(ConsistOf("TCP/30011", "TCP/30016", "UDP/30025", "UDP/30026", "UDP/30027", "TCP/30037"))
		Expect(manager).To(ConsistOf("TCP/30017"))
		expectLabeled("alice", admin, home, dep, svc, role, binding, policy)
	})

	It("writes nothing when nothing changed", func() {
		dep := &appsv1.Deployment{}
		get("alice", dep)
		policy := &networkingv1.NetworkPolicy{}
		get("alice", policy)
		before, policyBefore := dep.ResourceVersion, policy.ResourceVersion
		reconcile("alice")
		get("alice", dep)
		Expect(dep.ResourceVersion).To(Equal(before))
		Expect(dep.Generation).To(Equal(int64(1)))
		get("alice", policy)
		Expect(policy.ResourceVersion).To(Equal(policyBefore))
	})

	It("reports Ready once the pod is ready and Sunshine answers", func() {
		startPod("alice")
		reconcile("alice")
		vdi := &vdiv1alpha1.VdiInstance{}
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseStarting))
		Expect(meta.IsStatusConditionTrue(vdi.Status.Conditions, ConditionPodReady)).To(BeTrue())
		Expect(meta.IsStatusConditionFalse(vdi.Status.Conditions, ConditionStreamingReady)).To(BeTrue())

		probe = &sunshine.ServerInfo{StatusCode: 200, State: "SUNSHINE_SERVER_BUSY"}
		reconcile("alice")
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseReady))
		Expect(vdi.Status.SessionState).To(Equal("busy"))
		Expect(meta.IsStatusConditionTrue(vdi.Status.Conditions, ConditionReady)).To(BeTrue())
		Expect(probedVia).To(Equal(podIP))

		// A manager outside the cluster cannot route to the pod.
		r.ProbeExternal = true
		reconcile("alice")
		r.ProbeExternal = false
		Expect(probedVia).To(Equal("203.0.113.10"))
	})

	It("follows a new image and an explicit host", func() {
		vdi := &vdiv1alpha1.VdiInstance{}
		get("alice", vdi)
		vdi.Spec.Image = "registry.example/desktop:2"
		vdi.Spec.Expose.Host = "desk.example.org"
		vdi.Spec.Stream.Encoder = "vaapi"
		Expect(k8sClient.Update(ctx, vdi)).To(Succeed())
		reconcile("alice")

		dep := &appsv1.Deployment{}
		get("alice", dep)
		c := dep.Spec.Template.Spec.Containers[0]
		Expect(c.Image).To(Equal("registry.example/desktop:2"))
		Expect(c.Env).To(ContainElement(corev1.EnvVar{Name: "DESKUBE_WEB_ORIGINS", Value: "https://desk.example.org:30017"}))
		Expect(c.Env).To(ContainElement(corev1.EnvVar{Name: "DESKUBE_ENCODER", Value: "vaapi"}))
		get("alice", vdi)
		Expect(vdi.Status.Address).To(Equal("desk.example.org:30016"))
	})

	It("publishes the web UI only when asked, or for a manager outside the cluster", func() {
		webPort := func() (int32, string) {
			svc := &corev1.Service{}
			get("alice", svc)
			var nodePort int32
			for _, p := range svc.Spec.Ports {
				if p.Name == "web" {
					nodePort = p.NodePort
				}
			}
			dep := &appsv1.Deployment{}
			get("alice", dep)
			for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
				if e.Name == "DESKUBE_WEB_UI_ALLOWED" {
					return nodePort, e.Value
				}
			}
			return nodePort, ""
		}
		ownerSecrets := func() []string {
			role := &rbacv1.Role{}
			get("alice-owner", role)
			var names []string
			for _, rule := range role.Rules {
				if slices.Contains(rule.Resources, "secrets") {
					Expect(rule.Verbs).To(ConsistOf("get"))
					names = append(names, rule.ResourceNames...)
				}
			}
			return names
		}
		vdi := &vdiv1alpha1.VdiInstance{}
		get("alice", vdi)
		vdi.Spec.Expose.WebUI = ptr.To(true)
		Expect(k8sClient.Update(ctx, vdi)).To(Succeed())
		reconcile("alice")
		port, allowed := webPort()
		Expect(port).To(Equal(int32(30017)))
		Expect(allowed).To(Equal("wan"))
		Expect(ownerSecrets()).To(ConsistOf("alice-sunshine-admin"))
		anyone, manager := policyPorts("alice")
		Expect(anyone).To(ContainElement("TCP/30017"))
		Expect(manager).To(BeEmpty())

		get("alice", vdi)
		vdi.Spec.Expose.WebUI = ptr.To(false)
		Expect(k8sClient.Update(ctx, vdi)).To(Succeed())
		reconcile("alice")
		port, allowed = webPort()
		Expect(port).To(BeZero())
		Expect(allowed).To(Equal("lan"))
		Expect(ownerSecrets()).To(BeEmpty())
		anyone, manager = policyPorts("alice")
		Expect(anyone).NotTo(ContainElement("TCP/30017"))
		Expect(manager).To(ConsistOf("TCP/30017"))

		r.ProbeExternal = true
		reconcile("alice")
		r.ProbeExternal = false
		port, allowed = webPort()
		Expect(port).To(Equal(int32(30017)))
		Expect(allowed).To(Equal("lan"))
		Expect(ownerSecrets()).To(BeEmpty())
		anyone, _ = policyPorts("alice")
		Expect(anyone).To(ContainElement("TCP/30017"))
		reconcile("alice")
		port, _ = webPort()
		Expect(port).To(BeZero())
		_, manager = policyPorts("alice")
		Expect(manager).To(ConsistOf("TCP/30017"))
	})

	It("gives the next instance the next window", func() {
		bob := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "bob", Namespace: testNS},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				Image: testImage,
				Home:  vdiv1alpha1.HomeSpec{Size: resource.MustParse("5Gi"), StorageClassName: ptr.To("fast")},
			},
		}
		Expect(k8sClient.Create(ctx, bob)).To(Succeed())
		reconcile("bob")
		get("bob", bob)
		Expect(bob.Status.BasePort).To(Equal(int32(30048)))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "bob-owner", Namespace: testNS}, &rbacv1.Role{})).NotTo(Succeed())
		home := &corev1.PersistentVolumeClaim{}
		get("bob-home", home)
		Expect(home.Spec.Resources.Requests.Storage().String()).To(Equal("5Gi"))
		Expect(*home.Spec.StorageClassName).To(Equal("fast"))
	})

	It("gives a load balancer desktop no node ports, and frees those it held", func() {
		nodePorts := func(svc *corev1.Service) []int32 {
			var held []int32
			for _, p := range svc.Spec.Ports {
				if p.NodePort != 0 {
					held = append(held, p.NodePort)
				}
			}
			return held
		}
		lb := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "lb", Namespace: testNS},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				Image: testImage, Expose: vdiv1alpha1.ExposeSpec{
					ClassExposeSpec: vdiv1alpha1.ClassExposeSpec{Type: vdiv1alpha1.ExposeLoadBalancer},
				},
			},
		}
		Expect(k8sClient.Create(ctx, lb)).To(Succeed())
		reconcile("lb")
		svc := &corev1.Service{}
		get("lb", svc)
		Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
		Expect(*svc.Spec.AllocateLoadBalancerNodePorts).To(BeFalse())
		Expect(nodePorts(svc)).To(BeEmpty())
		before := svc.ResourceVersion
		reconcile("lb")
		get("lb", svc)
		Expect(svc.ResourceVersion).To(Equal(before))

		// An older manager left the default: a random node port per port.
		svc.Spec.AllocateLoadBalancerNodePorts = ptr.To(true)
		Expect(k8sClient.Update(ctx, svc)).To(Succeed())
		get("lb", svc)
		Expect(nodePorts(svc)).To(HaveLen(6))
		reconcile("lb")
		get("lb", svc)
		Expect(*svc.Spec.AllocateLoadBalancerNodePorts).To(BeFalse())
		Expect(nodePorts(svc)).To(BeEmpty())
		before = svc.ResourceVersion
		reconcile("lb")
		get("lb", svc)
		Expect(svc.ResourceVersion).To(Equal(before))
	})

	It("says why a desktop does not start", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "bob-0", Namespace: testNS, Labels: map[string]string{labelInstance: "bob"}},
			Spec: corev1.PodSpec{
				NodeName:   "node-a",
				Containers: []corev1.Container{{Name: containerName, Image: testImage}},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  containerName,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull access denied"}},
		}}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		reconcile("bob")
		bob := &vdiv1alpha1.VdiInstance{}
		get("bob", bob)
		Expect(bob.Status.Phase).To(Equal(vdiv1alpha1.PhaseFailed))
		cond := meta.FindStatusCondition(bob.Status.Conditions, ConditionPodReady)
		Expect(cond.Reason).To(Equal("ImagePullBackOff"))
		Expect(cond.Message).To(Equal("pull access denied"))
	})

	It("gives a sandbox desktop its own identity and user namespace, and nothing more", func() {
		Expect(k8sClient.Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: nestedSCCRole}})).To(Succeed())
		// envtest runs no service account controller.
		Expect(k8sClient.Create(ctx, &corev1.ServiceAccount{
			ObjectMeta:       metav1.ObjectMeta{Name: "default", Namespace: testNS},
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry"}, {Name: "default-dockercfg-x7k2p"}},
		})).To(Succeed())
		carol := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "carol", Namespace: testNS},
			Spec:       vdiv1alpha1.VdiInstanceSpec{Image: testImage, NestedSandboxes: ptr.To(true)},
		}
		Expect(k8sClient.Create(ctx, carol)).To(Succeed())
		reconcile("carol")
		get("carol", carol)

		sa := &corev1.ServiceAccount{}
		get("carol-desktop", sa)
		Expect(*sa.AutomountServiceAccountToken).To(BeFalse())
		Expect(metav1.IsControlledBy(sa, carol)).To(BeTrue())
		binding := &rbacv1.RoleBinding{}
		get("carol-desktop", binding)
		Expect(binding.RoleRef.Kind).To(Equal("ClusterRole"))
		Expect(binding.RoleRef.Name).To(Equal(nestedSCCRole))
		Expect(binding.Subjects).To(ConsistOf(rbacv1.Subject{Kind: "ServiceAccount", Name: "carol-desktop", Namespace: testNS}))
		expectLabeled("carol", sa, binding)

		dep := &appsv1.Deployment{}
		get("carol", dep)
		pod := dep.Spec.Template.Spec
		Expect(pod.ServiceAccountName).To(Equal("carol-desktop"))
		Expect(*pod.AutomountServiceAccountToken).To(BeFalse())
		Expect(*pod.HostUsers).To(BeFalse())
		Expect(*pod.SecurityContext.RunAsNonRoot).To(BeTrue())
		Expect(*pod.SecurityContext.RunAsUser).To(Equal(int64(1000)))
		Expect(*pod.SecurityContext.FSGroup).To(Equal(int64(1000)))
		// No Security Profiles Operator here and no profile named: no filter, and it says so.
		Expect(pod.SecurityContext.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeUnconfined))
		filtered := meta.FindStatusCondition(carol.Status.Conditions, ConditionSeccompFiltered)
		Expect(filtered.Status).To(Equal(metav1.ConditionFalse))
		Expect(filtered.Reason).To(Equal("Unconfined"))
		Expect(filtered.Message).To(ContainSubstring("Security Profiles Operator"))
		Expect(pod.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: "registry"}))
		c := pod.Containers[0].SecurityContext
		Expect(*c.ProcMount).To(Equal(corev1.UnmaskedProcMount))
		Expect(*c.AllowPrivilegeEscalation).To(BeFalse())
		Expect(c.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
		Expect(c.Capabilities.Add).To(BeEmpty())
		Expect(c.Privileged).To(BeNil())

		before := dep.ResourceVersion
		reconcile("carol")
		get("carol", dep)
		Expect(dep.ResourceVersion).To(Equal(before))

		// A profile named on the command line replaces "no filter at all".
		r.SandboxSeccompProfile = "profiles/sandbox.json"
		reconcile("carol")
		r.SandboxSeccompProfile = ""
		get("carol", dep)
		seccomp := dep.Spec.Template.Spec.SecurityContext.SeccompProfile
		Expect(seccomp.Type).To(Equal(corev1.SeccompProfileTypeLocalhost))
		Expect(*seccomp.LocalhostProfile).To(Equal("profiles/sandbox.json"))
		get("carol", carol)
		Expect(meta.IsStatusConditionTrue(carol.Status.Conditions, ConditionSeccompFiltered)).To(BeTrue())

		carol.Spec.NestedSandboxes = nil
		Expect(k8sClient.Update(ctx, carol)).To(Succeed())
		reconcile("carol")
		key := types.NamespacedName{Name: "carol-desktop", Namespace: testNS}
		Expect(k8sClient.Get(ctx, key, &corev1.ServiceAccount{})).NotTo(Succeed())
		Expect(k8sClient.Get(ctx, key, &rbacv1.RoleBinding{})).NotTo(Succeed())
		get("carol", dep)
		pod = dep.Spec.Template.Spec
		Expect(pod.ServiceAccountName).To(BeEmpty())
		Expect(pod.HostUsers).To(BeNil())
		Expect(pod.SecurityContext.RunAsUser).To(BeNil())
		Expect(pod.SecurityContext.FSGroup).To(BeNil())
		Expect(pod.SecurityContext.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
		Expect(pod.Containers[0].SecurityContext.ProcMount).To(BeNil())
		Expect(pod.ImagePullSecrets).To(BeEmpty())
		get("carol", carol)
		Expect(meta.FindStatusCondition(carol.Status.Conditions, ConditionSeccompFiltered).Reason).To(Equal("RuntimeDefault"))
	})

	It("has the Security Profiles Operator install its filter and waits for it", func() {
		_, err := envtest.InstallCRDs(cfg, envtest.CRDInstallOptions{
			Paths: []string{filepath.Join("..", "..", "test", "crds")}, ErrorIfPathMissing: true,
		})
		Expect(err).NotTo(HaveOccurred())

		dave := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "dave", Namespace: testNS},
			Spec:       vdiv1alpha1.VdiInstanceSpec{Image: testImage, NestedSandboxes: ptr.To(true)},
		}
		Expect(k8sClient.Create(ctx, dave)).To(Succeed())
		// The mapper learns the new kind on its first miss.
		Eventually(func() error {
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "dave", Namespace: testNS}})
			return err
		}, "20s", "1s").Should(Succeed())

		profile := &unstructured.Unstructured{}
		profile.SetGroupVersionKind(seccompProfileGVK)
		get(sandboxProfileName, profile)
		action, _, _ := unstructured.NestedString(profile.Object, "spec", "defaultAction")
		Expect(action).To(Equal("SCMP_ACT_ERRNO"))
		arches, _, _ := unstructured.NestedStringSlice(profile.Object, "spec", "architectures")
		Expect(arches).To(ContainElement("SCMP_ARCH_X86"))
		// One per namespace, shared by its desktops.
		Expect(profile.GetLabels()).To(Equal(map[string]string{labelManagedBy: managedBy}))

		// Not on the nodes yet: the desktop waits instead of starting unfiltered.
		get("dave", dave)
		Expect(dave.Status.Phase).To(Equal(vdiv1alpha1.PhaseProvisioning))
		Expect(meta.FindStatusCondition(dave.Status.Conditions, ConditionSeccompFiltered).Reason).To(Equal("ProfilePending"))
		key := types.NamespacedName{Name: "dave", Namespace: testNS}
		Expect(k8sClient.Get(ctx, key, &appsv1.Deployment{})).NotTo(Succeed())

		Expect(unstructured.SetNestedField(profile.Object, "Installed", "status", "status")).To(Succeed())
		Expect(unstructured.SetNestedField(profile.Object, "operator/deskube-sandbox.json", "status", "localhostProfile")).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, profile)).To(Succeed())
		reconcile("dave")

		dep := &appsv1.Deployment{}
		get("dave", dep)
		seccomp := dep.Spec.Template.Spec.SecurityContext.SeccompProfile
		Expect(seccomp.Type).To(Equal(corev1.SeccompProfileTypeLocalhost))
		Expect(*seccomp.LocalhostProfile).To(Equal("operator/deskube-sandbox.json"))
		get("dave", dave)
		filtered := meta.FindStatusCondition(dave.Status.Conditions, ConditionSeccompFiltered)
		Expect(filtered.Status).To(Equal(metav1.ConditionTrue))
		Expect(filtered.Reason).To(Equal("ManagedProfile"))

		// The profile is written once, not on every pass.
		get(sandboxProfileName, profile)
		before := profile.GetResourceVersion()
		reconcile("dave")
		get(sandboxProfileName, profile)
		Expect(profile.GetResourceVersion()).To(Equal(before))

		// An older manager left it unlabeled: labeling it keeps the desktop running.
		profile.SetLabels(nil)
		Expect(k8sClient.Update(ctx, profile)).To(Succeed())
		reconcile("dave")
		get(sandboxProfileName, profile)
		Expect(profile.GetLabels()).To(HaveKeyWithValue(labelManagedBy, managedBy))
		get("dave", dave)
		Expect(meta.FindStatusCondition(dave.Status.Conditions, ConditionSeccompFiltered).Reason).To(Equal("ManagedProfile"))
	})

	It("rejects a pinned port outside the NodePort range", func() {
		bad := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: testNS},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				Image:  testImage,
				Expose: vdiv1alpha1.ExposeSpec{BasePort: 47989},
			},
		}
		Expect(k8sClient.Create(ctx, bad)).To(Succeed())
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "bad", Namespace: testNS}})
		Expect(err).To(HaveOccurred())
		get("bad", bad)
		Expect(bad.Status.Phase).To(Equal(vdiv1alpha1.PhaseFailed))
	})

	It("takes a keyboard layout only as XKB layout names", func() {
		create := func(layout string) error {
			return k8sClient.Create(ctx, &vdiv1alpha1.VdiInstance{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "layout-", Namespace: testNS},
				Spec:       vdiv1alpha1.VdiInstanceSpec{Image: testImage, Stream: vdiv1alpha1.StreamSpec{KeyboardLayout: layout}},
			}, client.DryRunAll)
		}
		Expect(create("fr")).To(Succeed())
		Expect(create("us,de,ru,ara")).To(Succeed())
		for _, bad := range []string{"us(intl)", "US", "us,", "us de", "us,de,fr,ru,ara", "us;reboot"} {
			Expect(apierrors.IsInvalid(create(bad))).To(BeTrue(), bad)
		}
	})

	It("scales to zero when running is false and keeps the home", func() {
		vdi := &vdiv1alpha1.VdiInstance{}
		get("alice", vdi)
		vdi.Spec.Running = ptr.To(false)
		Expect(k8sClient.Update(ctx, vdi)).To(Succeed())
		reconcile("alice")

		dep := &appsv1.Deployment{}
		get("alice", dep)
		Expect(*dep.Spec.Replicas).To(Equal(int32(0)))
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseStopping))

		stopPod("alice")
		reconcile("alice")
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseStopped))
		Expect(vdi.Status.LastActive).To(BeNil())
		get("alice-home", &corev1.PersistentVolumeClaim{})
	})

	It("stops a desktop that sat free for idleTimeout", func() {
		vdi := &vdiv1alpha1.VdiInstance{}
		get("alice", vdi)
		vdi.Spec.Running = ptr.To(true)
		vdi.Spec.IdleTimeout = &metav1.Duration{Duration: time.Minute}
		Expect(k8sClient.Update(ctx, vdi)).To(Succeed())
		reconcile("alice")
		startPod("alice")
		probe = &sunshine.ServerInfo{StatusCode: 200, State: "SUNSHINE_SERVER_FREE"}

		reconcile("alice")
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseReady))
		Expect(vdi.Status.LastActive).NotTo(BeNil())
		Expect(*vdi.Spec.Running).To(BeTrue())

		// A session refreshes the clock even when it started long ago.
		vdi.Status.LastActive = &metav1.Time{Time: time.Now().Add(-2 * time.Minute)}
		Expect(k8sClient.Status().Update(ctx, vdi)).To(Succeed())
		probe = &sunshine.ServerInfo{StatusCode: 200, State: "SUNSHINE_SERVER_BUSY"}
		reconcile("alice")
		get("alice", vdi)
		Expect(*vdi.Spec.Running).To(BeTrue())
		Expect(vdi.Status.LastActive.Time).To(BeTemporally("~", time.Now(), 10*time.Second))

		vdi.Status.LastActive = &metav1.Time{Time: time.Now().Add(-2 * time.Minute)}
		Expect(k8sClient.Status().Update(ctx, vdi)).To(Succeed())
		probe = &sunshine.ServerInfo{StatusCode: 200, State: "SUNSHINE_SERVER_FREE"}
		reconcile("alice")
		get("alice", vdi)
		Expect(*vdi.Spec.Running).To(BeFalse())
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseStopping))
		Expect(meta.FindStatusCondition(vdi.Status.Conditions, ConditionReady).Reason).To(Equal("IdleShutdown"))
		// The spec change queues one more reconcile, that one scales down.
		reconcile("alice")
		dep := &appsv1.Deployment{}
		get("alice", dep)
		Expect(*dep.Spec.Replicas).To(Equal(int32(0)))
	})

	Context("next to Services DeskuBe does not own", Ordered, func() {
		var raw, cacheLike client.WithWatch
		reconcileErr := func(name string) error {
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: testNS}})
			return err
		}
		desktop := func(name string, pinned int32) *vdiv1alpha1.VdiInstance {
			vdi := &vdiv1alpha1.VdiInstance{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
				Spec:       vdiv1alpha1.VdiInstanceSpec{Image: testImage, Expose: vdiv1alpha1.ExposeSpec{BasePort: pinned}},
			}
			Expect(k8sClient.Create(ctx, vdi)).To(Succeed())
			return vdi
		}

		BeforeAll(func() {
			var err error
			raw, err = client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			// The manager's cache will hold DeskuBe's objects only: foreign
			// Services must come from the API reader.
			r.APIReader = k8sClient
			cacheLike = interceptor.NewClient(raw, interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*corev1.ServiceList); ok {
						return errors.New("the cache holds no foreign Service")
					}
					return c.List(ctx, list, opts...)
				},
			})
			r.Client = cacheLike
			DeferCleanup(func() { r.Client, r.APIReader = k8sClient, nil })

			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "elsewhere"}})).To(Succeed())
			// Its node port is the control port of the first free window, 30144.
			Expect(k8sClient.Create(ctx, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "squatter", Namespace: "elsewhere"},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeNodePort,
					Selector: map[string]string{"app": "squatter"},
					Ports:    []corev1.ServicePort{{Port: 80, NodePort: 30154}},
				},
			})).To(Succeed())
		})

		It("skips a window a foreign Service holds one port of", func() {
			erin := desktop("erin", 0)
			reconcile("erin")
			get("erin", erin)
			Expect(erin.Status.BasePort).To(Equal(int32(30176)))
		})

		It("moves to another window when the Service cannot have its node ports", func() {
			frank := desktop("frank", 0)
			// As if the squatter took the port between allocation and the Service.
			frank.Status.BasePort = 30144
			Expect(k8sClient.Status().Update(ctx, frank)).To(Succeed())

			Expect(reconcileErr("frank")).To(HaveOccurred())
			get("frank", frank)
			Expect(frank.Status.BasePort).To(BeZero())
			cond := meta.FindStatusCondition(frank.Status.Conditions, ConditionNetworkReady)
			Expect(cond.Reason).To(Equal("PortConflict"))
			Expect(cond.Message).To(ContainSubstring("30154"))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "frank", Namespace: testNS}, &appsv1.Deployment{})).NotTo(Succeed())

			reconcile("frank")
			get("frank", frank)
			Expect(frank.Status.BasePort).To(Equal(int32(30208)))
			svc := &corev1.Service{}
			get("frank", svc)
			Expect(svc.Spec.Ports[0].NodePort).To(Equal(int32(30203)))

			// A cache that missed frank's Service must not cost frank its window.
			r.Client = interceptor.NewClient(raw, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*corev1.Service); ok && key.Name == "frank" {
						return apierrors.NewNotFound(corev1.Resource("services"), key.Name)
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})
			Expect(reconcileErr("frank")).To(Succeed())
			r.Client = cacheLike
			get("frank", frank)
			Expect(frank.Status.BasePort).To(Equal(int32(30208)))
			Expect(meta.FindStatusCondition(frank.Status.Conditions, ConditionNetworkReady).Reason).To(Equal("Address"))
		})

		It("refuses a pinned base whose ports another desktop or Service holds", func() {
			// Off the grid: only its http port, 30026, is alice's control port.
			gina := desktop("gina", 30026)
			Expect(reconcileErr("gina")).To(HaveOccurred())
			get("gina", gina)
			Expect(gina.Status.Phase).To(Equal(vdiv1alpha1.PhaseFailed))
			Expect(gina.Status.BasePort).To(BeZero())
			cond := meta.FindStatusCondition(gina.Status.Conditions, ConditionNetworkReady)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("PortConflict"))
			Expect(cond.Message).To(Equal("basePort 30026 needs node port 30026, held by VdiInstance vdi-test/alice"))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "gina", Namespace: testNS}, &corev1.Service{})).NotTo(Succeed())

			hank := desktop("hank", 30154)
			Expect(reconcileErr("hank")).To(HaveOccurred())
			get("hank", hank)
			cond = meta.FindStatusCondition(hank.Status.Conditions, ConditionNetworkReady)
			Expect(cond.Reason).To(Equal("PortConflict"))
			Expect(cond.Message).To(HaveSuffix("held by Service elsewhere/squatter"))
		})
	})
})
