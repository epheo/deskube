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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

type fakeSunshine struct {
	host     string
	user     string
	pass     string
	pending  []sunshine.PendingPairing
	goodPIN  string
	approved []string
}

func (f *fakeSunshine) PendingPairings(context.Context) ([]sunshine.PendingPairing, error) {
	return f.pending, nil
}

func (f *fakeSunshine) ApprovePairing(_ context.Context, id, pin, name string) (bool, error) {
	f.approved = append(f.approved, id+":"+name)
	return pin == f.goodPIN, nil
}

var _ = Describe("Pairing", Ordered, func() {
	const ns = "vdi-pairing-test"
	var (
		r    *VdiInstanceReconciler
		fake *fakeSunshine
		key  = types.NamespacedName{Name: "desk", Namespace: ns}
	)

	reconcile := func() (ctrl.Result, *vdiv1alpha1.VdiInstance) {
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		vdi := &vdiv1alpha1.VdiInstance{}
		Expect(k8sClient.Get(ctx, key, vdi)).To(Succeed())
		return result, vdi
	}
	requestPIN := func(pin, clientName string) {
		vdi := &vdiv1alpha1.VdiInstance{}
		Expect(k8sClient.Get(ctx, key, vdi)).To(Succeed())
		vdi.Spec.Pairing = &vdiv1alpha1.PairingSpec{PIN: pin, ClientName: clientName}
		Expect(k8sClient.Update(ctx, vdi)).To(Succeed())
	}

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		fake = &fakeSunshine{goodPIN: "4693", pending: []sunshine.PendingPairing{{ID: "abc", Name: "roth", Address: "100.64.0.2"}}}
		r = &VdiInstanceReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
			Probe: func(context.Context, string, int32) (*sunshine.ServerInfo, error) {
				return &sunshine.ServerInfo{StatusCode: 200, State: "SUNSHINE_SERVER_FREE"}, nil
			},
			AdminClient: func(host string, _ int32, user, pass string) PairingClient {
				fake.host, fake.user, fake.pass = host, user, pass
				return fake
			},
		}
		Expect(k8sClient.Create(ctx, &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: ns},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				Image: testImage,
				// Pinned away from the windows the instance specs expect.
				Expose: vdiv1alpha1.ExposeSpec{Host: "desk.example.org", BasePort: 32704},
			},
		})).To(Succeed())
		_, vdi := reconcile()
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseProvisioning))
		Expect(vdi.Status.Pairing).To(BeNil())
	})

	It("waits while the desktop is not ready", func() {
		requestPIN("4693", "laptop")
		result, vdi := reconcile()
		st := vdi.Status.Pairing
		Expect(st.Phase).To(Equal(vdiv1alpha1.PairingPending))
		Expect(st.PIN).To(Equal("4693"))
		Expect(st.Message).To(ContainSubstring("not ready"))
		Expect(st.RequestedAt.Time).To(BeTemporally("~", time.Now(), 10*time.Second))
		Expect(vdi.Spec.Pairing).NotTo(BeNil())
		Expect(result.RequeueAfter).To(BeNumerically("<=", requeueStarting))
		Expect(fake.approved).To(BeEmpty())
	})

	It("approves the waiting client with the admin credentials once Ready, then drops the PIN", func() {
		// envtest runs no kubelet and no deployment controller: the test plays both.
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "desk-0", Namespace: ns, Labels: map[string]string{labelInstance: key.Name}},
			Spec:       corev1.PodSpec{NodeName: "node-p", Containers: []corev1.Container{{Name: containerName, Image: testImage}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status.PodIP = "10.128.4.7"
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, dep)).To(Succeed())
		dep.Status.Replicas, dep.Status.ReadyReplicas = 1, 1
		Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())

		_, vdi := reconcile()
		Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseReady))
		st := vdi.Status.Pairing
		Expect(st.Phase).To(Equal(vdiv1alpha1.PairingPaired))
		Expect(st.PIN).To(Equal("4693"))
		Expect(st.Message).To(ContainSubstring("roth from 100.64.0.2"))
		Expect(st.PairedAt).NotTo(BeNil())
		Expect(vdi.Spec.Pairing).To(BeNil())
		Expect(fake.approved).To(ConsistOf("abc:laptop"))

		Expect(fake.host).To(Equal("10.128.4.7"))
		admin := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "desk-sunshine-admin", Namespace: ns}, admin)).To(Succeed())
		Expect(fake.user).To(Equal("admin"))
		Expect(fake.pass).To(Equal(string(admin.Data["password"])))

		// A final pairing is never replayed.
		fake.approved = nil
		_, vdi = reconcile()
		Expect(fake.approved).To(BeEmpty())
		Expect(vdi.Status.Pairing.Phase).To(Equal(vdiv1alpha1.PairingPaired))
	})

	It("reaches the admin API through the client address from outside the cluster", func() {
		r.ProbeExternal = true
		defer func() { r.ProbeExternal = false }()
		requestPIN("1111", "")
		reconcile()
		Expect(fake.host).To(Equal("desk.example.org"))
	})

	It("fails on a wrong PIN and waits when nobody is pairing", func() {
		fake.approved = nil
		requestPIN("0000", "")
		_, vdi := reconcile()
		Expect(vdi.Status.Pairing.Phase).To(Equal(vdiv1alpha1.PairingFailed))
		Expect(vdi.Status.Pairing.PIN).To(Equal("0000"))
		Expect(vdi.Spec.Pairing).To(BeNil())
		// The CRD defaults the client name.
		Expect(fake.approved).To(ConsistOf("abc:moonlight"))

		fake.pending = nil
		requestPIN("4693", "")
		result, vdi := reconcile()
		Expect(vdi.Status.Pairing.Phase).To(Equal(vdiv1alpha1.PairingPending))
		Expect(vdi.Status.Pairing.Message).To(ContainSubstring("no Moonlight client"))
		Expect(vdi.Spec.Pairing).NotTo(BeNil())
		Expect(result.RequeueAfter).To(Equal(requeuePairing))
	})

	It("gives a newer PIN its own ten minutes", func() {
		_, vdi := reconcile()
		first := vdi.Status.Pairing.RequestedAt
		vdi.Status.Pairing.RequestedAt = &metav1.Time{Time: first.Add(-5 * time.Minute)}
		Expect(k8sClient.Status().Update(ctx, vdi)).To(Succeed())

		requestPIN("1234", "")
		_, vdi = reconcile()
		Expect(vdi.Status.Pairing.PIN).To(Equal("1234"))
		Expect(vdi.Status.Pairing.RequestedAt.Time).To(BeTemporally("~", time.Now(), 10*time.Second))
	})

	It("keeps a PIN the owner wrote after the one being cleared", func() {
		stale := &vdiv1alpha1.VdiInstance{}
		Expect(k8sClient.Get(ctx, key, stale)).To(Succeed())
		requestPIN("5678", "")
		Expect(r.clearPairingRequest(ctx, stale, "1234")).To(Succeed())
		vdi := &vdiv1alpha1.VdiInstance{}
		Expect(k8sClient.Get(ctx, key, vdi)).To(Succeed())
		Expect(vdi.Spec.Pairing.PIN).To(Equal("5678"))
	})

	It("gives up ten minutes after the request", func() {
		_, vdi := reconcile()
		Expect(vdi.Status.Pairing.PIN).To(Equal("5678"))
		vdi.Status.Pairing.RequestedAt = &metav1.Time{Time: time.Now().Add(-pairingTTL - time.Second)}
		Expect(k8sClient.Status().Update(ctx, vdi)).To(Succeed())

		_, vdi = reconcile()
		Expect(vdi.Status.Pairing.Phase).To(Equal(vdiv1alpha1.PairingFailed))
		Expect(vdi.Status.Pairing.Message).To(ContainSubstring("expired"))
		Expect(vdi.Spec.Pairing).To(BeNil())
	})

	It("fails a request the owner withdrew", func() {
		requestPIN("4321", "")
		_, vdi := reconcile()
		Expect(vdi.Status.Pairing.Phase).To(Equal(vdiv1alpha1.PairingPending))

		patch := client.MergeFrom(vdi.DeepCopy())
		vdi.Spec.Pairing = nil
		Expect(k8sClient.Patch(ctx, vdi, patch)).To(Succeed())
		_, vdi = reconcile()
		Expect(vdi.Status.Pairing.Phase).To(Equal(vdiv1alpha1.PairingFailed))
		Expect(vdi.Status.Pairing.Message).To(ContainSubstring("withdrawn"))
	})
})
