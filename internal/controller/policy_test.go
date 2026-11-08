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
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
)

var (
	policyFile        = filepath.Join("..", "..", "config", "policy", "vdiinstance_owner_fields.yaml")
	policyBindingFile = filepath.Join("..", "..", "config", "policy", "vdiinstance_owner_fields_binding.yaml")
	editorRoleFile    = filepath.Join("..", "..", "config", "rbac", "vdiinstance_editor_role.yaml")

	// The spec fields an owner may change on their own desktop.
	ownerMutable = map[string]bool{"running": true, "pairing": true}

	specCompare       = regexp.MustCompile(`^object\.spec\.\?(\w+) == oldObject\.spec\.\?(\w+)$`)
	finalizersCompare = "object.metadata.?finalizers == oldObject.metadata.?finalizers"
)

func readManifest(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, into)
}

// A spec field added later stays out of the owners' reach only if the
// policy compares it; this fails until it does, or it is declared mutable.
func TestOwnerPolicyCoversEverySpecField(t *testing.T) {
	policy := &admissionregistrationv1.ValidatingAdmissionPolicy{}
	if err := readManifest(policyFile, policy); err != nil {
		t.Fatal(err)
	}
	if fp := policy.Spec.FailurePolicy; fp == nil || *fp != admissionregistrationv1.Fail {
		t.Errorf("failurePolicy %v, want Fail", fp)
	}

	compared := map[string]bool{}
	finalizers := false
	for _, v := range policy.Spec.Validations {
		m := specCompare.FindStringSubmatch(v.Expression)
		switch {
		case m != nil && m[1] == m[2]:
			compared[m[1]] = true
		case v.Expression == finalizersCompare:
			finalizers = true
		default:
			t.Errorf("validation %q has a shape this test cannot vouch for", v.Expression)
		}
	}
	if !finalizers {
		t.Error("the policy does not hold metadata.finalizers")
	}

	fields := map[string]bool{}
	spec := reflect.TypeFor[vdiv1alpha1.VdiInstanceSpec]()
	for i := range spec.NumField() {
		name, _, _ := strings.Cut(spec.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			t.Fatalf("field %s has no JSON name of its own", spec.Field(i).Name)
		}
		fields[name] = true
		switch {
		case ownerMutable[name] && compared[name]:
			t.Errorf("spec.%s is owner mutable, yet the policy holds it", name)
		case !ownerMutable[name] && !compared[name]:
			t.Errorf("spec.%s is neither owner mutable nor compared by the policy", name)
		}
	}
	for name := range compared {
		if !fields[name] {
			t.Errorf("the policy compares spec.%s, which VdiInstanceSpec lacks", name)
		}
	}
	for name := range ownerMutable {
		if !fields[name] {
			t.Errorf("spec.%s is owner mutable, but VdiInstanceSpec lacks it", name)
		}
	}
}

