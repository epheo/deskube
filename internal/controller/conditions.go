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

// Package controller implements the reconciliation logic for VDI resources
package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
)

/*
Kubernetes Conditions Implementation for VDI Instances

This file implements the standard Kubernetes conditions pattern for VDI instances.
Conditions provide a standardized way to communicate the status of resources to users
and other components in the system. The VDI controller uses the following conditions:

1. Ready: Aggregated condition indicating if the VDI instance is fully ready for use
2. Available: Indicates if the VDI instance is accessible for connections
3. Progressing: Indicates active changes being applied to the VDI instance
4. Degraded: Indicates the VDI instance is in a non-functional state

In addition to these standard conditions, we track component-specific conditions:
- VMReady: Status of the underlying KubeVirt VM
- StreamingReady: Status of the Sunshine streaming components
- NetworkReady: Status of the networking resources (services, routes, ingress)
- RBACReady: Status of RBAC permissions and role bindings

The Ready condition is an aggregation of these component conditions, becoming True
only when all components are ready.

The controller updates these conditions throughout the reconciliation process to
provide detailed status information to users and other system components.
*/

// Standard condition types for VDI instances
const (
	// ConditionReady indicates the VDI instance is ready for use.
	// This is an aggregate condition that becomes true only when all
	// component conditions are true.
	ConditionReady = "Ready"

	// ConditionAvailable indicates the VDI instance is available for connection.
	// This is typically true when the VM and networking are configured properly,
	// even if some optional components aren't ready.
	ConditionAvailable = "Available"

	// ConditionProgressing indicates the VDI instance reconciliation is in progress.
	// This is true during creation and when changes are being applied.
	ConditionProgressing = "Progressing"

	// ConditionDegraded indicates the VDI instance is in a degraded state.
	// This becomes true when there are persistent errors that prevent proper functioning.
	ConditionDegraded = "Degraded"

	// ConditionVMReady indicates the underlying KubeVirt VM is ready.
	// This tracks the status of the VM that the VDI instance connects to.
	ConditionVMReady = "VMReady"

	// ConditionStreamingReady indicates the streaming components are ready.
	// This tracks the status of the Sunshine streaming server components.
	ConditionStreamingReady = "StreamingReady"

	// ConditionNetworkReady indicates the networking components are ready.
	// This tracks the status of services, routes, and ingresses.
	ConditionNetworkReady = "NetworkReady"

	// ConditionRBACReady indicates the RBAC resources are configured.
	// This tracks the status of role bindings and permissions.
	ConditionRBACReady = "RBACReady"
)

// Condition status reasons
const (
	ReasonReconcileSuccess    = "ReconcileSuccess"
	ReasonReconcileError      = "ReconcileError"
	ReasonReconcileInProgress = "ReconcileInProgress"
	ReasonVMNotReady          = "VMNotReady"
	ReasonVMNotFound          = "VMNotFound"
	ReasonStreamingNotReady   = "StreamingNotReady"
	ReasonNetworkNotReady     = "NetworkNotReady"
	ReasonRBACNotReady        = "RBACNotReady"
)

// setCondition sets a condition on the VDI instance status
func setCondition(vdi *vdiv1alpha1.VdiInstance, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.NewTime(time.Now())
	existingCondition := findCondition(vdi, conditionType)

	if existingCondition == nil {
		// Create new condition if it doesn't exist
		condition := metav1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		}
		vdi.Status.Conditions = append(vdi.Status.Conditions, condition)
	} else if existingCondition.Status != status || existingCondition.Reason != reason || existingCondition.Message != message {
		// Update existing condition if values changed
		existingCondition.Status = status
		existingCondition.Reason = reason
		existingCondition.Message = message
		existingCondition.LastTransitionTime = now
		existingCondition.ObservedGeneration = vdi.Generation
	}
}

