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

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
)

// Helper function for tests to find a condition by type
func findConditionInList(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

var _ = Describe("VdiInstance Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		vdiinstance := &vdiv1alpha1.VdiInstance{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind VdiInstance")
			err := k8sClient.Get(ctx, typeNamespacedName, vdiinstance)
			if err != nil && errors.IsNotFound(err) {
				resource := &vdiv1alpha1.VdiInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &vdiv1alpha1.VdiInstance{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance VdiInstance")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &VdiInstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Check if the standard conditions were initialized
			updatedResource := &vdiv1alpha1.VdiInstance{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedResource)
			Expect(err).NotTo(HaveOccurred())

			By("Validating that initial conditions were set")
			Expect(updatedResource.Status.Conditions).NotTo(BeEmpty())

			// Check for all required standard conditions
			conditionTypes := []string{
				ConditionReady,
				ConditionAvailable,
				ConditionProgressing,
				ConditionVMReady,
				ConditionStreamingReady,
				ConditionNetworkReady,
				ConditionRBACReady,
			}

			for _, condType := range conditionTypes {
				condition := findConditionInList(updatedResource.Status.Conditions, condType)
				Expect(condition).NotTo(BeNil(), "Expected condition %s to be set", condType)
			}

			// Initial reconciliation should set Progressing to true
			progressingCond := findConditionInList(updatedResource.Status.Conditions, ConditionProgressing)
			Expect(progressingCond).NotTo(BeNil())
			Expect(progressingCond.Status).To(Equal(metav1.ConditionTrue))

			// Ready should be false initially
			readyCond := findConditionInList(updatedResource.Status.Conditions, ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})
