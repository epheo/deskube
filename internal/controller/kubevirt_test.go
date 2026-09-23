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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

var _ = Describe("KubeVirt Integration Tests", func() {
	Context("When checking KubeVirt VM status", func() {
		const (
			vmName          = "test-vm"
			vmNamespace     = "default"
			vdiInstanceName = "test-vdi-instance"
			vdiInstanceNs   = "default"
		)

		ctx := context.Background()

		// Create a VdiInstance to test with
		vdiInstance := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdiInstanceName,
				Namespace: vdiInstanceNs,
			},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				KubeVirtVMRef: &vdiv1alpha1.KubeVirtVMReference{
					Name:      vmName,
					Namespace: vmNamespace,
				},
			},
		}

		// Create a KubeVirt VM to test with
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				Running: ptr(true),
			},
		}

		BeforeEach(func() {
			// Create the VM in the test cluster
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up the test VM
			Expect(k8sClient.Delete(ctx, vm)).To(Succeed())
		})

		It("Should find a VM that exists", func() {
			// Set the VM as ready
			updatedVM := &kubevirtv1.VirtualMachine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: vmNamespace}, updatedVM)).To(Succeed())

			updatedVM.Status.Ready = true
			updatedVM.Status.PrintableStatus = "Running"
			Expect(k8sClient.Status().Update(ctx, updatedVM)).To(Succeed())

			// Create a controller for testing
			r := &VdiInstanceReconciler{
				Client: k8sClient,
				Scheme: runtime.NewScheme(),
			}

			// Call the function being tested
			status, err := r.checkKubeVirtVM(ctx, vdiInstance)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("Ready"))
		})

		It("Should handle missing VM", func() {
			// Delete the VM
			Expect(k8sClient.Delete(ctx, vm)).To(Succeed())

			// Create a controller for testing
			r := &VdiInstanceReconciler{
				Client: k8sClient,
				Scheme: runtime.NewScheme(),
			}

			// Call the function being tested
			status, err := r.checkKubeVirtVM(ctx, vdiInstance)
			Expect(err).To(HaveOccurred())
			Expect(status).To(Equal("NotFound"))
		})

		It("Should handle missing KubeVirtVMRef", func() {
			// Create a VDI instance without a KubeVirtVMRef
			vdiWithoutRef := &vdiv1alpha1.VdiInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vdi-without-ref",
					Namespace: vdiInstanceNs,
				},
			}

			// Create a controller for testing
			r := &VdiInstanceReconciler{
				Client: k8sClient,
				Scheme: runtime.NewScheme(),
			}

			// Call the function being tested
			status, err := r.checkKubeVirtVM(ctx, vdiWithoutRef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("KubeVirtVMRef is not specified"))
			Expect(status).To(Equal(""))
		})

		It("Should handle different VM states", func() {
			// Get the VM
			updatedVM := &kubevirtv1.VirtualMachine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: vmNamespace}, updatedVM)).To(Succeed())

			// Test "Starting" state
			updatedVM.Status.Ready = false
			updatedVM.Status.PrintableStatus = "Starting"
			Expect(k8sClient.Status().Update(ctx, updatedVM)).To(Succeed())

			r := &VdiInstanceReconciler{
				Client: k8sClient,
				Scheme: runtime.NewScheme(),
			}

			status, err := r.checkKubeVirtVM(ctx, vdiInstance)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("Starting"))

			// Test "Stopped" state
			updatedVM = &kubevirtv1.VirtualMachine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: vmNamespace}, updatedVM)).To(Succeed())
			updatedVM.Status.Ready = false
			updatedVM.Status.PrintableStatus = "Stopped"
			Expect(k8sClient.Status().Update(ctx, updatedVM)).To(Succeed())

			status, err = r.checkKubeVirtVM(ctx, vdiInstance)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("Stopped"))
		})
	})

	Context("When using VM connectivity diagnostics", func() {
		const (
			vmName          = "debug-test-vm"
			vmNamespace     = "default"
			vdiInstanceName = "debug-test-vdi-instance"
			vdiInstanceNs   = "default"
		)

		ctx := context.Background()

		// Create a VdiInstance to test with
		vdiInstance := &vdiv1alpha1.VdiInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdiInstanceName,
				Namespace: vdiInstanceNs,
			},
			Spec: vdiv1alpha1.VdiInstanceSpec{
				KubeVirtVMRef: &vdiv1alpha1.KubeVirtVMReference{
					Name:      vmName,
					Namespace: vmNamespace,
				},
			},
		}

		// Create a KubeVirt VM to test with
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				Running: ptr(true),
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"kubevirt.io/vm": vmName,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Devices: kubevirtv1.Devices{
								Interfaces: []kubevirtv1.Interface{
									{
										Name:  "default",
										Model: "virtio",
										InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
											Bridge: &kubevirtv1.InterfaceBridge{},
										},
									},
								},
							},
						},
						Networks: []kubevirtv1.Network{
							{
								Name: "default",
								NetworkSource: kubevirtv1.NetworkSource{
									Pod: &kubevirtv1.PodNetwork{},
								},
							},
						},
					},
				},
			},
		}

		// Create a VMI for the VM
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
				Labels: map[string]string{
					"kubevirt.io/vm": vmName,
				},
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: "Running",
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
					{
						IP:            "10.244.0.15",
						Name:          "default",
						InterfaceName: "eth0",
					},
				},
				NodeName: "worker-node1",
			},
		}

		BeforeEach(func() {
			// Create the VM and VMI in the test cluster
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())
			Expect(k8sClient.Create(ctx, vmi)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up the test VM and VMI
			Expect(k8sClient.Delete(ctx, vmi)).To(Succeed())
			Expect(k8sClient.Delete(ctx, vm)).To(Succeed())
		})

		It("Should run connectivity diagnostics without errors", func() {
			// Set the VM as ready
			updatedVM := &kubevirtv1.VirtualMachine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: vmNamespace}, updatedVM)).To(Succeed())

			updatedVM.Status.Ready = true
			updatedVM.Status.PrintableStatus = "Running"
			Expect(k8sClient.Status().Update(ctx, updatedVM)).To(Succeed())

			// Create a controller for testing
			r := &VdiInstanceReconciler{
				Client: k8sClient,
				Scheme: runtime.NewScheme(),
			}

			// Call the debug function
			err := r.debugKubeVirtVMConnectivity(ctx, vdiInstance)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should handle VM not being ready during diagnostics", func() {
			// Set the VM as not ready
			updatedVM := &kubevirtv1.VirtualMachine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: vmNamespace}, updatedVM)).To(Succeed())

			updatedVM.Status.Ready = false
			updatedVM.Status.PrintableStatus = "Starting"
			Expect(k8sClient.Status().Update(ctx, updatedVM)).To(Succeed())

			// Create a controller for testing
			r := &VdiInstanceReconciler{
				Client: k8sClient,
				Scheme: runtime.NewScheme(),
			}

			// Call the debug function
			err := r.debugKubeVirtVMConnectivity(ctx, vdiInstance)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// Helper function to create a pointer to a boolean
func ptr(b bool) *bool {
	return &b
}
