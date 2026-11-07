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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HomeSpec is the volume that keeps the desktop user's files, pairings
// included, across restarts.
type HomeSpec struct {
	// +kubebuilder:default="20Gi"
	// +optional
	Size resource.Quantity `json:"size,omitempty"`
	// StorageClassName is the cluster default when empty.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// ExposeType selects how the stream reaches the client.
// +kubebuilder:validation:Enum=NodePort;LoadBalancer
type ExposeType string

// Exposure types.
const (
	ExposeNodePort     ExposeType = "NodePort"
	ExposeLoadBalancer ExposeType = "LoadBalancer"
)

// ExposeSpec controls the Service in front of the desktop.
type ExposeSpec struct {
	// +kubebuilder:default=NodePort
	// +optional
	Type ExposeType `json:"type,omitempty"`
	// BasePort pins the Sunshine base port. Allocated when empty.
	// Moonlight derives the other ports from it: -5, +1, +9, +10, +11, +21.
	// +kubebuilder:validation:Minimum=1029
	// +kubebuilder:validation:Maximum=65514
	// +optional
	BasePort int32 `json:"basePort,omitempty"`
	// Host overrides the address reported to clients, for example a DNS name.
	// +optional
	Host string `json:"host,omitempty"`
	// WebUI publishes Sunshine's admin web UI, on base+1, and lets it
	// answer any address instead of private ones only. Changing it
	// restarts the desktop. Behind a LoadBalancer the web UI also needs
	// host: it refuses changes from an address it was not started with.
	// +optional
	WebUI bool `json:"webUI,omitempty"`
}

// StreamSpec is passed to the session inside the desktop.
type StreamSpec struct {
	// Encoder is chosen by the image when empty: vaapi when the desktop
	// has a GPU, software otherwise.
	// +kubebuilder:validation:Enum=software;vaapi
	// +optional
	Encoder string `json:"encoder,omitempty"`
	// KeyboardLayout is up to four XKB layout names separated by commas,
	// us when empty. Variants and options are not accepted.
	// +kubebuilder:validation:Pattern=`^[a-z]{2,8}(,[a-z]{2,8}){0,3}$`
	// +optional
	KeyboardLayout string `json:"keyboardLayout,omitempty"`
}

// VdiInstanceSpec defines one desktop for one owner.
type VdiInstanceSpec struct {
	// Image is the desktop's container image.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Resources bound the desktop's CPU and memory.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	Home HomeSpec `json:"home,omitempty"`
	// Owner is the Kubernetes user allowed to read this instance and pair clients.
	// +optional
	Owner string `json:"owner,omitempty"`
	// Running is the desired power state. A stopped desktop keeps its home.
	// +kubebuilder:default=true
	// +optional
	Running *bool `json:"running,omitempty"`
	// IdleTimeout stops the desktop after this long without a session.
	// The operator sets running to false; set it back to true to resume.
	// +optional
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`
	// +optional
	Expose ExposeSpec `json:"expose,omitempty"`
	// +optional
	Stream StreamSpec `json:"stream,omitempty"`
	// NestedSandboxes lets programs in the desktop build sandboxes of
	// their own, as Steam, Flatpak, and container engines do. The pod
	// then runs in its own user namespace with an unmasked /proc, and on
	// OpenShift under the nested-container SCC. It still holds no
	// capability, cannot gain any, and is not root in its namespace. What
	// it gives up is the runtime's default seccomp filter, which forbids
	// nested namespaces, unless the manager was given a profile to use.
	// +optional
	NestedSandboxes bool `json:"nestedSandboxes,omitempty"`
}

// VdiInstancePhase is a one word summary of the status conditions.
type VdiInstancePhase string

// Phases, from creation to a desktop a client can dial.
const (
	PhaseProvisioning VdiInstancePhase = "Provisioning"
	PhaseStarting     VdiInstancePhase = "Starting"
	PhaseReady        VdiInstancePhase = "Ready"
	PhaseStopping     VdiInstancePhase = "Stopping"
	PhaseStopped      VdiInstancePhase = "Stopped"
	PhaseFailed       VdiInstancePhase = "Failed"
)

// VdiInstanceStatus reports what a client needs to connect.
type VdiInstanceStatus struct {
	// +optional
	Phase VdiInstancePhase `json:"phase,omitempty"`
	// Address is what to type in Moonlight, host:port.
	// +optional
	Address string `json:"address,omitempty"`
	// +optional
	BasePort int32 `json:"basePort,omitempty"`
	// AdminSecret holds the Sunshine web UI credentials.
	// +optional
	AdminSecret string `json:"adminSecret,omitempty"`
	// SessionState is free or busy, from Sunshine serverinfo.
	// +optional
	SessionState string `json:"sessionState,omitempty"`
	// LastActive is when a session was last seen, or when the desktop became Ready.
	// +optional
	LastActive *metav1.Time `json:"lastActive,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`
// +kubebuilder:printcolumn:name="Session",type=string,JSONPath=`.status.sessionState`
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=`.spec.owner`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VdiInstance is a streamed virtual desktop.
type VdiInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VdiInstanceSpec   `json:"spec,omitempty"`
	Status VdiInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VdiInstanceList contains a list of VdiInstance.
type VdiInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VdiInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VdiInstance{}, &VdiInstanceList{})
}
