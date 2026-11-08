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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

var _ = Describe("The manager's cache", Ordered, func() {
	const ns = "vdi-cache"
	var (
		cached client.Client
		r      *VdiInstanceReconciler
	)
	key := func(name string) client.ObjectKey { return client.ObjectKey{Name: name, Namespace: ns} }
	fresh := func(obj client.Object) client.Object { return obj.DeepCopyObject().(client.Object) }
	// What a desktop with an owner and nested sandboxes is made of.
	children := func() []client.Object {
		meta := func(suffix string) metav1.ObjectMeta { return metav1.ObjectMeta{Name: "erin" + suffix, Namespace: ns} }
		return []client.Object{
			&corev1.Secret{ObjectMeta: meta("-sunshine-admin")},
			&corev1.PersistentVolumeClaim{ObjectMeta: meta("-home")},
			&corev1.ServiceAccount{ObjectMeta: meta("-desktop")},
			&rbacv1.RoleBinding{ObjectMeta: meta("-desktop")},
			&appsv1.Deployment{ObjectMeta: meta("")},
			&corev1.Service{ObjectMeta: meta("")},
			&networkingv1.NetworkPolicy{ObjectMeta: meta("")},
			&rbacv1.Role{ObjectMeta: meta("-owner")},
			&rbacv1.RoleBinding{ObjectMeta: meta("-owner")},
		}
	}

	BeforeAll(func() {
		cacheCtx, stop := context.WithCancel(ctx)
		DeferCleanup(stop)
		informers, err := cache.New(cfg, cache.Options{Scheme: scheme.Scheme, ByObject: CacheByObject()})
		Expect(err).NotTo(HaveOccurred())
		go func() {
			defer GinkgoRecover()
			Expect(informers.Start(cacheCtx)).To(Succeed())
		}()
		cached, err = client.New(cfg, client.Options{Scheme: scheme.Scheme, Cache: &client.CacheOptions{Reader: informers}})
		Expect(err).NotTo(HaveOccurred())
		r = &VdiInstanceReconciler{
			Client: cached, APIReader: k8sClient, Scheme: scheme.Scheme,
			SandboxSeccompProfile: "profiles/sandbox.json",
			Probe: func(context.Context, string, int32) (*sunshine.ServerInfo, error) {
				return nil, errors.New("connection refused")
			},
		}

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		// envtest runs no service account controller.
		Expect(k8sClient.Create(ctx, &corev1.ServiceAccount{
			ObjectMeta:       metav1.ObjectMeta{Name: "default", Namespace: ns},
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry"}},
		})).To(Succeed())
		// Another spec may have made it first; whoever makes it removes it.
		scc := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: nestedSCCRole}}
		if err := k8sClient.Create(ctx, scc); err == nil {
			DeferCleanup(k8sClient.Delete, ctx, scc)
		} else {
			Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())
		}
	})

	It("holds what DeskuBe made, desktop pods and nodes, nothing else", func() {
		Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "theirs", Namespace: ns}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: "ours", Namespace: ns, Labels: map[string]string{labelManagedBy: managedBy},
		}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "theirs", Namespace: ns}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
			Name: "ours", Namespace: ns, Labels: map[string]string{labelManagedBy: managedBy},
		}})).To(Succeed())
		pod := func(name string, labels map[string]string) *corev1.Pod {
			return &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: containerName, Image: testImage}}},
			}
		}
		Expect(k8sClient.Create(ctx, pod("theirs", nil))).To(Succeed())
		Expect(k8sClient.Create(ctx, pod("desk-0", map[string]string{labelInstance: "desk"}))).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-cache"}})).To(Succeed())

		// Objects of one kind reach the cache in the order they were made.
		Eventually(func() error { return cached.Get(ctx, key("ours"), &corev1.Secret{}) }).Should(Succeed())
		Expect(apierrors.IsNotFound(cached.Get(ctx, key("theirs"), &corev1.Secret{}))).To(BeTrue())
		Eventually(func() error { return cached.Get(ctx, key("ours"), &networkingv1.NetworkPolicy{}) }).Should(Succeed())
		Expect(apierrors.IsNotFound(cached.Get(ctx, key("theirs"), &networkingv1.NetworkPolicy{}))).To(BeTrue())
		Eventually(func(g Gomega) {
			pods := &corev1.PodList{}
			g.Expect(cached.List(ctx, pods, client.InNamespace(ns))).To(Succeed())
			g.Expect(pods.Items).To(HaveLen(1))
			g.Expect(pods.Items[0].Name).To(Equal("desk-0"))
		}).Should(Succeed())
		Eventually(func() error { return cached.Get(ctx, client.ObjectKey{Name: "node-cache"}, &corev1.Node{}) }).Should(Succeed())
	})

	It("adopts the unlabeled children an older manager made, in one pass", func() {
		erin := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "erin", Namespace: ns},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				Image: testImage, Owner: "erin", NestedSandboxes: ptr.To(true),
				// The last window: the other specs count on the first ones.
				Expose: vdiv1alpha1.ExposeSpec{BasePort: 32736},
			},
		}
		Expect(k8sClient.Create(ctx, erin)).To(Succeed())
		// What the previous release left: no labels, but the instance
		// label on the claim, the Deployment and the Service.
		older := *r
		older.Client = k8sClient
		_, err := older.Reconcile(ctx, ctrl.Request{NamespacedName: key("erin")})
		Expect(err).NotTo(HaveOccurred())
		var password []byte
		for _, child := range children() {
			Expect(k8sClient.Get(ctx, key(child.GetName()), child)).To(Succeed())
			switch child.(type) {
			case *networkingv1.NetworkPolicy:
				// It made none.
				Expect(k8sClient.Delete(ctx, child)).To(Succeed())
				continue
			case *corev1.PersistentVolumeClaim, *appsv1.Deployment, *corev1.Service:
				child.SetLabels(map[string]string{labelInstance: "erin"})
			default:
				child.SetLabels(nil)
			}
			if secret, ok := child.(*corev1.Secret); ok {
				password = secret.Data["password"]
			}
			Expect(k8sClient.Update(ctx, child)).To(Succeed())
		}
		Eventually(func(g Gomega) {
			for _, child := range children() {
				err := cached.Get(ctx, key(child.GetName()), fresh(child))
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), child.GetName())
			}
		}).Should(Succeed())

		// Reconcile turns AlreadyExists into a quiet retry; reconcile shows it.
		Expect(k8sClient.Get(ctx, key("erin"), erin)).To(Succeed())
		_, err = r.reconcile(ctx, erin)
		Expect(err).NotTo(HaveOccurred())

		for _, child := range children() {
			Expect(k8sClient.Get(ctx, key(child.GetName()), child)).To(Succeed())
			Expect(child.GetLabels()).To(HaveKeyWithValue(labelManagedBy, managedBy), child.GetName())
			Expect(child.GetLabels()).To(HaveKeyWithValue(labelInstance, "erin"), child.GetName())
		}
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, key("erin-sunshine-admin"), secret)).To(Succeed())
		Expect(secret.Data["password"]).To(Equal(password))
		// Read past the cache: the namespace's default account is not ours.
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key("erin"), dep)).To(Succeed())
		Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: "registry"}))

		// The cache catches up, and from then on nothing is written.
		versions := make([]string, len(children()))
		Eventually(func(g Gomega) {
			for i, child := range children() {
				seen := fresh(child)
				g.Expect(cached.Get(ctx, key(child.GetName()), seen)).To(Succeed())
				g.Expect(k8sClient.Get(ctx, key(child.GetName()), child)).To(Succeed())
				g.Expect(seen.GetResourceVersion()).To(Equal(child.GetResourceVersion()), child.GetName())
				versions[i] = child.GetResourceVersion()
			}
		}).Should(Succeed())
		Expect(k8sClient.Get(ctx, key("erin"), erin)).To(Succeed())
		_, err = r.reconcile(ctx, erin)
		Expect(err).NotTo(HaveOccurred())
		for i, child := range children() {
			Expect(k8sClient.Get(ctx, key(child.GetName()), child)).To(Succeed())
			Expect(child.GetResourceVersion()).To(Equal(versions[i]), child.GetName())
		}
	})
})
