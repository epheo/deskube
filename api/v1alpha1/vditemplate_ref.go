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

// VdiTemplateRef defines a reference to a VdiTemplate
type VdiTemplateRef struct {
	// Name of the VdiTemplate
	Name string `json:"name"`

	// Namespace of the VdiTemplate (optional - defaults to the same namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// InstanceName allows specifying a custom name for the instance
	// If empty, one will be generated using the template's DefaultInstancePrefix
	// +optional
	InstanceName string `json:"instanceName,omitempty"`
}

// ProvisionSource indicates the source for provisioning a VDI instance
// +kubebuilder:validation:Enum=Template;Direct
type ProvisionSource string

const (
	// ProvisionFromTemplate means the VDI instance should be created from a template
	ProvisionFromTemplate ProvisionSource = "Template"

	// ProvisionDirect means the VDI instance configuration is provided directly
	ProvisionDirect ProvisionSource = "Direct"
)
