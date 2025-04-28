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
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
)

// processTemplateBasedInstance processes a VDI instance that should be provisioned from a template
// This function should be called at the beginning of the reconcile loop
func (r *VdiInstanceReconciler) processTemplateBasedInstance(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx)

	// Skip if this is not a template-based instance
	if vdi.Spec.ProvisionSource != vdiv1alpha1.ProvisionFromTemplate || vdi.Spec.TemplateRef == nil {
		return nil
	}

	// Skip if already populated with template values (we can tell by checking if KubeVirtVMRef is set)
	if vdi.Spec.KubeVirtVMRef != nil && vdi.Spec.KubeVirtVMRef.Name != "" {
		return nil
	}

	logger.Info("Processing template-based VDI instance", "templateRef", vdi.Spec.TemplateRef)

	// Get the template
	templateNamespace := vdi.Spec.TemplateRef.Namespace
	if templateNamespace == "" {
		templateNamespace = vdi.Namespace
	}

	template := &vdiv1alpha1.VdiTemplate{}
	templateName := types.NamespacedName{
		Name:      vdi.Spec.TemplateRef.Name,
		Namespace: templateNamespace,
	}

	if err := r.Get(ctx, templateName, template); err != nil {
		logger.Error(err, "Failed to retrieve VdiTemplate", "name", templateName.Name, "namespace", templateName.Namespace)
		return fmt.Errorf("failed to retrieve template %s/%s: %w", templateNamespace, vdi.Spec.TemplateRef.Name, err)
	}

	// Apply template properties to the instance
	return r.applyTemplateToInstance(ctx, vdi, template)
}

// applyTemplateToInstance applies the properties from a template to a VDI instance
func (r *VdiInstanceReconciler) applyTemplateToInstance(ctx context.Context, vdi *vdiv1alpha1.VdiInstance, template *vdiv1alpha1.VdiTemplate) error {
	logger := log.FromContext(ctx)
	logger.Info("Applying template to VDI instance", "template", template.Name, "instance", vdi.Name)

	// Apply KubeVirt VM reference if specified
	if template.Spec.KubeVirtVMTemplateRef.Name != "" {
		if vdi.Spec.KubeVirtVMRef == nil {
			vdi.Spec.KubeVirtVMRef = &vdiv1alpha1.KubeVirtVMReference{}
		}
		vdi.Spec.KubeVirtVMRef.Name = template.Spec.KubeVirtVMTemplateRef.Name

		if template.Spec.KubeVirtVMTemplateRef.Namespace != "" {
			vdi.Spec.KubeVirtVMRef.Namespace = template.Spec.KubeVirtVMTemplateRef.Namespace
		}
	}

	// Apply Sunshine configuration if not overridden
	if vdi.Spec.SunshineConfig.Image == "" {
		vdi.Spec.SunshineConfig = template.Spec.VdiInstanceTemplateSpec.SunshineConfig
	}

	// Apply Moonlight configuration if not overridden
	if vdi.Spec.MoonlightConfig.Image == "" {
		vdi.Spec.MoonlightConfig = template.Spec.VdiInstanceTemplateSpec.MoonlightConfig
	}

	// Apply user access if not specified
	if len(vdi.Spec.UserAccess) == 0 {
		vdi.Spec.UserAccess = template.Spec.VdiInstanceTemplateSpec.UserAccess
	}

	// Apply RBAC access control if not specified
	if vdi.Spec.AccessControl == nil {
		vdi.Spec.AccessControl = template.Spec.VdiInstanceTemplateSpec.AccessControl
	}

	// Apply resources if not specified
	if vdi.Spec.Resources.Sunshine == nil && vdi.Spec.Resources.Moonlight == nil {
		vdi.Spec.Resources = template.Spec.VdiInstanceTemplateSpec.Resources
	}

	// Apply network configuration if not specified
	if vdi.Spec.NetworkConfig.ServiceType == "" {
		vdi.Spec.NetworkConfig = template.Spec.VdiInstanceTemplateSpec.NetworkConfig
	}

	// Set labels to indicate this instance was created from a template
	if vdi.Labels == nil {
		vdi.Labels = make(map[string]string)
	}
	vdi.Labels["vdi.desku.be/template-name"] = template.Name
	vdi.Labels["vdi.desku.be/template-namespace"] = template.Namespace

	// Update the VDI instance to persist the template-applied changes
	if err := r.Update(ctx, vdi); err != nil {
		logger.Error(err, "Failed to update VDI instance with template values")
		return err
	}

	// Update the template's last used timestamp in a non-blocking way
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Get the latest version of the template
		latestTemplate := &vdiv1alpha1.VdiTemplate{}
		if err := r.Get(updateCtx, types.NamespacedName{Name: template.Name, Namespace: template.Namespace}, latestTemplate); err == nil {
			// Update LastUsedTimestamp and increment InstanceCount
			now := metav1.Now()
			latestTemplate.Status.LastUsedTimestamp = &now
			latestTemplate.Status.InstanceCount++

			if err := r.Status().Update(updateCtx, latestTemplate); err != nil {
				logger.Error(err, "Failed to update template status after creating instance",
					"template", template.Name, "namespace", template.Namespace)
			}
		}
	}()

	return nil
}
