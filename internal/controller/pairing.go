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
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

const (
	// A Moonlight pairing request times out on its own, the PIN is useless after this.
	pairingTTL     = 10 * time.Minute
	requeuePairing = 5 * time.Second
)

// PairingClient is the slice of the Sunshine admin API the controller uses.
type PairingClient interface {
	PendingPairings(ctx context.Context) ([]sunshine.PendingPairing, error)
	ApprovePairing(ctx context.Context, id, pin, name string) (bool, error)
}

// NewPairingClient builds the admin API client for one desktop.
type NewPairingClient func(host string, basePort int32, username, password string) PairingClient

// reconcilePairing hands spec.pairing to Sunshine, records the outcome in
// status.pairing, and removes spec.pairing once the outcome is final.
// It runs after the instance status is written, with its own merge patch:
// a conflict on the instance must not lose an approval Sunshine accepted.
func (r *VdiInstanceReconciler) reconcilePairing(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, result ctrl.Result,
) (ctrl.Result, error) {
	before := vdi.DeepCopy()
	again := r.stepPairing(ctx, vdi)
	if !equality.Semantic.DeepEqual(before.Status.Pairing, vdi.Status.Pairing) {
		if err := r.Status().Patch(ctx, vdi, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, err
		}
	}
	req, st := vdi.Spec.Pairing, vdi.Status.Pairing
	if req != nil && st != nil && st.PIN == req.PIN && st.Phase != vdiv1alpha1.PairingPending {
		if err := r.clearPairingRequest(ctx, vdi, req.PIN); err != nil {
			return ctrl.Result{}, err
		}
	}
	if again > 0 && (result.RequeueAfter == 0 || again < result.RequeueAfter) {
		result.RequeueAfter = again
	}
	return result, nil
}

// stepPairing moves the request one step on and returns when to look
// again, zero when nothing waits.
func (r *VdiInstanceReconciler) stepPairing(ctx context.Context, vdi *vdiv1alpha1.VdiInstance) time.Duration {
	req, st := vdi.Spec.Pairing, vdi.Status.Pairing
	if req == nil {
		if st != nil && st.Phase == vdiv1alpha1.PairingPending {
			setPairing(st, vdiv1alpha1.PairingFailed, "withdrawn before Sunshine accepted it")
		}
		return 0
	}
	// The PIN tells a new request from a final one whose removal failed.
	if st == nil || st.PIN != req.PIN || st.RequestedAt == nil {
		st = &vdiv1alpha1.PairingStatus{PIN: req.PIN, RequestedAt: ptrNow()}
		vdi.Status.Pairing = st
	}
	if st.Phase != "" && st.Phase != vdiv1alpha1.PairingPending {
		return 0
	}
	left := pairingTTL - time.Since(st.RequestedAt.Time)
	if left <= 0 {
		setPairing(st, vdiv1alpha1.PairingFailed, "expired, Moonlight gave up waiting for the PIN")
		return 0
	}
	if vdi.Status.Phase != vdiv1alpha1.PhaseReady {
		setPairing(st, vdiv1alpha1.PairingPending, "desktop is not ready")
		return left
	}
	phase, message := r.approve(ctx, vdi, req)
	setPairing(st, phase, message)
	if phase == vdiv1alpha1.PairingPending {
		return min(requeuePairing, left)
	}
	return 0
}

// approve submits the PIN for each client waiting on Sunshine; the first
// one it is accepted for is paired.
func (r *VdiInstanceReconciler) approve(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, req *vdiv1alpha1.PairingSpec,
) (vdiv1alpha1.PairingPhase, string) {
	pod, err := r.findPod(ctx, vdi)
	if err != nil {
		return vdiv1alpha1.PairingPending, err.Error()
	}
	host, err := r.sunshineHost(vdi, pod)
	if err != nil {
		return vdiv1alpha1.PairingPending, err.Error()
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: adminSecretName(vdi), Namespace: vdi.Namespace}, secret); err != nil {
		return vdiv1alpha1.PairingPending, "admin secret: " + err.Error()
	}
	newClient := r.AdminClient
	if newClient == nil {
		newClient = func(host string, basePort int32, username, password string) PairingClient {
			return sunshine.NewClient(host, basePort, username, password)
		}
	}
	api := newClient(host, vdi.Status.BasePort, string(secret.Data["username"]), string(secret.Data["password"]))

	pending, err := api.PendingPairings(ctx)
	if err != nil {
		return vdiv1alpha1.PairingPending, "admin API: " + err.Error()
	}
	if len(pending) == 0 {
		return vdiv1alpha1.PairingPending, "no Moonlight client is waiting for a PIN"
	}
	name := req.ClientName
	if name == "" {
		name = "moonlight"
	}
	for _, p := range pending {
		ok, err := api.ApprovePairing(ctx, p.ID, req.PIN, name)
		if err != nil {
			return vdiv1alpha1.PairingPending, "admin API: " + err.Error()
		}
		if ok {
			logf.FromContext(ctx).Info("paired client", "client", p.Name, "address", p.Address)
			return vdiv1alpha1.PairingPaired, fmt.Sprintf("paired %s from %s", p.Name, p.Address)
		}
	}
	return vdiv1alpha1.PairingFailed, "PIN rejected, start pairing again in Moonlight"
}

// clearPairingRequest removes spec.pairing, unless the owner replaced the
// PIN since it was read.
func (r *VdiInstanceReconciler) clearPairingRequest(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, pin string,
) error {
	ops, err := json.Marshal([]map[string]string{
		{"op": "test", "path": "/spec/pairing/pin", "value": pin},
		{"op": "remove", "path": "/spec/pairing"},
	})
	if err != nil {
		return err
	}
	err = r.Patch(ctx, vdi, client.RawPatch(types.JSONPatchType, ops))
	if apierrors.IsInvalid(err) {
		// The test failed: a newer PIN waits, or the owner withdrew it.
		return nil
	}
	return err
}

func setPairing(st *vdiv1alpha1.PairingStatus, phase vdiv1alpha1.PairingPhase, message string) {
	st.Phase, st.Message = phase, message
	if phase == vdiv1alpha1.PairingPaired {
		st.PairedAt = ptrNow()
	}
}

func ptrNow() *metav1.Time {
	now := metav1.Now()
	return &now
}