// findCondition finds a condition by type in the VDI instance status
func findCondition(vdi *vdiv1alpha1.VdiInstance, conditionType string) *metav1.Condition {
	for i := range vdi.Status.Conditions {
		if vdi.Status.Conditions[i].Type == conditionType {
			return &vdi.Status.Conditions[i]
		}
	}
	return nil
}

// updateReadyCondition updates the Ready condition based on other conditions
func updateReadyCondition(vdi *vdiv1alpha1.VdiInstance) {
	vmReady := isConditionTrue(vdi, ConditionVMReady)
	streamingReady := isConditionTrue(vdi, ConditionStreamingReady)
	networkReady := isConditionTrue(vdi, ConditionNetworkReady)
	rbacReady := isConditionTrue(vdi, ConditionRBACReady)

	if vmReady && streamingReady && networkReady && rbacReady {
		setCondition(vdi, ConditionReady, metav1.ConditionTrue, ReasonReconcileSuccess,
			"VDI instance is ready for use")

		setCondition(vdi, ConditionAvailable, metav1.ConditionTrue, ReasonReconcileSuccess,
			"VDI instance is available for connection")
	} else {
		reasons := []string{}
		message := "VDI instance is not ready: "

		if !vmReady {
			reasons = append(reasons, ReasonVMNotReady)
			message += "VM not ready. "
		}

		if !streamingReady {
			reasons = append(reasons, ReasonStreamingNotReady)
			message += "Streaming not ready. "
		}

		if !networkReady {
			reasons = append(reasons, ReasonNetworkNotReady)
			message += "Network not ready. "
		}

		if !rbacReady {
			reasons = append(reasons, ReasonRBACNotReady)
			message += "RBAC not ready."
		}

		reason := ReasonReconcileInProgress
		if len(reasons) > 0 {
			reason = reasons[0]
		}

		setCondition(vdi, ConditionReady, metav1.ConditionFalse, reason, message)
		setCondition(vdi, ConditionAvailable, metav1.ConditionFalse, reason, message)
	}
}

// isConditionTrue checks if a condition has status true
func isConditionTrue(vdi *vdiv1alpha1.VdiInstance, conditionType string) bool {
	condition := findCondition(vdi, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// isConditionFalse checks if a condition has status false
func isConditionFalse(vdi *vdiv1alpha1.VdiInstance, conditionType string) bool {
	condition := findCondition(vdi, conditionType)
	return condition != nil && condition.Status == metav1.ConditionFalse
}

// isAnyConditionTrue checks if any of the given condition types has status true
func isAnyConditionTrue(vdi *vdiv1alpha1.VdiInstance, conditionTypes ...string) bool {
	for _, condType := range conditionTypes {
		if isConditionTrue(vdi, condType) {
			return true
		}
	}
	return false
}

// initializeConditions sets all the standard conditions on a newly created VDI instance
func initializeConditions(vdi *vdiv1alpha1.VdiInstance) {
	now := metav1.NewTime(time.Now())

	vdi.Status.Conditions = []metav1.Condition{
		{
			Type:               ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonReconcileInProgress,
			Message:            "VDI instance is being initialized",
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		},
		{
			Type:               ConditionAvailable,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonReconcileInProgress,
			Message:            "VDI instance is not yet available",
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		},
		{
			Type:               ConditionProgressing,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonReconcileInProgress,
			Message:            "VDI instance reconciliation has started",
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		},
		{
			Type:               ConditionVMReady,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonReconcileInProgress,
			Message:            "Checking VM status",
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		},
		{
			Type:               ConditionStreamingReady,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonReconcileInProgress,
			Message:            "Streaming components not yet configured",
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		},
		{
			Type:               ConditionNetworkReady,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonReconcileInProgress,
			Message:            "Network components not yet configured",
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		},
		{
			Type:               ConditionRBACReady,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonReconcileInProgress,
			Message:            "RBAC resources not yet configured",
			LastTransitionTime: now,
			ObservedGeneration: vdi.Generation,
		},
	}
}
