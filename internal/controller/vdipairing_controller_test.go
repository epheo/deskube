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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

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

var _ = Describe("VdiPairing controller", Ordered, func() {
	const ns = "vdi-pairing-test"
	var (
		r    *VdiPairingReconciler
		fake *fakeSunshine
	)

	reconcile := func(name string) *vdiv1alpha1.VdiPairing {
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}})
		Expect(err).NotTo(HaveOccurred())
		p := &vdiv1alpha1.VdiPairing{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, p)).To(Succeed())
		return p
	}

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		fake = &fakeSunshine{goodPIN: "4693", pending: []sunshine.PendingPairing{{ID: "abc", Name: "roth", Address: "100.64.0.2"}}}
		r = &VdiPairingReconciler{
			Client:        k8sClient,
			Scheme:        scheme.Scheme,
			ProbeExternal: true,
			NewClient: func(host string, _ int32, user, pass string) PairingClient {
				fake.host, fake.user, fake.pass = host, user, pass
				return fake
			},
		}
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "desk-sunshine-admin", Namespace: ns},
			StringData: map[string]string{"username": "admin", "password": "s3cret"},
		})).To(Succeed())
		vdi := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "desk", Namespace: ns},
			Spec:       vdiv1alpha1.VdiInstanceSpec{Image: "registry.example/desktop:1"},
		}
		Expect(k8sClient.Create(ctx, vdi)).To(Succeed())
		vdi.Status = vdiv1alpha1.VdiInstanceStatus{Phase: vdiv1alpha1.PhaseStarting, BasePort: 31989, Address: "203.0.113.10:31989", AdminSecret: "desk-sunshine-admin"}
		Expect(k8sClient.Status().Update(ctx, vdi)).To(Succeed())
	})

	It("waits while the instance is not ready", func() {
		Expect(k8sClient.Create(ctx, &vdiv1alpha1.VdiPairing{
			ObjectMeta: metav1.ObjectMeta{Name: "laptop", Namespace: ns},
			Spec:       vdiv1alpha1.VdiPairingSpec{InstanceRef: "desk", PIN: "4693", ClientName: "laptop"},
		})).To(Succeed())
		p := reconcile("laptop")
		Expect(p.Status.Phase).To(Equal(vdiv1alpha1.PairingPending))
		Expect(p.Status.Message).To(ContainSubstring("not ready"))
		Expect(fake.approved).To(BeEmpty())
	})

	It("approves the pending pairing with the admin credentials", func() {
		vdi := &vdiv1alpha1.VdiInstance{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "desk", Namespace: ns}, vdi)).To(Succeed())
		vdi.Status.Phase = vdiv1alpha1.PhaseReady
		Expect(k8sClient.Status().Update(ctx, vdi)).To(Succeed())

		p := reconcile("laptop")
		Expect(p.Status.Phase).To(Equal(vdiv1alpha1.PairingPaired))
		Expect(p.Status.Message).To(ContainSubstring("roth from 100.64.0.2"))
		Expect(p.Status.PairedAt).NotTo(BeNil())
		Expect(fake.approved).To(ConsistOf("abc:laptop"))
		Expect(fake.host).To(Equal("203.0.113.10"))
		Expect(fake.user).To(Equal("admin"))
		Expect(fake.pass).To(Equal("s3cret"))

		// A finished pairing is never replayed.
		fake.approved = nil
		reconcile("laptop")
		Expect(fake.approved).To(BeEmpty())
	})

	It("fails on a wrong PIN and waits when nobody is pairing", func() {
		Expect(k8sClient.Create(ctx, &vdiv1alpha1.VdiPairing{
			ObjectMeta: metav1.ObjectMeta{Name: "typo", Namespace: ns},
			Spec:       vdiv1alpha1.VdiPairingSpec{InstanceRef: "desk", PIN: "0000"},
		})).To(Succeed())
		p := reconcile("typo")
		Expect(p.Status.Phase).To(Equal(vdiv1alpha1.PairingFailed))
		Expect(fake.approved).To(ConsistOf("abc:moonlight"))

		fake.pending = nil
		Expect(k8sClient.Create(ctx, &vdiv1alpha1.VdiPairing{
			ObjectMeta: metav1.ObjectMeta{Name: "early", Namespace: ns},
			Spec:       vdiv1alpha1.VdiPairingSpec{InstanceRef: "desk", PIN: "4693"},
		})).To(Succeed())
		p = reconcile("early")
		Expect(p.Status.Phase).To(Equal(vdiv1alpha1.PairingPending))
		Expect(p.Status.Message).To(ContainSubstring("no Moonlight client"))
	})
})
