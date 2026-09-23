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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	"github.com/deskube/deskube/internal/sunshine"
)

const testNS = "vdi-test"

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
				Image:             vdiv1alpha1.ImageSource{DataSource: vdiv1alpha1.DataSourceRef{Name: "golden"}},
				Owner:             "alice",
				SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA alice"},
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

		ci := &corev1.Secret{}
		get("alice-cloudinit", ci)
		Expect(string(ci.Data["userdata"])).To(ContainSubstring("port = 30016"))
		Expect(string(ci.Data["userdata"])).To(ContainSubstring(string(admin.Data["password"])))
		Expect(string(ci.Data["userdata"])).To(ContainSubstring("ssh-ed25519 AAAA alice"))

		vm := &kubevirtv1.VirtualMachine{}
		get("alice", vm)
		Expect(*vm.Spec.RunStrategy).To(Equal(kubevirtv1.RunStrategyAlways))
		Expect(vm.Spec.Instancetype.Name).To(Equal("u1.xlarge"))
		Expect(vm.Spec.DataVolumeTemplates).To(HaveLen(1))
		Expect(vm.Spec.DataVolumeTemplates[0].Spec.SourceRef.Name).To(Equal("golden"))
		Expect(*vm.Spec.DataVolumeTemplates[0].Spec.SourceRef.Namespace).To(Equal(testNS))
		Expect(vm.Spec.DataVolumeTemplates[0].Annotations).To(HaveKeyWithValue("cdi.kubevirt.io/cloneType", "snapshot"))
		Expect(vm.Spec.Template.Spec.Volumes[1].CloudInitNoCloud.UserDataSecretRef.Name).To(Equal("alice-cloudinit"))
		Expect(vm.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue(labelInstance, "alice"))

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
		Expect(role.Rules[0].ResourceNames).To(ConsistOf("alice"))
		binding := &rbacv1.RoleBinding{}
		get("alice-owner", binding)
		Expect(binding.Subjects[0].Name).To(Equal("alice"))
	})

	It("reports Ready once the VM runs and Sunshine answers", func() {
		vm := &kubevirtv1.VirtualMachine{}
		get("alice", vm)
		vm.Status.Ready = true
		vm.Status.PrintableStatus = kubevirtv1.VirtualMachineStatusRunning
		Expect(k8sClient.Status().Update(ctx, vm)).To(Succeed())

		reconcile("alice")
		vdi := &vdiv1alpha1.VdiInstance{}
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseStarting))
		Expect(meta.IsStatusConditionTrue(vdi.Status.Conditions, ConditionVMReady)).To(BeTrue())
		Expect(meta.IsStatusConditionFalse(vdi.Status.Conditions, ConditionStreamingReady)).To(BeTrue())

		probe = &sunshine.ServerInfo{StatusCode: 200, State: "SUNSHINE_SERVER_BUSY"}
		reconcile("alice")
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseReady))
		Expect(vdi.Status.SessionState).To(Equal("busy"))
		Expect(meta.IsStatusConditionTrue(vdi.Status.Conditions, ConditionReady)).To(BeTrue())
	})

	It("gives the next instance the next window", func() {
		bob := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "bob", Namespace: testNS},
			Spec:       vdiv1alpha1.VdiInstanceSpec{Image: vdiv1alpha1.ImageSource{DataSource: vdiv1alpha1.DataSourceRef{Name: "golden"}}},
		}
		Expect(k8sClient.Create(ctx, bob)).To(Succeed())
		reconcile("bob")
		get("bob", bob)
		Expect(bob.Status.BasePort).To(Equal(int32(30048)))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "bob-owner", Namespace: testNS}, &rbacv1.Role{})).NotTo(Succeed())
	})

	It("rejects a pinned port outside the NodePort range", func() {
		bad := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: testNS},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				Image:  vdiv1alpha1.ImageSource{DataSource: vdiv1alpha1.DataSourceRef{Name: "golden"}},
				Expose: vdiv1alpha1.ExposeSpec{BasePort: 47989},
			},
		}
		Expect(k8sClient.Create(ctx, bad)).To(Succeed())
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "bad", Namespace: testNS}})
		Expect(err).To(HaveOccurred())
		get("bad", bad)
		Expect(bad.Status.Phase).To(Equal(vdiv1alpha1.PhaseFailed))
	})

	It("halts the VM when running is false", func() {
		vdi := &vdiv1alpha1.VdiInstance{}
		get("alice", vdi)
		vdi.Spec.Running = ptr.To(false)
		Expect(k8sClient.Update(ctx, vdi)).To(Succeed())
		reconcile("alice")

		vm := &kubevirtv1.VirtualMachine{}
		get("alice", vm)
		Expect(*vm.Spec.RunStrategy).To(Equal(kubevirtv1.RunStrategyHalted))
		vm.Status.Ready = false
		vm.Status.PrintableStatus = kubevirtv1.VirtualMachineStatusStopped
		Expect(k8sClient.Status().Update(ctx, vm)).To(Succeed())
		reconcile("alice")
		get("alice", vdi)
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseStopped))
	})
})
