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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

var _ = Describe("Desktop classes", Ordered, func() {
	const ns = "vdi-class"
	key := func(name string) client.ObjectKey { return client.ObjectKey{Name: name, Namespace: ns} }
	// Pinned high, below the pairing and cache specs: others count on the first windows.
	desktop := func(name string, base int32, spec vdiv1alpha1.VdiInstanceSpec) {
		spec.Expose.BasePort = base
		Expect(k8sClient.Create(ctx, &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: spec,
		})).To(Succeed())
	}
	class := func(name string, spec vdiv1alpha1.VdiClassSpec) {
		c := &vdiv1alpha1.VdiClass{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: spec}
		Expect(k8sClient.Create(ctx, c)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, c))).To(Succeed()) })
	}
	pod := func(g Gomega, name string) corev1.PodSpec {
		dep := &appsv1.Deployment{}
		g.Expect(k8sClient.Get(ctx, key(name), dep)).To(Succeed())
		return dep.Spec.Template.Spec
	}
	container := func(g Gomega, name string) corev1.Container { return pod(g, name).Containers[0] }
	gpuNodes := &vdiv1alpha1.Placement{
		NodeSelector: map[string]string{"gpu": "yes"},
		Tolerations:  []corev1.Toleration{{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}},
		Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a", "b"}}},
			}}},
		}},
		PriorityClassName: "desktops",
		RuntimeClassName:  ptr.To("gpu"),
	}
	env := func(c corev1.Container) map[string]string {
		vars := map[string]string{}
		for _, e := range c.Env {
			vars[e.Name] = e.Value
		}
		return vars
	}
	webPublished := func(g Gomega, name string) bool {
		svc := &corev1.Service{}
		g.Expect(k8sClient.Get(ctx, key(name), svc)).To(Succeed())
		for _, p := range svc.Spec.Ports {
			if p.Name == "web" {
				return true
			}
		}
		return false
	}
	webManagerOnly := func(g Gomega, name string) bool {
		policy := &networkingv1.NetworkPolicy{}
		g.Expect(k8sClient.Get(ctx, key(name), policy)).To(Succeed())
		for _, rule := range policy.Spec.Ingress {
			if len(rule.From) > 0 {
				return true
			}
		}
		return false
	}
	home := func(g Gomega, name string) corev1.PersistentVolumeClaimSpec {
		pvc := &corev1.PersistentVolumeClaim{}
		g.Expect(k8sClient.Get(ctx, key(name+"-home"), pvc)).To(Succeed())
		return pvc.Spec
	}
	version := func(name string) string {
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key(name), dep)).To(Succeed())
		return dep.ResourceVersion
	}

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		// A running manager, so the class watch and its index are the ones in use.
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
			// The desktops of other specs are theirs to reconcile.
			Cache:   cache.Options{ByObject: CacheByObject(), DefaultNamespaces: map[string]cache.Config{ns: {}}},
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())
		r := &VdiInstanceReconciler{
			Client: mgr.GetClient(), APIReader: mgr.GetAPIReader(), Scheme: mgr.GetScheme(),
			Probe: func(context.Context, string, int32) (*sunshine.ServerInfo, error) {
				return nil, errors.New("connection refused")
			},
		}
		Expect(r.SetupWithManager(mgr)).To(Succeed())
		mgrCtx, stop := context.WithCancel(ctx)
		stopped := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(stopped)
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		DeferCleanup(func() {
			stop()
			<-stopped
		})

		class("office", vdiv1alpha1.VdiClassSpec{
			Image: "registry.example/office:1",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("2Gi")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
			},
			Home:      vdiv1alpha1.HomeSpec{Size: resource.MustParse("10Gi"), StorageClassName: ptr.To("classy")},
			Expose:    vdiv1alpha1.ClassExposeSpec{WebUI: ptr.To(true)},
			Stream:    vdiv1alpha1.StreamSpec{KeyboardLayout: "de"},
			Placement: gpuNodes,
		})
	})

	It("refuses an instance with neither image nor className, and a class without an image", func() {
		err := k8sClient.Create(ctx, &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "nothing", Namespace: ns},
			Spec:       vdiv1alpha1.VdiInstanceSpec{Owner: "alice"},
		}, client.DryRunAll)
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("needs image or className")))

		bare := &unstructured.Unstructured{}
		bare.SetGroupVersionKind(vdiv1alpha1.GroupVersion.WithKind("VdiInstance"))
		bare.SetName("bare")
		bare.SetNamespace(ns)
		Expect(apierrors.IsInvalid(k8sClient.Create(ctx, bare, client.DryRunAll))).To(BeTrue())

		err = k8sClient.Create(ctx, &vdiv1alpha1.VdiClass{ObjectMeta: metav1.ObjectMeta{Name: "imageless"}}, client.DryRunAll)
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
	})

	It("accepts the samples", func() {
		samples := filepath.Join("..", "..", "config", "samples")
		sampleClass := &vdiv1alpha1.VdiClass{}
		Expect(readManifest(filepath.Join(samples, "vdi_v1alpha1_vdiclass.yaml"), sampleClass)).To(Succeed())
		Expect(k8sClient.Create(ctx, sampleClass, client.DryRunAll)).To(Succeed())
		sample := &vdiv1alpha1.VdiInstance{}
		Expect(readManifest(filepath.Join(samples, "vdi_v1alpha1_vdiinstance.yaml"), sample)).To(Succeed())
		Expect(sample.Spec.ClassName).To(Equal(sampleClass.Name))
		sample.Namespace = ns
		Expect(k8sClient.Create(ctx, sample, client.DryRunAll)).To(Succeed())
	})

	It("gives an instance that names only a class the class's settings", func() {
		desktop("office-a", 32672, vdiv1alpha1.VdiInstanceSpec{ClassName: "office"})
		desktop("office-b", 32640, vdiv1alpha1.VdiInstanceSpec{ClassName: "office"})
		for _, name := range []string{"office-a", "office-b"} {
			Eventually(func(g Gomega) {
				c := container(g, name)
				g.Expect(c.Image).To(Equal("registry.example/office:1"))
				g.Expect(c.Resources.Requests.Cpu().String()).To(Equal("1"))
				g.Expect(c.Resources.Requests.Memory().String()).To(Equal("2Gi"))
				g.Expect(c.Resources.Limits.Memory().String()).To(Equal("2Gi"))
				g.Expect(env(c)).To(HaveKeyWithValue("DESKUBE_KEYBOARD_LAYOUT", "de"))
				g.Expect(env(c)).To(HaveKeyWithValue("DESKUBE_WEB_UI_ALLOWED", "wan"))
				g.Expect(webPublished(g, name)).To(BeTrue())
				g.Expect(webManagerOnly(g, name)).To(BeFalse())
				pvc := home(g, name)
				g.Expect(pvc.Resources.Requests.Storage().String()).To(Equal("10Gi"))
				g.Expect(pvc.StorageClassName).To(HaveValue(Equal("classy")))
				p := pod(g, name)
				g.Expect(p.NodeSelector).To(Equal(gpuNodes.NodeSelector))
				g.Expect(p.Tolerations).To(Equal(gpuNodes.Tolerations))
				g.Expect(p.Affinity).To(Equal(gpuNodes.Affinity))
				g.Expect(p.PriorityClassName).To(Equal("desktops"))
				g.Expect(p.RuntimeClassName).To(HaveValue(Equal("gpu")))
			}, "10s", "100ms").Should(Succeed(), name)
		}
		vdi := &vdiv1alpha1.VdiInstance{}
		Expect(k8sClient.Get(ctx, key("office-a"), vdi)).To(Succeed())
		Expect(vdi.Spec.Image).To(BeEmpty())
	})

	It("exposes a desktop the way its class says", func() {
		class("remote", vdiv1alpha1.VdiClassSpec{
			Image:  "registry.example/remote:1",
			Expose: vdiv1alpha1.ClassExposeSpec{Type: vdiv1alpha1.ExposeLoadBalancer},
		})
		desktop("remote-a", 0, vdiv1alpha1.VdiInstanceSpec{ClassName: "remote"})
		Eventually(func(g Gomega) {
			svc := &corev1.Service{}
			g.Expect(k8sClient.Get(ctx, key("remote-a"), svc)).To(Succeed())
			g.Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
			g.Expect(svc.Spec.AllocateLoadBalancerNodePorts).To(HaveValue(BeFalse()))
			vdi := &vdiv1alpha1.VdiInstance{}
			g.Expect(k8sClient.Get(ctx, key("remote-a"), vdi)).To(Succeed())
			g.Expect(vdi.Status.BasePort).To(Equal(int32(loadBalancerBasePort)))
		}, "10s", "100ms").Should(Succeed())
	})

	It("lets a field the instance sets win over the class's", func() {
		desktop("own", 32608, vdiv1alpha1.VdiInstanceSpec{
			ClassName: "office",
			Image:     "registry.example/own:1",
			// Taken whole: no memory from the class.
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("3")},
			},
			// Merged by field: the storage class stays the class's.
			Home:   vdiv1alpha1.HomeSpec{Size: resource.MustParse("5Gi")},
			Expose: vdiv1alpha1.ExposeSpec{ClassExposeSpec: vdiv1alpha1.ClassExposeSpec{WebUI: ptr.To(false)}},
			Stream: vdiv1alpha1.StreamSpec{KeyboardLayout: "fr"},
			// Whole by field: no gpu key, the class's tolerations.
			Placement: &vdiv1alpha1.Placement{NodeSelector: map[string]string{"zone": "a"}},
		})
		Eventually(func(g Gomega) {
			c := container(g, "own")
			g.Expect(c.Image).To(Equal("registry.example/own:1"))
			g.Expect(c.Resources.Requests).To(HaveLen(1))
			g.Expect(c.Resources.Requests.Cpu().String()).To(Equal("3"))
			g.Expect(c.Resources.Limits).To(BeEmpty())
			g.Expect(env(c)).To(HaveKeyWithValue("DESKUBE_KEYBOARD_LAYOUT", "fr"))
			g.Expect(env(c)).To(HaveKeyWithValue("DESKUBE_WEB_UI_ALLOWED", "lan"))
			g.Expect(webPublished(g, "own")).To(BeFalse())
			g.Expect(webManagerOnly(g, "own")).To(BeTrue())
			pvc := home(g, "own")
			g.Expect(pvc.Resources.Requests.Storage().String()).To(Equal("5Gi"))
			g.Expect(pvc.StorageClassName).To(HaveValue(Equal("classy")))
			p := pod(g, "own")
			g.Expect(p.NodeSelector).To(Equal(map[string]string{"zone": "a"}))
			g.Expect(p.Tolerations).To(Equal(gpuNodes.Tolerations))
			g.Expect(p.PriorityClassName).To(Equal("desktops"))
		}, "10s", "100ms").Should(Succeed())
	})

	It("keeps a desktop whose class is missing from starting, and starts it once the class exists", func() {
		desktop("waiting", 32576, vdiv1alpha1.VdiInstanceSpec{ClassName: "later"})
		vdi := &vdiv1alpha1.VdiInstance{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, key("waiting"), vdi)).To(Succeed())
			g.Expect(vdi.Status.Phase).To(Equal(vdiv1alpha1.PhaseProvisioning))
			ready := meta.FindStatusCondition(vdi.Status.Conditions, ConditionReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Reason).To(Equal("ClassNotFound"))
			g.Expect(ready.Message).To(ContainSubstring("later"))
		}, "10s", "100ms").Should(Succeed())
		Expect(vdi.Status.BasePort).To(BeZero())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, key("waiting"), &appsv1.Deployment{}))).To(BeTrue())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, key("waiting"), &corev1.Service{}))).To(BeTrue())

		// Nothing requeues a desktop without its class: the class watch wakes it.
		class("later", vdiv1alpha1.VdiClassSpec{Image: "registry.example/later:1"})
		Eventually(func(g Gomega) {
			g.Expect(container(g, "waiting").Image).To(Equal("registry.example/later:1"))
		}, "10s", "100ms").Should(Succeed())
		Expect(k8sClient.Get(ctx, key("waiting"), vdi)).To(Succeed())
		Expect(vdi.Status.BasePort).To(Equal(int32(32576)))
	})

	It("carries a class change to every desktop that takes it, and to no other", func() {
		desktop("plain", 32544, vdiv1alpha1.VdiInstanceSpec{Image: testImage})
		Eventually(func(g Gomega) { g.Expect(container(g, "plain").Image).To(Equal(testImage)) }, "10s", "100ms").Should(Succeed())
		plain := pod(Default, "plain")
		Expect(plain.NodeSelector).To(BeNil())
		Expect(plain.Tolerations).To(BeNil())
		Expect(plain.Affinity).To(BeNil())
		Expect(plain.PriorityClassName).To(BeEmpty())
		Expect(plain.RuntimeClassName).To(BeNil())
		untouched := map[string]string{}
		for _, name := range []string{"own", "waiting", "plain"} {
			untouched[name] = version(name)
		}

		office := &vdiv1alpha1.VdiClass{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "office"}, office)).To(Succeed())
		office.Spec.Image = "registry.example/office:2"
		Expect(k8sClient.Update(ctx, office)).To(Succeed())

		for _, name := range []string{"office-a", "office-b"} {
			Eventually(func(g Gomega) {
				g.Expect(container(g, name).Image).To(Equal("registry.example/office:2"))
			}, "10s", "100ms").Should(Succeed(), name)
		}
		Consistently(func(g Gomega) {
			for name, rv := range untouched {
				dep := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, key(name), dep)).To(Succeed())
				g.Expect(dep.ResourceVersion).To(Equal(rv), name)
			}
		}, "2s", "200ms").Should(Succeed())
	})
})
