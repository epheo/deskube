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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// debugKubeVirtVMConnectivity performs extended connectivity checks on a KubeVirt VM
// This function is useful for troubleshooting VM connectivity issues
func (r *VdiInstanceReconciler) debugKubeVirtVMConnectivity(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	logger := log.FromContext(ctx).WithValues("vdiName", vdi.Name, "namespace", vdi.Namespace)

	// First verify the VM exists and get its status
	status, err := r.checkKubeVirtVM(ctx, vdi)
	if err != nil {
		logger.Error(err, "Failed to check KubeVirt VM")
		return fmt.Errorf("failed to check VM: %w", err)
	}

	if status != "Ready" {
		logger.Info("VM is not in Ready state, connectivity checks may fail", "status", status)
	}

	// Get the VM details for additional information
	vm := &kubevirtv1.VirtualMachine{}
	vmKey := types.NamespacedName{
		Name:      vdi.Spec.KubeVirtVMRef.Name,
		Namespace: vdi.Spec.KubeVirtVMRef.Namespace,
	}

	if err := r.Get(ctx, vmKey, vm); err != nil {
		return fmt.Errorf("failed to get VM details: %w", err)
	}

	// Log detailed VM information that might help with debugging connectivity
	logger.Info("VM Details",
		"running", vm.Spec.Running,
		"status", vm.Status.PrintableStatus,
		"ready", vm.Status.Ready,
		"created", vm.CreationTimestamp,
	)

	// Log VM network interfaces information
	if vm.Spec.Template != nil && vm.Spec.Template.Spec.Domain.Devices.Interfaces != nil {
		for i, iface := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
			logger.Info("VM Network Interface",
				"index", i,
				"name", iface.Name,
				"model", iface.Model,
				"macAddress", iface.MacAddress,
			)
		}
	}

	// Check for any VMI (Virtual Machine Instance)
	vmi := &kubevirtv1.VirtualMachineInstance{}
	vmiKey := types.NamespacedName{
		Name:      vm.Name,
		Namespace: vm.Namespace,
	}

	if err := r.Get(ctx, vmiKey, vmi); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("No VMI found for this VM - VM may not be running")
			return nil
		}
		logger.Error(err, "Failed to get VMI")
		return nil
	}

	// Log VMI status and network information
	logger.Info("VMI Details",
		"phase", vmi.Status.Phase,
		"nodeName", vmi.Status.NodeName,
		"interfaces", len(vmi.Status.Interfaces),
	)

	// Log network interfaces status
	for _, iface := range vmi.Status.Interfaces {
		logger.Info("VMI Network Interface Status",
			"name", iface.Name,
			"ipAddress", iface.IP,
			"interfaceName", iface.InterfaceName,
		)
	}

	return nil
}
