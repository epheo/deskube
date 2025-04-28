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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// VdiTemplateReconciler reconciles a VdiTemplate object
type VdiTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vdi.desku.be,resources=vditemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vditemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vditemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiinstances,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *VdiTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling VdiTemplate", "namespace", req.Namespace, "name", req.Name)

	// Fetch the VdiTemplate instance
	vdiTemplate := &vdiv1alpha1.VdiTemplate{}
	if err := r.Get(ctx, req.NamespacedName, vdiTemplate); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			logger.Info("VdiTemplate resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get VdiTemplate")
		return ctrl.Result{}, err
	}

	// Update the template status by counting associated VDI instances
	if err := r.updateTemplateStatus(ctx, vdiTemplate); err != nil {
		logger.Error(err, "Failed to update VdiTemplate status")
		return ctrl.Result{}, err
	}

	// Nothing else to do for the template - it's mostly a passive object
	// used when creating new VDI instances

	return ctrl.Result{RequeueAfter: 1 * time.Hour}, nil
}

// updateTemplateStatus updates the status of the VdiTemplate by counting how many
// VdiInstances reference this template and when it was last used.
func (r *VdiTemplateReconciler) updateTemplateStatus(ctx context.Context, vdiTemplate *vdiv1alpha1.VdiTemplate) error {
	logger := log.FromContext(ctx)

	// Get a list of all VdiInstances that were created from this template
	vdiInstanceList := &vdiv1alpha1.VdiInstanceList{}
	err := r.List(ctx, vdiInstanceList, client.InNamespace(vdiTemplate.Namespace))
	if err != nil {
		return fmt.Errorf("failed to list VDI instances: %w", err)
	}

	instanceCount := 0
	var lastUsedTime *metav1.Time

	// Count instances created from this template by checking the ownerReferences
	for _, instance := range vdiInstanceList.Items {
		// Check for reference to this template in a label or annotation
		if instance.Labels["vdi.desku.be/template-name"] == vdiTemplate.Name &&
			instance.Labels["vdi.desku.be/template-namespace"] == vdiTemplate.Namespace {
			instanceCount++

			// Track the most recent creation timestamp
			if lastUsedTime == nil || instance.CreationTimestamp.After(lastUsedTime.Time) {
				lastUsedTime = &instance.CreationTimestamp
			}
		}
	}

	// Only update if values have changed
	if vdiTemplate.Status.InstanceCount != int32(instanceCount) ||
		(lastUsedTime != nil && (vdiTemplate.Status.LastUsedTimestamp == nil ||
			!vdiTemplate.Status.LastUsedTimestamp.Equal(lastUsedTime))) {

		// Update status
		vdiTemplate.Status.InstanceCount = int32(instanceCount)
		vdiTemplate.Status.LastUsedTimestamp = lastUsedTime

		// Update ready condition
		meta.SetStatusCondition(&vdiTemplate.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "TemplateAvailable",
			Message:            fmt.Sprintf("Template is available with %d instances", instanceCount),
			LastTransitionTime: metav1.Now(),
		})

		if err := r.Status().Update(ctx, vdiTemplate); err != nil {
			logger.Error(err, "Failed to update VdiTemplate status")
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VdiTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vdiv1alpha1.VdiTemplate{}).
		// Watch VdiInstances that reference this template
		Watches(
			&vdiv1alpha1.VdiInstance{},
			handler.EnqueueRequestsFromMapFunc(r.findTemplatesForInstance),
		).
		Complete(r)
}

// findTemplatesForInstance returns requests for templates that might be referenced by the instance.
func (r *VdiTemplateReconciler) findTemplatesForInstance(ctx context.Context, instance client.Object) []reconcile.Request {
	vdiInstance, ok := instance.(*vdiv1alpha1.VdiInstance)
	if !ok {
		return nil
	}

	templateName := vdiInstance.Labels["vdi.desku.be/template-name"]
	templateNamespace := vdiInstance.Labels["vdi.desku.be/template-namespace"]
	if templateName == "" {
		return nil
	}

	// If templateNamespace is not set, use the instance's namespace
	if templateNamespace == "" {
		templateNamespace = vdiInstance.Namespace
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      templateName,
				Namespace: templateNamespace,
			},
		},
	}
}
