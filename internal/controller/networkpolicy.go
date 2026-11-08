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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

// The manager's pod label in config/manager/manager.yaml. kustomize
// prefixes the namespace, not labels; TestManagerPodMatchesDesktopPolicies
// holds the two together.
const (
	managerPodLabel = "control-plane"
	managerPodValue = "controller-manager"
)

// ensureNetworkPolicy closes the desktop pod to the rest of the cluster.
// Egress stays open: a desktop browses the internet.
func (r *VdiInstanceReconciler) ensureNetworkPolicy(
	ctx context.Context, vdi *vdiv1alpha1.VdiInstance, settings *vdiv1alpha1.VdiClassSpec,
) error {
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: vdi.Name, Namespace: vdi.Namespace}}
	_, err := r.createOrUpdate(ctx, policy, func() error {
		labelManaged(policy, vdi.Name)
		policy.Spec = desktopPolicy(vdi.Name, vdi.Status.BasePort, r.webOpen(settings), r.ManagerNamespace)
		return controllerutil.SetControllerReference(vdi, policy, r.Scheme)
	})
	return err
}

// desktopPolicy admits Moonlight's ports from any source: NodePort and
// load balancer traffic arrives SNATed from node addresses. The web port
// takes the manager's pods alone unless it is open.
func desktopPolicy(instance string, base int32, webOpen bool, managerNamespace string) networkingv1.NetworkPolicySpec {
	open := make([]networkingv1.NetworkPolicyPort, 0, len(sunshine.Ports))
	var web []networkingv1.NetworkPolicyPort
	for _, p := range sunshine.Ports {
		port := networkingv1.NetworkPolicyPort{
			Protocol: ptr.To(corev1.Protocol(p.Protocol)),
			Port:     ptr.To(intstr.FromInt32(base + p.Offset)),
		}
		if p.Offset == sunshine.OffsetWeb && !webOpen {
			web = append(web, port)
			continue
		}
		open = append(open, port)
	}
	spec := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{labelInstance: instance}},
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		Ingress:     []networkingv1.NetworkPolicyIngressRule{{Ports: open}},
	}
	if web != nil {
		spec.Ingress = append(spec.Ingress, networkingv1.NetworkPolicyIngressRule{
			Ports: web,
			From: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{corev1.LabelMetadataName: managerNamespace},
				},
				PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{managerPodLabel: managerPodValue}},
			}},
		})
	}
	return spec
}
