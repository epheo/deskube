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
	"crypto/sha256"
	_ "embed"
	"encoding/hex"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
)

// The default seccomp profile of podman on Fedora, resolved for a
// container without capabilities by hack/seccomp/flatten.py. It allows
// the nested namespaces a sandbox needs and still refuses what a
// desktop has no use for.
//
//go:embed sandbox_seccomp.json
var sandboxSeccompSpec []byte

const (
	sandboxProfileName = "deskube-sandbox"
	profileHashKey     = "vdi.desku.be/profile-hash"

	// Reasons on the SeccompFiltered condition.
	seccompRuntimeDefault = "RuntimeDefault"
	seccompManaged        = "ManagedProfile"
	seccompNode           = "NodeProfile"
	seccompPending        = "ProfilePending"
	seccompNone           = "Unconfined"
)

// The Security Profiles Operator's resource. It is read and written as
// unstructured data, so that operator stays optional.
var seccompProfileGVK = schema.GroupVersionKind{
	Group: "security-profiles-operator.x-k8s.io", Version: "v1beta1", Kind: "SeccompProfile",
}

// +kubebuilder:rbac:groups=security-profiles-operator.x-k8s.io,resources=seccompprofiles,verbs=get;create;update;patch

// seccompChoice is what a desktop's pod runs under, and why.
type seccompChoice struct {
	Profile *corev1.SeccompProfile
	Reason  string
	Message string
}

// chooseSeccomp picks the tightest filter a desktop can run under.
// A plain desktop takes the runtime's default. A sandbox desktop cannot,
// that filter refuses nested namespaces: it gets the profile named on
// the command line, else one the Security Profiles Operator installs
// for us, else none. A nil Profile means "not yet, do not start".
func (r *VdiInstanceReconciler) chooseSeccomp(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance,
) (seccompChoice, error) {
	if !vdi.Spec.NestedSandboxes {
		return seccompChoice{
			Profile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			Reason:  seccompRuntimeDefault,
		}, nil
	}
	if r.SandboxSeccompProfile != "" {
		return localhost(r.SandboxSeccompProfile, seccompNode), nil
	}
	path, installed, err := r.ensureManagedProfile(ctx, vdi.Namespace)
	switch {
	case meta.IsNoMatchError(err):
		return seccompChoice{
			Profile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
			Reason:  seccompNone,
			Message: "no seccomp filter: install the Security Profiles Operator, " +
				"or start the manager with --sandbox-seccomp-profile",
		}, nil
	case err != nil:
		return seccompChoice{}, err
	case !installed:
		return seccompChoice{Reason: seccompPending, Message: "waiting for the seccomp profile to reach the nodes"}, nil
	}
	return localhost(path, seccompManaged), nil
}

func localhost(path, reason string) seccompChoice {
	return seccompChoice{
		Profile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost, LocalhostProfile: ptr.To(path)},
		Reason:  reason,
		Message: path,
	}
}

// ensureManagedProfile keeps one SeccompProfile per namespace and
// reports where the nodes hold it. It returns a no match error when
// the Security Profiles Operator is not installed.
func (r *VdiInstanceReconciler) ensureManagedProfile(ctx context.Context, namespace string) (string, bool, error) {
	var spec map[string]any
	if err := json.Unmarshal(sandboxSeccompSpec, &spec); err != nil {
		return "", false, err
	}
	sum := sha256.Sum256(sandboxSeccompSpec)
	hash := hex.EncodeToString(sum[:8])

	// Uncached: a watch on a kind that may not exist would never sync.
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	profile := &unstructured.Unstructured{}
	profile.SetGroupVersionKind(seccompProfileGVK)
	err := reader.Get(ctx, client.ObjectKey{Name: sandboxProfileName, Namespace: namespace}, profile)
	switch {
	case apierrors.IsNotFound(err):
		profile.SetName(sandboxProfileName)
		profile.SetNamespace(namespace)
		labelManaged(profile, "")
		profile.SetAnnotations(map[string]string{profileHashKey: hash})
		profile.Object["spec"] = spec
		return "", false, r.Create(ctx, profile)
	case err != nil:
		return "", false, err
	}
	// The hash, not the spec, decides: the API server may add defaults.
	if profile.GetAnnotations()[profileHashKey] != hash {
		labelManaged(profile, "")
		annotations := profile.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[profileHashKey] = hash
		profile.SetAnnotations(annotations)
		profile.Object["spec"] = spec
		return "", false, r.Update(ctx, profile)
	}
	if profile.GetLabels()[labelManagedBy] != managedBy {
		// An older manager made it unlabeled; its spec is still current.
		labelManaged(profile, "")
		if err := r.Update(ctx, profile); err != nil {
			return "", false, err
		}
	}
	state, _, _ := unstructured.NestedString(profile.Object, "status", "status")
	path, _, _ := unstructured.NestedString(profile.Object, "status", "localhostProfile")
	return path, state == "Installed" && path != "", nil
}
