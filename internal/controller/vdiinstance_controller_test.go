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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	"github.com/deskube/deskube/internal/sunshine"
)

const (
	testNS    = "vdi-test"
	testImage = "registry.example/desktop:1"
)

var _ = Describe("VdiInstance controller", Ordered, func() {
	var (
		r     *VdiInstanceReconciler
		probe *sunshine.ServerInfo
	)

	reconcile := func(name string) {
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: testNS}})
		Expect(err).NotTo(HaveOccurred())
	}
	get := func(name string, obj client.Object) {
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNS}, obj)).To(Succeed())
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
		dep := &appsv1.Deployment{}
		get(name, dep)
		dep.Status.Replicas, dep.Status.ReadyReplicas = 1, 1
		Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
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
			Client: k8sClient,
			Scheme: scheme.Scheme,
			Probe: func(context.Context, string, int32) (*sunshine.ServerInfo, error) {
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
		Expect(dep.Spec.Template.Labels).To(HaveKeyWithValue(labelInstance, "alice"))
		pod := dep.Spec.Template.Spec
		Expect(*pod.AutomountServiceAccountToken).To(BeFalse())
		Expect(*pod.SecurityContext.RunAsNonRoot).To(BeTrue())
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
		Expect(env["DESKUBE_ADMIN_PASSWORD"].ValueFrom.SecretKeyRef.Name).To(Equal("alice-sunshine-admin"))
		Expect(env["DESKUBE_HOST_IP"].ValueFrom.FieldRef.FieldPath).To(Equal("status.hostIP"))
		Expect(env).NotTo(HaveKey("DESKUBE_ENCODER"))
		Expect(env).NotTo(HaveKey("DESKUBE_WEB_ORIGINS"))

		svc := &corev1.Service{}
		get("alice", svc)
		Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
		Expect(svc.Spec.Selector).To(HaveKeyWithValue(labelInstance, "alice"))
		Expect(svc.Spec.Ports).To(HaveLen(7))
		for _, p := range svc.Spec.Ports {
			Expect(p.NodePort).To(Equal(p.Port))
		}

		role := &rbacv1.Role{}
		get("alice-owner", role)
		Expect(role.Rules[0].ResourceNames).To(BeEmpty())
		Expect(role.Rules[1].ResourceNames).To(ConsistOf("alice"))
		Expect(role.Rules[1].Verbs).To(ConsistOf("update", "patch"))
		binding := &rbacv1.RoleBinding{}
		get("alice-owner", binding)
		Expect(binding.Subjects[0].Name).To(Equal("alice"))
	})

	It("writes nothing when nothing changed", func() {
		dep := &appsv1.Deployment{}
		get("alice", dep)
		before := dep.ResourceVersion
		reconcile("alice")
		get("alice", dep)
		Expect(dep.ResourceVersion).To(Equal(before))
		Expect(dep.Generation).To(Equal(int64(1)))
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
})
