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

// VdiTemplateSpec defines the template for creating VdiInstances
type VdiTemplateSpec struct {
	// Description is a human-readable description of the template
	// +optional
	Description string `json:"description,omitempty"`

	// DisplayName is a user-friendly name for the template
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Icon is a base64-encoded image to represent the template in UIs
	// +optional
	Icon string `json:"icon,omitempty"`

	// Tags are keywords that can be used to search for this template
	// +optional
	Tags []string `json:"tags,omitempty"`

	// KubeVirtVMTemplateRef refers to a KubeVirt VirtualMachineInstance template
	// that will be used as the base for creating new VMs
	// +optional
	KubeVirtVMTemplateRef KubeVirtVMReference `json:"kubeVirtVMTemplateRef,omitempty"`

	// DefaultInstancePrefix defines the prefix to use when creating
	// VDI instances from this template
	// +optional
	DefaultInstancePrefix string `json:"defaultInstancePrefix,omitempty"`

	// VdiInstanceTemplateSpec defines the template for creating new VdiInstances
	VdiInstanceTemplateSpec VdiInstanceTemplateSpec `json:"vdiInstanceTemplateSpec"`
}

// VdiInstanceTemplateSpec contains the template for a VdiInstance without
// the TypeMeta or ObjectMeta that will be generated during instantiation
type VdiInstanceTemplateSpec struct {
	// SunshineConfig defines the configuration for the Sunshine streaming server
	// +optional
	SunshineConfig SunshineConfiguration `json:"sunshineConfig,omitempty"`

	// MoonlightConfig defines the configuration for the Moonlight client
	// +optional
	MoonlightConfig MoonlightConfiguration `json:"moonlightConfig,omitempty"`

	// UserAccess defines who can access VDI instances created from this template
	// +optional
	UserAccess []UserAccessPolicy `json:"userAccess,omitempty"`

	// AccessControl defines RBAC integration for access control
	// +optional
	AccessControl *RBACAccessControl `json:"accessControl,omitempty"`

	// Resources defines the compute resources for the VDI components
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty"`

	// NetworkConfig defines network settings for VDI instances created from this template
	// +optional
	NetworkConfig NetworkConfiguration `json:"networkConfig,omitempty"`
}

// VdiTemplateStatus defines the observed state of VdiTemplate
type VdiTemplateStatus struct {
	// Conditions represents the latest available observations
	// of the template's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// InstanceCount represents the number of VdiInstances created from this template
	// +optional
	InstanceCount int32 `json:"instanceCount,omitempty"`

	// LastUsedTimestamp represents when this template was last used to create an instance
	// +optional
	LastUsedTimestamp *metav1.Time `json:"lastUsedTimestamp,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Display Name",type="string",JSONPath=".spec.displayName"
// +kubebuilder:printcolumn:name="Instances",type="integer",JSONPath=".status.instanceCount"
// +kubebuilder:printcolumn:name="Last Used",type="date",JSONPath=".status.lastUsedTimestamp"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// VdiTemplate is the Schema for the vditemplates API
type VdiTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VdiTemplateSpec   `json:"spec,omitempty"`
	Status VdiTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VdiTemplateList contains a list of VdiTemplate
type VdiTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VdiTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VdiTemplate{}, &VdiTemplateList{})
}
