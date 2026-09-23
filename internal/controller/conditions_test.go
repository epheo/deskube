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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
)

func TestSetCondition(t *testing.T) {
	// Setup
	vdi := &vdiv1alpha1.VdiInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-instance",
			Namespace:  "default",
			Generation: 1,
		},
	}

	// Test initial condition creation
	setCondition(vdi, ConditionReady, metav1.ConditionFalse, "TestReason", "Test message")

	assert.Len(t, vdi.Status.Conditions, 1, "Expected 1 condition")
	assert.Equal(t, ConditionReady, vdi.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionFalse, vdi.Status.Conditions[0].Status)
	assert.Equal(t, "TestReason", vdi.Status.Conditions[0].Reason)
	assert.Equal(t, "Test message", vdi.Status.Conditions[0].Message)
	assert.Equal(t, int64(1), vdi.Status.Conditions[0].ObservedGeneration)

	// Test updating existing condition
	origTime := vdi.Status.Conditions[0].LastTransitionTime
	time.Sleep(10 * time.Millisecond) // Ensure time changes

	setCondition(vdi, ConditionReady, metav1.ConditionTrue, "UpdatedReason", "Updated message")

	assert.Len(t, vdi.Status.Conditions, 1, "Expected condition count to remain at 1")
	assert.Equal(t, ConditionReady, vdi.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, vdi.Status.Conditions[0].Status)
	assert.Equal(t, "UpdatedReason", vdi.Status.Conditions[0].Reason)
	assert.Equal(t, "Updated message", vdi.Status.Conditions[0].Message)
	assert.NotEqual(t, origTime, vdi.Status.Conditions[0].LastTransitionTime,
		"Expected LastTransitionTime to be updated")
}

func TestInitializeConditions(t *testing.T) {
	// Setup
	vdi := &vdiv1alpha1.VdiInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-init",
			Namespace:  "default",
			Generation: 2,
		},
	}

	// Initialize conditions
	initializeConditions(vdi)

	// Validate all required conditions were created with expected values
	conditionTypes := []string{
		ConditionReady,
		ConditionAvailable,
		ConditionProgressing,
		ConditionVMReady,
		ConditionStreamingReady,
		ConditionNetworkReady,
		ConditionRBACReady,
	}

	assert.Len(t, vdi.Status.Conditions, len(conditionTypes),
		"Expected all standard conditions to be initialized")

	// Check if all expected conditions exist and have correct initial values
	for _, condType := range conditionTypes {
		cond := findCondition(vdi, condType)
		assert.NotNil(t, cond, "Expected condition %s to be set", condType)

		// Progressing should initially be True, others False
		if condType == ConditionProgressing {
			assert.Equal(t, metav1.ConditionTrue, cond.Status,
				"Expected %s condition to be True initially", condType)
		} else {
			assert.Equal(t, metav1.ConditionFalse, cond.Status,
				"Expected %s condition to be False initially", condType)
		}

		assert.Equal(t, int64(2), cond.ObservedGeneration,
			"Expected condition to record current generation")
	}
}

func TestUpdateReadyCondition(t *testing.T) {
	// Setup
	vdi := &vdiv1alpha1.VdiInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ready",
			Namespace:  "default",
			Generation: 1,
		},
	}

	// Initialize with all sub-conditions as false
	initializeConditions(vdi)

	// Test 1: With all dependencies false, Ready should be false
	updateReadyCondition(vdi)
	readyCond := findCondition(vdi, ConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)

	// Test 2: With all dependencies true, Ready should be true
	setCondition(vdi, ConditionVMReady, metav1.ConditionTrue, ReasonReconcileSuccess, "VM is ready")
	setCondition(vdi, ConditionStreamingReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Streaming is ready")
	setCondition(vdi, ConditionNetworkReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Network is ready")
	setCondition(vdi, ConditionRBACReady, metav1.ConditionTrue, ReasonReconcileSuccess, "RBAC is ready")

	updateReadyCondition(vdi)
	readyCond = findCondition(vdi, ConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)

	// Test 3: With one dependency false, Ready should be false
	setCondition(vdi, ConditionVMReady, metav1.ConditionFalse, ReasonVMNotReady, "VM is not ready")

	updateReadyCondition(vdi)
	readyCond = findCondition(vdi, ConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonVMNotReady, readyCond.Reason)
}