var _ = Describe("Owner policy", Ordered, func() {
	const ns = "vdi-policy-test"
	var owner, admin client.Client

	userClient := func(name string) client.Client {
		u, err := testEnv.AddUser(envtest.User{Name: name}, nil)
		Expect(err).NotTo(HaveOccurred())
		c, err := client.New(u.Config(), client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		return c
	}
	create := func(obj client.Object) {
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).To(Succeed()) })
	}
	mergePatch := func(body map[string]any) client.Patch {
		data, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		return client.RawPatch(types.MergePatchType, data)
	}
	desk := func(name string) *vdiv1alpha1.VdiInstance {
		return &vdiv1alpha1.VdiInstance{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	}
	patch := func(c client.Client, name string, body map[string]any) error {
		return c.Patch(ctx, desk(name), mergePatch(body))
	}
	spec := func(fields map[string]any) map[string]any { return map[string]any{"spec": fields} }
	byPolicy := func(field string) OmegaMatcher {
		return SatisfyAll(
			WithTransform(apierrors.IsForbidden, BeTrue()),
			MatchError(ContainSubstring("ValidatingAdmissionPolicy")),
			MatchError(ContainSubstring("only an admin may change "+field)),
		)
	}
	stored := func(name string) *vdiv1alpha1.VdiInstance {
		vdi := &vdiv1alpha1.VdiInstance{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, vdi)).To(Succeed())
		return vdi
	}

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())

		policy := &admissionregistrationv1.ValidatingAdmissionPolicy{}
		Expect(readManifest(policyFile, policy)).To(Succeed())
		create(policy)
		binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
		Expect(readManifest(policyBindingFile, binding)).To(Succeed())
		create(binding)

		// An admin here is whoever holds the shipped editor role.
		editor := &rbacv1.ClusterRole{}
		Expect(readManifest(editorRoleFile, editor)).To(Succeed())
		create(editor)
		create(&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "desktop-admins", Namespace: ns},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "dana"}},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: editor.Name},
		})

		// The owner Role is the one the reconciler hands out.
		r := &VdiInstanceReconciler{Client: k8sClient, Scheme: scheme.Scheme}
		for _, o := range []string{"alice", "bob"} {
			vdi := &vdiv1alpha1.VdiInstance{
				ObjectMeta: metav1.ObjectMeta{Name: o + "-desk", Namespace: ns},
				Spec:       vdiv1alpha1.VdiInstanceSpec{Image: testImage, Owner: o},
			}
			Expect(k8sClient.Create(ctx, vdi)).To(Succeed())
			settings := resolve(vdi, nil)
			Expect(r.ensureOwnerRBAC(ctx, vdi, &settings)).To(Succeed())
		}
		owner, admin = userClient("alice"), userClient("dana")

		// A policy takes effect a moment after it is created.
		Eventually(func() error {
			image := mergePatch(spec(map[string]any{"image": "evil.example/x:1"}))
			return owner.Patch(ctx, desk("alice-desk"), image, client.DryRunAll)
		}, "20s", "200ms").Should(byPolicy("spec.image"))
	})

	It("lets an owner start, stop and pair their own desktop", func() {
		Expect(patch(owner, "alice-desk", spec(map[string]any{"running": false}))).To(Succeed())
		Expect(patch(owner, "alice-desk", spec(map[string]any{"pairing": map[string]any{"pin": "4693"}}))).To(Succeed())
		vdi := stored("alice-desk")
		Expect(*vdi.Spec.Running).To(BeFalse())
		Expect(vdi.Spec.Pairing.PIN).To(Equal("4693"))

		// A full update passes as long as only those fields move.
		vdi.Spec.Running = nil
		vdi.Spec.Pairing = nil
		Expect(owner.Update(ctx, vdi)).To(Succeed())
		Expect(*stored("alice-desk").Spec.Running).To(BeTrue())
	})

	It("keeps every other field and the finalizers to admins", func() {
		Expect(patch(owner, "alice-desk", spec(map[string]any{"image": "evil.example/x:1"}))).To(byPolicy("spec.image"))
		Expect(patch(owner, "alice-desk", spec(map[string]any{"nestedSandboxes": true}))).To(byPolicy("spec.nestedSandboxes"))
		Expect(patch(owner, "alice-desk", spec(map[string]any{"owner": "mallory"}))).To(byPolicy("spec.owner"))
		// A class carries an image and sandboxes of its own.
		Expect(patch(owner, "alice-desk", spec(map[string]any{"className": "steam"}))).To(byPolicy("spec.className"))
		// Placement could move a desktop onto nodes kept for others, or preempt their pods.
		gpu := spec(map[string]any{"placement": map[string]any{"nodeSelector": map[string]any{"gpu": "yes"}}})
		Expect(patch(owner, "alice-desk", gpu)).To(byPolicy("spec.placement"))
		// Publishing the web UI would also hand the owner its admin password.
		webUI := spec(map[string]any{"expose": map[string]any{"webUI": true}})
		Expect(patch(owner, "alice-desk", webUI)).To(byPolicy("spec.expose"))
		finalizer := map[string]any{"metadata": map[string]any{"finalizers": []string{"example.com/keep"}}}
		Expect(patch(owner, "alice-desk", finalizer)).To(byPolicy("metadata.finalizers"))

		vdi := stored("alice-desk")
		Expect(vdi.Spec.Image).To(Equal(testImage))
		Expect(vdi.Spec.ClassName).To(BeEmpty())
		Expect(vdi.Spec.Placement).To(BeNil())
		Expect(vdi.Spec.NestedSandboxes).To(BeNil())
		Expect(vdi.Spec.Expose.WebUI).To(BeNil())
		Expect(vdi.Finalizers).To(BeEmpty())
	})

	It("keeps an owner off another owner's desktop", func() {
		err := patch(owner, "bob-desk", spec(map[string]any{"running": false}))
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring(`cannot patch resource "vdiinstances"`)))
		err = owner.Get(ctx, types.NamespacedName{Name: "bob-desk-sunshine-admin", Namespace: ns}, &corev1.Secret{})
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
		Expect(*stored("bob-desk").Spec.Running).To(BeTrue())
	})

	It("leaves admins free to change anything", func() {
		Expect(patch(admin, "alice-desk", spec(map[string]any{"image": "registry.example/desktop:2", "nestedSandboxes": true}))).To(Succeed())
		vdi := stored("alice-desk")
		Expect(vdi.Spec.Image).To(Equal("registry.example/desktop:2"))
		Expect(*vdi.Spec.NestedSandboxes).To(BeTrue())
	})
})
