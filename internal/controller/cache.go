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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CacheByObject keeps the manager's informers to what DeskuBe made.
// Unfiltered, they hold every Secret and Pod in the cluster.
// Read anything else through the APIReader.
// Nodes stay whole: they are few, and their addresses are needed.
func CacheByObject() map[client.Object]cache.ByObject {
	made := cache.ByObject{Label: labels.SelectorFromSet(labels.Set{labelManagedBy: managedBy})}
	desktop, err := labels.NewRequirement(labelInstance, selection.Exists, nil)
	utilruntime.Must(err)
	return map[client.Object]cache.ByObject{
		&appsv1.Deployment{}:            made,
		&corev1.PersistentVolumeClaim{}: made,
		&corev1.Secret{}:                made,
		&corev1.Service{}:               made,
		&corev1.ServiceAccount{}:        made,
		&rbacv1.Role{}:                  made,
		&rbacv1.RoleBinding{}:           made,
		// The Deployment makes them, from a template with this label only.
		&corev1.Pod{}: {Label: labels.NewSelector().Add(*desktop)},
	}
}

// createOrUpdate looks for a child the cache misses on the API server.
// An older manager made children without labels; the mutation adds them.
// Creating one again fails, for a Service not even as AlreadyExists.
// Cost: one uncached read for each child made.
func (r *VdiInstanceReconciler) createOrUpdate(
	ctx context.Context, obj client.Object, mutate controllerutil.MutateFn,
) (controllerutil.OperationResult, error) {
	return controllerutil.CreateOrUpdate(ctx, cacheThenAPI{Client: r.Client, api: r.reader()}, obj, mutate)
}

type cacheThenAPI struct {
	client.Client
	api client.Reader
}

func (c cacheThenAPI) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	err := c.Client.Get(ctx, key, obj, opts...)
	if apierrors.IsNotFound(err) {
		return c.api.Get(ctx, key, obj, opts...)
	}
	return err
}
