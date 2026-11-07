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
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

// A Moonlight pairing request times out on its own, the PIN is useless after this.
const pairingTTL = 10 * time.Minute

// PairingClient is the slice of the Sunshine admin API the controller uses.
type PairingClient interface {
	PendingPairings(ctx context.Context) ([]sunshine.PendingPairing, error)
	ApprovePairing(ctx context.Context, id, pin, name string) (bool, error)
}

// VdiPairingReconciler hands the PIN to Sunshine on the owner's behalf.
type VdiPairingReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ProbeExternal bool
	// NewClient builds the admin API client; tests inject a fake.
	NewClient NewPairingClient
}

// NewPairingClient builds the admin API client for one desktop.
type NewPairingClient func(host string, basePort int32, username, password string) PairingClient

// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdipairings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vdi.desku.be,resources=vdipairings/status,verbs=get;update;patch

// Reconcile approves the pending pairing whose PIN matches.
func (r *VdiPairingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	pairing := &vdiv1alpha1.VdiPairing{}
	if err := r.Get(ctx, req.NamespacedName, pairing); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	result, err := r.reconcile(ctx, pairing)
	if isTransient(err) {
		log.V(1).Info("retrying after a stale read", "cause", err.Error())
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}
	return result, err
}

func (r *VdiPairingReconciler) reconcile(ctx context.Context, pairing *vdiv1alpha1.VdiPairing) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if pairing.Status.Phase == vdiv1alpha1.PairingPaired || pairing.Status.Phase == vdiv1alpha1.PairingFailed {
		return ctrl.Result{}, nil
	}
	if time.Since(pairing.CreationTimestamp.Time) > pairingTTL {
		return r.finish(ctx, pairing, vdiv1alpha1.PairingFailed, "expired, Moonlight gave up waiting for the PIN")
	}

	vdi := &vdiv1alpha1.VdiInstance{}
	key := types.NamespacedName{Name: pairing.Spec.InstanceRef, Namespace: pairing.Namespace}
	if err := r.Get(ctx, key, vdi); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return r.wait(ctx, pairing, "instance not found")
	}
	if vdi.Status.Phase != vdiv1alpha1.PhaseReady {
		return r.wait(ctx, pairing, "instance not ready")
	}

	host, err := r.adminHost(ctx, vdi)
	if err != nil {
		return r.wait(ctx, pairing, err.Error())
	}
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: vdi.Status.AdminSecret, Namespace: vdi.Namespace}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		return ctrl.Result{}, err
	}
	newClient := r.NewClient
	if newClient == nil {
		newClient = func(host string, basePort int32, username, password string) PairingClient {
			return sunshine.NewClient(host, basePort, username, password)
		}
	}
	api := newClient(host, vdi.Status.BasePort, string(secret.Data["username"]), string(secret.Data["password"]))

	pending, err := api.PendingPairings(ctx)
	if err != nil {
		return r.wait(ctx, pairing, "admin API: "+err.Error())
	}
	if len(pending) == 0 {
		return r.wait(ctx, pairing, "no Moonlight client is waiting for a PIN")
	}
	name := pairing.Spec.ClientName
	if name == "" {
		name = "moonlight"
	}
	for _, p := range pending {
		ok, err := api.ApprovePairing(ctx, p.ID, pairing.Spec.PIN, name)
		if err != nil {
			return r.wait(ctx, pairing, "admin API: "+err.Error())
		}
		if ok {
			log.Info("paired client", "instance", vdi.Name, "client", p.Name, "address", p.Address)
			return r.finish(ctx, pairing, vdiv1alpha1.PairingPaired, fmt.Sprintf("paired %s from %s", p.Name, p.Address))
		}
	}
	return r.finish(ctx, pairing, vdiv1alpha1.PairingFailed, "PIN rejected, start pairing again in Moonlight")
}

// adminHost is where the web UI answers from the manager's point of view.
func (r *VdiPairingReconciler) adminHost(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) (string, error) {
	if !r.ProbeExternal {
		// The Service no longer carries the web port unless expose.webUI.
		pods := &corev1.PodList{}
		if err := r.List(ctx, pods, client.InNamespace(vdi.Namespace),
			client.MatchingLabels{labelInstance: vdi.Name}); err != nil {
			return "", err
		}
		for i := range pods.Items {
			if p := &pods.Items[i]; p.DeletionTimestamp == nil && p.Status.PodIP != "" {
				return p.Status.PodIP, nil
			}
		}
		return "", fmt.Errorf("desktop pod has no IP yet")
	}
	host, _, err := net.SplitHostPort(vdi.Status.Address)
	if err != nil {
		return "", fmt.Errorf("instance has no address yet")
	}
	return host, nil
}

func (r *VdiPairingReconciler) wait(
	ctx context.Context, pairing *vdiv1alpha1.VdiPairing, message string,
) (ctrl.Result, error) {
	pairing.Status.Phase = vdiv1alpha1.PairingPending
	pairing.Status.Message = message
	if err := r.Status().Update(ctx, pairing); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *VdiPairingReconciler) finish(
	ctx context.Context, pairing *vdiv1alpha1.VdiPairing, phase vdiv1alpha1.VdiPairingPhase, message string,
) (ctrl.Result, error) {
	pairing.Status.Phase = phase
	pairing.Status.Message = message
	if phase == vdiv1alpha1.PairingPaired {
		now := metav1.Now()
		pairing.Status.PairedAt = &now
	}
	return ctrl.Result{}, r.Status().Update(ctx, pairing)
}

// SetupWithManager registers the controller.
func (r *VdiPairingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vdiv1alpha1.VdiPairing{}).
		Named("vdipairing").
		Complete(r)
}
