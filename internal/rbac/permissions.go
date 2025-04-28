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

// Package rbac provides permission checking and role-based access control functionality
package rbac

import (
	"context"
	"fmt"
	"strings"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// Common VDI permission constants
const (
	PermissionView    = "view"    // Can view the VDI instance details
	PermissionConnect = "connect" // Can connect to the VDI instance
	PermissionManage  = "manage"  // Can update configuration of the VDI instance
	PermissionAdmin   = "admin"   // Has full administrative access

	// Subject kinds
	SubjectKindUser           = "User"
	SubjectKindGroup          = "Group"
	SubjectKindServiceAccount = "ServiceAccount"
)

// SubjectInfo contains information about the user/group/serviceaccount requesting access
type SubjectInfo struct {
	// Kind is the type of subject (User, Group, ServiceAccount)
	Kind string
	// Name is the name of the subject
	Name string
	// Groups are the groups the subject belongs to (for User kind)
	Groups []string
}

// HasPermission checks if a subject has a specific permission for a VDI instance
// HasPermission checks if a subject has permission to perform actions on a VDI instance
func HasPermission(
	ctx context.Context,
	client kubernetes.Interface,
	vdi *vdiv1alpha1.VdiInstance,
	subject SubjectInfo,
	permission string,
) (bool, error) {
	// First check if RBAC integration is enabled
	if vdi.Spec.AccessControl == nil || !vdi.Spec.AccessControl.Enabled {
		// If RBAC is disabled, check legacy UserAccess rules
		return hasLegacyPermission(vdi, subject, permission)
	}

	// Map permission to the corresponding Kubernetes RBAC permission
	resource := schema.GroupVersionResource{
		Group:    "vdi.desku.be",
		Version:  "v1alpha1",
		Resource: "vdiinstances",
	}

	var verb string
	switch permission {
	case PermissionView:
		verb = "get"
	case PermissionConnect:
		verb = "get" // This might be a custom verb in a more mature implementation
	case PermissionManage:
		verb = "update"
	case PermissionAdmin:
		verb = "update" // Admin might need additional checks beyond RBAC
	default:
		return false, fmt.Errorf("unknown permission: %s", permission)
	}

	// Create a SubjectAccessReview to check if the subject has permission
	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: vdi.Namespace,
				Verb:      verb,
				Group:     resource.Group,
				Version:   resource.Version,
				Resource:  resource.Resource,
				Name:      vdi.Name,
			},
		},
	}

	// Set the user or group
	switch subject.Kind {
	case SubjectKindUser:
		sar.Spec.User = subject.Name
		sar.Spec.Groups = subject.Groups
	case SubjectKindGroup:
		sar.Spec.Groups = []string{subject.Name}
	case SubjectKindServiceAccount:
		sar.Spec.User = fmt.Sprintf("system:serviceaccount:%s:%s", vdi.Namespace, subject.Name)
	default:
		return false, fmt.Errorf("unknown subject kind: %s", subject.Kind)
	}

	// Perform the authorization check
	result, err := client.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("error checking permissions: %w", err)
	}

	return result.Status.Allowed, nil
}

// hasLegacyPermission checks permissions using the legacy UserAccess field
func hasLegacyPermission(vdi *vdiv1alpha1.VdiInstance, subject SubjectInfo, permission string) (bool, error) {
	// If no UserAccess is defined, deny access (unless it's the default Kubernetes admin)
	if len(vdi.Spec.UserAccess) == 0 {
		// Allow cluster admins (this is simplistic, real implementation would be more robust)
		if subject.Kind == SubjectKindUser && subject.Name == "system:admin" {
			return true, nil
		}
		return false, nil
	}

	// Check if the subject matches any UserAccess entry
	for _, access := range vdi.Spec.UserAccess {
		// Check for direct username match
		if subject.Kind == SubjectKindUser && (access.Username == subject.Name || access.Name == subject.Name) {
			return hasPermissionInPolicy(access, permission)
		}

		// Check for group matches
		if subject.Kind == SubjectKindGroup && contains(access.Groups, subject.Name) {
			return hasPermissionInPolicy(access, permission)
		}

		// Check if user is part of an allowed group
		if subject.Kind == SubjectKindUser {
			for _, group := range subject.Groups {
				if contains(access.Groups, group) {
					return hasPermissionInPolicy(access, permission)
				}
			}
		}
	}

	return false, nil
}

// hasPermissionInPolicy checks if a specific permission is granted by a UserAccessPolicy
func hasPermissionInPolicy(policy vdiv1alpha1.UserAccessPolicy, permission string) (bool, error) {
	// If no permissions are specified, deny
	if len(policy.Permissions) == 0 {
		return false, nil
	}

	// Check for specific permission or if admin permission is granted
	for _, perm := range policy.Permissions {
		// Direct match
		if strings.EqualFold(perm, permission) {
			return true, nil
		}

		// Admin permission grants everything
		if strings.EqualFold(perm, "admin") || strings.EqualFold(perm, PermissionAdmin) {
			return true, nil
		}

		// Manage permission grants view and connect
		if strings.EqualFold(perm, "manage") || strings.EqualFold(perm, PermissionManage) {
			if permission == PermissionView || permission == PermissionConnect {
				return true, nil
			}
		}

		// View permission grants connect (read-only access)
		if (strings.EqualFold(perm, "view") || strings.EqualFold(perm, PermissionView)) &&
			permission == PermissionConnect {
			return true, nil
		}
	}

	return false, nil
}

// contains checks if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
