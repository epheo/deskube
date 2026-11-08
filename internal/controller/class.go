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
	"cmp"
	"context"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
)

const (
	classNameField = ".spec.className"
	classInUse     = "vdi.desku.be/class-in-use"
)

// Patch is for the finalizer alone. Cost: RBAC cannot tell fields
// apart, so the manager could change any class.
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdiclasses,verbs=get;list;watch;patch

// resolve is what a desktop runs with: each field the instance sets,
// else its class's, else the default. class is nil when the instance
// names none. Read these settings from its result, never from
// vdi.Spec: a class's would go unseen.
func resolve(vdi *vdiv1alpha1.VdiInstance, class *vdiv1alpha1.VdiClass) vdiv1alpha1.VdiClassSpec {
	own := vdi.Spec.DeepCopy()
	var out vdiv1alpha1.VdiClassSpec
	if class != nil {
		out = *class.Spec.DeepCopy()
	}
	out.Image = cmp.Or(own.Image, out.Image)
	out.ImagePullPolicy = cmp.Or(own.ImagePullPolicy, out.ImagePullPolicy)
	if len(own.ImagePullSecrets) > 0 {
		out.ImagePullSecrets = own.ImagePullSecrets
	}
	// Whole: a limit from one side under a request from the other is a
	// pod the API server refuses.
	if len(own.Resources.Requests)+len(own.Resources.Limits)+len(own.Resources.Claims) > 0 {
		out.Resources = own.Resources
	}
	if !own.Home.Size.IsZero() {
		out.Home.Size = own.Home.Size
	}
	if out.Home.Size.IsZero() {
		out.Home.Size = resource.MustParse("20Gi")
	}
	out.Home.StorageClassName = cmp.Or(own.Home.StorageClassName, out.Home.StorageClassName)
	out.IdleTimeout = cmp.Or(own.IdleTimeout, out.IdleTimeout)
	out.Expose.Type = cmp.Or(own.Expose.Type, out.Expose.Type, vdiv1alpha1.ExposeNodePort)
	out.Expose.WebUI = cmp.Or(own.Expose.WebUI, out.Expose.WebUI)
	out.Stream.Encoder = cmp.Or(own.Stream.Encoder, out.Stream.Encoder)
	out.Stream.KeyboardLayout = cmp.Or(own.Stream.KeyboardLayout, out.Stream.KeyboardLayout)
	out.NestedSandboxes = cmp.Or(own.NestedSandboxes, out.NestedSandboxes)
	out.Placement = mergePlacement(own.Placement, out.Placement)
	return out
}

func mergePlacement(own, class *vdiv1alpha1.Placement) *vdiv1alpha1.Placement {
	if own == nil {
		return class
	}
	if class == nil {
		return own
	}
	out := *own
	if len(out.NodeSelector) == 0 {
		out.NodeSelector = class.NodeSelector
	}
	if len(out.Tolerations) == 0 {
		out.Tolerations = class.Tolerations
	}
	out.Affinity = cmp.Or(out.Affinity, class.Affinity)
	out.PriorityClassName = cmp.Or(out.PriorityClassName, class.PriorityClassName)
	out.RuntimeClassName = cmp.Or(out.RuntimeClassName, class.RuntimeClassName)
	return &out
}

// classOf returns the instance's class, nil when it names none.
func (r *VdiInstanceReconciler) classOf(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance,
) (*vdiv1alpha1.VdiClass, error) {
	if vdi.Spec.ClassName == "" {
		return nil, nil
	}
	class := &vdiv1alpha1.VdiClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: vdi.Spec.ClassName}, class); err != nil {
		return nil, err
	}
	return class, nil
}

// waitForClass reports the missing class. A desktop not started stays
// so, a running one keeps its pod, and only stopping still works. The
// class watch brings the instance back.
func (r *VdiInstanceReconciler) waitForClass(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (ctrl.Result, error) {
	if vdi.Spec.Running != nil && !*vdi.Spec.Running {
		if err := r.stopDeployment(ctx, vdi); err != nil {
			return ctrl.Result{}, err
		}
	}
	setCondition(vdi, ConditionReady, false, "ClassNotFound", "VdiClass "+vdi.Spec.ClassName+" does not exist")
	vdi.Status.Phase = vdiv1alpha1.PhaseProvisioning
	return ctrl.Result{}, r.Status().Update(ctx, vdi)
}

func (r *VdiInstanceReconciler) stopDeployment(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) error {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: vdi.Name, Namespace: vdi.Namespace}, dep)
	if apierrors.IsNotFound(err) || (err == nil && !metav1.IsControlledBy(dep, vdi)) {
		return nil
	}
	if err != nil || ptr.Deref(dep.Spec.Replicas, 1) == 0 {
		return err
	}
	stopped := client.MergeFrom(dep.DeepCopy())
	dep.Spec.Replicas = ptr.To(int32(0))
	return r.Patch(ctx, dep, stopped)
}

// reconcileClass holds a class with a finalizer while a desktop names
// it: without its class a desktop can only be stopped.
func (r *VdiInstanceReconciler) reconcileClass(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	class := &vdiv1alpha1.VdiClass{}
	if err := r.Get(ctx, req.NamespacedName, class); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	desktops := &vdiv1alpha1.VdiInstanceList{}
	if err := r.List(ctx, desktops, client.MatchingFields{classNameField: class.Name}); err != nil {
		return reconcile.Result{}, err
	}
	used := len(desktops.Items) > 0
	held := controllerutil.ContainsFinalizer(class, classInUse)
	if !used && held {
		// Past the cache: a desktop it has not seen yet would lose its class.
		if err := r.reader().List(ctx, desktops); err != nil {
			return reconcile.Result{}, err
		}
		used = slices.ContainsFunc(desktops.Items, func(d vdiv1alpha1.VdiInstance) bool {
			return d.Spec.ClassName == class.Name
		})
	}
	// The API server takes no new finalizer on an object being deleted.
	if used == held || (used && !class.DeletionTimestamp.IsZero()) {
		return reconcile.Result{}, nil
	}
	patch := client.MergeFromWithOptions(class.DeepCopy(), client.MergeFromWithOptimisticLock{})
	if used {
		controllerutil.AddFinalizer(class, classInUse)
	} else {
		controllerutil.RemoveFinalizer(class, classInUse)
	}
	return reconcile.Result{}, r.Patch(ctx, class, patch)
}

// classOfInstance names the class an instance event concerns. An update
// maps the old object too, so a class an instance leaves hears of it.
func classOfInstance(_ context.Context, obj client.Object) []reconcile.Request {
	if name := obj.(*vdiv1alpha1.VdiInstance).Spec.ClassName; name != "" {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: name}}}
	}
	return nil
}

// desktopsOfClass names the instances a change to the class concerns.
func (r *VdiInstanceReconciler) desktopsOfClass(ctx context.Context, class client.Object) []reconcile.Request {
	desktops := &vdiv1alpha1.VdiInstanceList{}
	if err := r.List(ctx, desktops, client.MatchingFields{classNameField: class.GetName()}); err != nil {
		logf.FromContext(ctx).Error(err, "cannot list the desktops of a class", "class", class.GetName())
		return nil
	}
	requests := make([]reconcile.Request, 0, len(desktops.Items))
	for i := range desktops.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&desktops.Items[i])})
	}
	return requests
}

func indexClassName(obj client.Object) []string {
	if name := obj.(*vdiv1alpha1.VdiInstance).Spec.ClassName; name != "" {
		return []string{name}
	}
	return nil
}
