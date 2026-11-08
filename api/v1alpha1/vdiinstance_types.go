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
	// Size is 20Gi when empty.
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

// ClassExposeSpec is the part of ExposeSpec a class may set too.
type ClassExposeSpec struct {
	// Type is NodePort when empty.
	// +optional
	Type ExposeType `json:"type,omitempty"`
	// WebUI publishes Sunshine's admin web UI, on base+1, and lets it
	// answer any address instead of private ones only. Changing it
	// restarts the desktop. Behind a LoadBalancer the web UI also needs
	// host: it refuses changes from an address it was not started with.
	// +optional
	WebUI *bool `json:"webUI,omitempty"`
}

// ExposeSpec controls the Service in front of the desktop.
type ExposeSpec struct {
	ClassExposeSpec `json:",inline"`
	// BasePort pins the Sunshine base port. Allocated when empty.
	// Moonlight derives the other ports from it: -5, +1, +9, +10, +11, +21.
	// +kubebuilder:validation:Minimum=1029
	// +kubebuilder:validation:Maximum=65514
	// +optional
	BasePort int32 `json:"basePort,omitempty"`
	// Host overrides the address reported to clients, for example a DNS name.
	// +optional
	Host string `json:"host,omitempty"`
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

// Placement picks the nodes a desktop runs on. Each field goes to the
// pod as is. An instance field replaces the class's whole: nodeSelector
// keys and tolerations are not merged. An empty one counts as unset.
// Changing one restarts the desktop.
type Placement struct {
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// PriorityClassName lets a desktop preempt pods of a lower priority.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// RuntimeClassName picks the container runtime, a GPU one for example.
	// +kubebuilder:validation:MinLength=1
	// +optional
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`
}

// PairingSpec carries the PIN Moonlight shows, so nobody types it inside
// the desktop.
type PairingSpec struct {
	// +kubebuilder:validation:Pattern=`^[0-9]{4}$`
	PIN string `json:"pin"`
	// ClientName labels the paired client in Sunshine.
	// +kubebuilder:default=moonlight
	// +optional
	ClientName string `json:"clientName,omitempty"`
}

// VdiInstanceSpec defines one desktop for one owner. A field it sets
// wins over its class's; expose, stream, home and placement merge field
// by field.
// +kubebuilder:validation:XValidation:rule="has(self.image) || has(self.className)",message="needs image or className"
type VdiInstanceSpec struct {
	// ClassName names the VdiClass this desktop takes what it leaves
	// unset from. While the class does not exist the desktop waits. A
	// class change reaches the desktop, and restarts it when the pod
	// changes.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ClassName string `json:"className,omitempty"`
	// Image is the desktop's container image, its class's when empty.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Image string `json:"image,omitempty"`
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Resources bound the desktop's CPU and memory. When set they replace
	// the class's whole: requests and limits are not merged by resource.
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
	// 0s never stops it, even when its class sets a timeout.
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
	NestedSandboxes *bool `json:"nestedSandboxes,omitempty"`
	// Placement is an admin's one-off for this desktop, over the class's.
	// +optional
	Placement *Placement `json:"placement,omitempty"`
	// Pairing approves the Moonlight client waiting for this PIN. The
	// operator removes it once the pairing succeeded or failed.
	// +optional
	Pairing *PairingSpec `json:"pairing,omitempty"`
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

// PairingPhase is the outcome of a pairing request.
type PairingPhase string

// Pairing phases; Pending retries, the other two are final.
const (
	PairingPending PairingPhase = "Pending"
	PairingPaired  PairingPhase = "Paired"
	PairingFailed  PairingPhase = "Failed"
)

// PairingStatus reports the last pairing request.
type PairingStatus struct {
	// PIN names the request this status is about.
	// +optional
	PIN string `json:"pin,omitempty"`
	// +optional
	Phase PairingPhase `json:"phase,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// RequestedAt is when the operator first saw the PIN. Moonlight gives
	// up ten minutes later, and so does the operator.
	// +optional
	RequestedAt *metav1.Time `json:"requestedAt,omitempty"`
	// +optional
	PairedAt *metav1.Time `json:"pairedAt,omitempty"`
}

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
	// Pairing reports the last PIN handed over in spec.pairing.
	// +optional
	Pairing *PairingStatus `json:"pairing,omitempty"`
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
// +kubebuilder:printcolumn:name="Class",type=string,JSONPath=`.spec.className`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VdiInstance is a streamed virtual desktop.
type VdiInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
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
