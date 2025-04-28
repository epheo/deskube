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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KubeVirtVMReference defines a reference to a KubeVirt VirtualMachine
type KubeVirtVMReference struct {
	// Name of the referenced KubeVirt VirtualMachine
	Name string `json:"name"`

	// Namespace of the referenced KubeVirt VirtualMachine
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RBACRoleRef defines a reference to a Kubernetes Role or ClusterRole
type RBACRoleRef struct {
	// Kind defines the type of role being referenced (Role or ClusterRole)
	// +kubebuilder:validation:Enum=Role;ClusterRole
	Kind string `json:"kind"`

	// Name is the name of the referenced role
	Name string `json:"name"`

	// Namespace is the namespace of the referenced role (only for Role, not for ClusterRole)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RBACAccessControl defines how RBAC is integrated with the VDI instance
type RBACAccessControl struct {
	// Enabled indicates whether RBAC controls are enforced
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// CreateRoles indicates whether the operator should create RBAC roles for the VDI instance
	// +optional
	CreateRoles bool `json:"createRoles,omitempty"`

	// RolePrefix is the prefix used for generated roles
	// +optional
	RolePrefix string `json:"rolePrefix,omitempty"`

	// DefaultRoleRef specifies a default role reference for users without specific roles
	// +optional
	DefaultRoleRef *RBACRoleRef `json:"defaultRoleRef,omitempty"`
}

// SunshineConfiguration defines the configuration for the Sunshine streaming server
type SunshineConfiguration struct {
	// Image defines the Docker image for Sunshine
	// +optional
	Image string `json:"image,omitempty"`

	// FPS defines the frames per second for streaming
	// +optional
	FPS int32 `json:"fps,omitempty"`

	// Resolution defines the streaming resolution
	// +optional
	Resolution string `json:"resolution,omitempty"`

	// Bitrate defines the streaming bitrate in Mbps
	// +optional
	Bitrate int32 `json:"bitrate,omitempty"`

	// AdditionalOptions defines additional Sunshine configuration options
	// +optional
	AdditionalOptions map[string]string `json:"additionalOptions,omitempty"`
}

// MoonlightConfiguration defines the configuration for the Moonlight client
type MoonlightConfiguration struct {
	// Image defines the Docker image for Moonlight
	// +optional
	Image string `json:"image,omitempty"`

	// ClientOptions defines client-specific configurations
	// +optional
	ClientOptions map[string]string `json:"clientOptions,omitempty"`
}

// UserAccessPolicy defines who can access this VDI instance
type UserAccessPolicy struct {
	// Type defines the type of access (user, group, serviceaccount)
	// +kubebuilder:validation:Enum=user;group;serviceaccount
	// +optional
	Type string `json:"type,omitempty"`

	// Name defines the name of the user, group, or serviceaccount
	// +optional
	Name string `json:"name,omitempty"`

	// Username defines the username for access (deprecated, use Name instead)
	// +optional
	Username string `json:"username,omitempty"`

	// Groups defines the groups allowed to access (deprecated, use separate UserAccessPolicy entries instead)
	// +optional
	Groups []string `json:"groups,omitempty"`

	// AuthType defines the authentication type (e.g., basic, oauth, ldap)
	// +optional
	AuthType string `json:"authType,omitempty"`

	// Permissions defines what operations are allowed
	// +optional
	Permissions []string `json:"permissions,omitempty"`

	// RoleRef specifies a ClusterRole or Role that defines permissions
	// +optional
	RoleRef *RBACRoleRef `json:"roleRef,omitempty"`
}

// ResourceRequirements defines resource requirements for VDI components
type ResourceRequirements struct {
	// Sunshine defines resources for the Sunshine streaming server
	// +optional
	Sunshine *corev1.ResourceRequirements `json:"sunshine,omitempty"`

	// Moonlight defines resources for the Moonlight client
	// +optional
	Moonlight *corev1.ResourceRequirements `json:"moonlight,omitempty"`
}

// NetworkConfiguration defines network settings for the VDI instance
type NetworkConfiguration struct {
	// ServiceType defines the type of service to expose VDI (ClusterIP, NodePort, LoadBalancer)
	// +optional
	ServiceType string `json:"serviceType,omitempty"`

	// Port defines the port for the VDI service
	// +optional
	Port int32 `json:"port,omitempty"`

	// ExposureMethod defines how to expose the VDI (Ingress, Route, LoadBalancer)
	// +optional
	ExposureMethod string `json:"exposureMethod,omitempty"`

	// Hostname defines the hostname for accessing the VDI
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// TLS defines whether TLS should be enabled
	// +optional
	TLS bool `json:"tls,omitempty"`
}

// ComponentStatus defines the status of a VDI component (Sunshine or Moonlight)
type ComponentStatus struct {
	// Ready indicates if the component is ready
	Ready bool `json:"ready"`

	// PodName contains the name of the pod running the component
	// +optional
	PodName string `json:"podName,omitempty"`

	// PodIP contains the IP address of the pod running the component
	// +optional
	PodIP string `json:"podIP,omitempty"`

	// Version contains the version of the component
	// +optional
	Version string `json:"version,omitempty"`

	// Message contains additional status information
	// +optional
	Message string `json:"message,omitempty"`
}

// ConnectionInfo defines the connection details for accessing the VDI instance
type ConnectionInfo struct {
	// AccessURL provides the URL for accessing the VDI
	// +optional
	AccessURL string `json:"accessURL,omitempty"`

	// Port provides the port number for accessing the VDI
	// +optional
	Port int32 `json:"port,omitempty"`

	// Credentials provides a reference to the secret containing credentials
	// +optional
	Credentials *corev1.SecretReference `json:"credentials,omitempty"`

	// ConnectionParams provides additional connection parameters
	// +optional
	ConnectionParams map[string]string `json:"connectionParams,omitempty"`
}

// VdiInstanceSpec defines the desired state of VdiInstance
type VdiInstanceSpec struct {
	// KubeVirtVMRef refers to the KubeVirt VirtualMachine that this VDI instance will connect to
	KubeVirtVMRef KubeVirtVMReference `json:"kubeVirtVMRef"`

	// SunshineConfig defines the configuration for the Sunshine streaming server
	// +optional
	SunshineConfig SunshineConfiguration `json:"sunshineConfig,omitempty"`

	// MoonlightConfig defines the configuration for the Moonlight client
	// +optional
	MoonlightConfig MoonlightConfiguration `json:"moonlightConfig,omitempty"`

	// UserAccess defines who can access this VDI instance
	// +optional
	UserAccess []UserAccessPolicy `json:"userAccess,omitempty"`

	// AccessControl defines RBAC integration for access control
	// +optional
	AccessControl *RBACAccessControl `json:"accessControl,omitempty"`

	// Resources defines the compute resources for the VDI components
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty"`

	// NetworkConfig defines network settings for the VDI instance
	// +optional
	NetworkConfig NetworkConfiguration `json:"networkConfig,omitempty"`
}

// VdiInstanceStatus defines the observed state of VdiInstance
type VdiInstanceStatus struct {
	// Phase represents the current phase of the VDI instance (Pending, Running, Failed, etc.)
	Phase string `json:"phase,omitempty"`

	// Conditions represents the latest available observations of the VDI state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// VMStatus represents the status of the underlying KubeVirt VM
	// +optional
	VMStatus string `json:"vmStatus,omitempty"`

	// SunshineStatus contains information about the Sunshine streaming server
	// +optional
	SunshineStatus ComponentStatus `json:"sunshineStatus,omitempty"`

	// MoonlightStatus contains information about the Moonlight client
	// +optional
	MoonlightStatus ComponentStatus `json:"moonlightStatus,omitempty"`

	// ConnectionDetails contains information needed to connect to the VDI instance
	// +optional
	ConnectionDetails ConnectionInfo `json:"connectionDetails,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VdiInstance is the Schema for the vdiinstances API
type VdiInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VdiInstanceSpec   `json:"spec,omitempty"`
	Status VdiInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VdiInstanceList contains a list of VdiInstance
type VdiInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VdiInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VdiInstance{}, &VdiInstanceList{})
}
