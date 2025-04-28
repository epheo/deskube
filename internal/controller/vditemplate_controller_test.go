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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
)

var _ = Describe("VdiTemplate controller", func() {
	const (
		templateName      = "test-vditemplate"
		templateNamespace = "default"
		timeout           = time.Second * 10
		interval          = time.Millisecond * 250
	)

	Context("When creating a VdiTemplate", func() {
		It("Should create successfully", func() {
			By("Creating a new VdiTemplate")
			ctx := context.Background()
			template := &vdiv1alpha1.VdiTemplate{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "vdi.desku.be/v1alpha1",
					Kind:       "VdiTemplate",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      templateName,
					Namespace: templateNamespace,
				},
				Spec: vdiv1alpha1.VdiTemplateSpec{
					Description: "Test VDI Template",
					DisplayName: "Test Template",
					Tags:        []string{"test", "desktop"},
					VdiInstanceTemplateSpec: vdiv1alpha1.VdiInstanceTemplateSpec{
						SunshineConfig: vdiv1alpha1.SunshineConfiguration{
							FPS:        60,
							Resolution: "1920x1080",
							Bitrate:    50,
						},
						NetworkConfig: vdiv1alpha1.NetworkConfiguration{
							ServiceType:    "ClusterIP",
							ExposureMethod: "Ingress",
							TLS:            true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).Should(Succeed())

			// Wait for the template to be created and its status to be updated
			templateLookupKey := types.NamespacedName{Name: templateName, Namespace: templateNamespace}
			createdTemplate := &vdiv1alpha1.VdiTemplate{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, templateLookupKey, createdTemplate)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Template should have been created with the correct spec
			Expect(createdTemplate.Spec.Description).Should(Equal("Test VDI Template"))
			Expect(createdTemplate.Spec.DisplayName).Should(Equal("Test Template"))
			Expect(createdTemplate.Spec.Tags).Should(ContainElements("test", "desktop"))

			// Initially, there should be no instances created from this template
			Expect(createdTemplate.Status.InstanceCount).Should(BeZero())
		})
	})

	Context("When creating a VdiInstance from a template", func() {
		It("Should update the template status", func() {
			By("Creating a VdiInstance with a reference to the template")
			ctx := context.Background()

			// Get the template to reference
			templateLookupKey := types.NamespacedName{Name: templateName, Namespace: templateNamespace}
			existingTemplate := &vdiv1alpha1.VdiTemplate{}
			Expect(k8sClient.Get(ctx, templateLookupKey, existingTemplate)).Should(Succeed())

			// Create a VdiInstance that references this template
			instance := &vdiv1alpha1.VdiInstance{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "vdi.desku.be/v1alpha1",
					Kind:       "VdiInstance",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: templateNamespace,
					Labels: map[string]string{
						"vdi.desku.be/template-name":      templateName,
						"vdi.desku.be/template-namespace": templateNamespace,
					},
				},
				Spec: vdiv1alpha1.VdiInstanceSpec{
					KubeVirtVMRef: &vdiv1alpha1.KubeVirtVMReference{
						Name:      "test-vm",
						Namespace: templateNamespace,
					},
					SunshineConfig: existingTemplate.Spec.VdiInstanceTemplateSpec.SunshineConfig,
					NetworkConfig:  existingTemplate.Spec.VdiInstanceTemplateSpec.NetworkConfig,
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for the template status to be updated to show 1 instance
			Eventually(func() int32 {
				updatedTemplate := &vdiv1alpha1.VdiTemplate{}
				err := k8sClient.Get(ctx, templateLookupKey, updatedTemplate)
				if err != nil {
					return 0
				}
				return updatedTemplate.Status.InstanceCount
			}, timeout, interval).Should(Equal(int32(1)))

			// Verify template's last used timestamp is updated
			updatedTemplate := &vdiv1alpha1.VdiTemplate{}
			Expect(k8sClient.Get(ctx, templateLookupKey, updatedTemplate)).Should(Succeed())
			Expect(updatedTemplate.Status.LastUsedTimestamp).ShouldNot(BeNil())
		})
	})

	Context("When deleting a VdiInstance created from a template", func() {
		It("Should update the template status", func() {
			By("Deleting the VdiInstance")
			ctx := context.Background()

			// Get the instance to delete
			instanceLookupKey := types.NamespacedName{Name: "test-instance", Namespace: templateNamespace}
			instance := &vdiv1alpha1.VdiInstance{}
			Expect(k8sClient.Get(ctx, instanceLookupKey, instance)).Should(Succeed())

			// Delete the instance
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())

			// Wait for the template status to be updated to show 0 instances
			templateLookupKey := types.NamespacedName{Name: templateName, Namespace: templateNamespace}
			Eventually(func() int32 {
				updatedTemplate := &vdiv1alpha1.VdiTemplate{}
				err := k8sClient.Get(ctx, templateLookupKey, updatedTemplate)
				if err != nil {
					return -1
				}
				return updatedTemplate.Status.InstanceCount
			}, timeout, interval).Should(Equal(int32(0)))
		})
	})
})
