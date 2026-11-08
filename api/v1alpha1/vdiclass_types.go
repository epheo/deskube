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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VdiClassSpec is what an admin chooses for a kind of desktop. A
// VdiInstance naming the class takes each field it leaves unset.
type VdiClassSpec struct {
	// Image is the desktops' container image.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets name Secrets in each desktop's own namespace.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Resources bound each desktop's CPU and memory. An instance that
	// sets resources replaces these whole, not resource by resource.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Home applies to desktops whose home does not exist yet: a bound
	// claim keeps its size and storage class.
	// +optional
	Home HomeSpec `json:"home,omitempty"`
	// IdleTimeout stops a desktop after this long without a session.
	// A desktop that sets 0s is never stopped.
	// +optional
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`
	// Expose has no basePort or host: those belong to one desktop.
	// +optional
	Expose ClassExposeSpec `json:"expose,omitempty"`
	// +optional
	Stream StreamSpec `json:"stream,omitempty"`
	// NestedSandboxes is spec.nestedSandboxes of VdiInstance, for the
	// desktops of this class.
	// +optional
	NestedSandboxes *bool `json:"nestedSandboxes,omitempty"`
	// Placement picks the nodes of the class's desktops: GPU nodes and a
	// GPU runtime, for one.
	// +optional
	Placement *Placement `json:"placement,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VdiClass is a kind of desktop an admin offers, as a StorageClass is a
// kind of volume. A change reaches every desktop naming it, and
// restarts the running ones whose pod it changes.
type VdiClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VdiClassSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// VdiClassList contains a list of VdiClass.
type VdiClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VdiClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VdiClass{}, &VdiClassList{})
}
