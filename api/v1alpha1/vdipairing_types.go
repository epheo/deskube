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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VdiPairingSpec carries the PIN Moonlight shows to the operator,
// so nobody has to type it inside the guest.
type VdiPairingSpec struct {
	// InstanceRef is a VdiInstance in the same namespace.
	InstanceRef string `json:"instanceRef"`
	// +kubebuilder:validation:Pattern=`^[0-9]{4}$`
	PIN string `json:"pin"`
	// ClientName labels the paired client in Sunshine.
	// +kubebuilder:default=moonlight
	// +optional
	ClientName string `json:"clientName,omitempty"`
}

// VdiPairingPhase is the outcome of the pairing attempt.
type VdiPairingPhase string

// Pairing phases; Pending retries, the other two are final.
const (
	PairingPending VdiPairingPhase = "Pending"
	PairingPaired  VdiPairingPhase = "Paired"
	PairingFailed  VdiPairingPhase = "Failed"
)

// VdiPairingStatus reports the outcome.
type VdiPairingStatus struct {
	// +optional
	Phase VdiPairingPhase `json:"phase,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	PairedAt *metav1.Time `json:"pairedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VdiPairing approves one Moonlight pairing request on a VdiInstance.
type VdiPairing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VdiPairingSpec   `json:"spec,omitempty"`
	Status VdiPairingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VdiPairingList contains a list of VdiPairing.
type VdiPairingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VdiPairing `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VdiPairing{}, &VdiPairingList{})
}
