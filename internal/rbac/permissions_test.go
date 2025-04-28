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

package rbac

import (
	"testing"

	vdiv1alpha1 "github.com/deskube/deskube/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestHasLegacyPermission(t *testing.T) {
	tests := []struct {
		name       string
		vdi        *vdiv1alpha1.VdiInstance
		subject    SubjectInfo
		permission string
		expected   bool
	}{
		{
			name: "User with explicit permission",
			vdi: &vdiv1alpha1.VdiInstance{
				Spec: vdiv1alpha1.VdiInstanceSpec{
					UserAccess: []vdiv1alpha1.UserAccessPolicy{
						{
							Username:    "testuser",
							Permissions: []string{"view", "connect"},
						},
					},
				},
			},
			subject: SubjectInfo{
				Kind: "User",
				Name: "testuser",
			},
			permission: PermissionView,
			expected:   true,
		},
		{
			name: "User without permission",
			vdi: &vdiv1alpha1.VdiInstance{
				Spec: vdiv1alpha1.VdiInstanceSpec{
					UserAccess: []vdiv1alpha1.UserAccessPolicy{
						{
							Username:    "testuser",
							Permissions: []string{"view"},
						},
					},
				},
			},
			subject: SubjectInfo{
				Kind: "User",
				Name: "testuser",
			},
			permission: PermissionManage,
			expected:   false,
		},
		{
			name: "User in group",
			vdi: &vdiv1alpha1.VdiInstance{
				Spec: vdiv1alpha1.VdiInstanceSpec{
					UserAccess: []vdiv1alpha1.UserAccessPolicy{
						{
							Groups:      []string{"devs"},
							Permissions: []string{"view", "connect"},
						},
					},
				},
			},
			subject: SubjectInfo{
				Kind:   "User",
				Name:   "testuser",
				Groups: []string{"devs", "users"},
			},
			permission: PermissionConnect,
			expected:   true,
		},
		{
			name: "Admin user has all permissions",
			vdi: &vdiv1alpha1.VdiInstance{
				Spec: vdiv1alpha1.VdiInstanceSpec{
					UserAccess: []vdiv1alpha1.UserAccessPolicy{
						{
							Username:    "adminuser",
							Permissions: []string{"admin"},
						},
					},
				},
			},
			subject: SubjectInfo{
				Kind: "User",
				Name: "adminuser",
			},
			permission: PermissionManage,
			expected:   true,
		},
		{
			name: "No access configured",
			vdi: &vdiv1alpha1.VdiInstance{
				Spec: vdiv1alpha1.VdiInstanceSpec{},
			},
			subject: SubjectInfo{
				Kind: "User",
				Name: "testuser",
			},
			permission: PermissionView,
			expected:   false,
		},
		{
			name: "User not in configured access",
			vdi: &vdiv1alpha1.VdiInstance{
				Spec: vdiv1alpha1.VdiInstanceSpec{
					UserAccess: []vdiv1alpha1.UserAccessPolicy{
						{
							Username:    "otheruser",
							Permissions: []string{"view", "connect"},
						},
					},
				},
			},
			subject: SubjectInfo{
				Kind: "User",
				Name: "testuser",
			},
			permission: PermissionView,
			expected:   false,
		},
		{
			name: "Using new name field",
			vdi: &vdiv1alpha1.VdiInstance{
				Spec: vdiv1alpha1.VdiInstanceSpec{
					UserAccess: []vdiv1alpha1.UserAccessPolicy{
						{
							Type:        "user",
							Name:        "testuser",
							Permissions: []string{"view", "connect"},
						},
					},
				},
			},
			subject: SubjectInfo{
				Kind: "User",
				Name: "testuser",
			},
			permission: PermissionConnect,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hasLegacyPermission(tt.vdi, tt.subject, tt.permission)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestHasPermissionInPolicy(t *testing.T) {
	tests := []struct {
		name       string
		policy     vdiv1alpha1.UserAccessPolicy
		permission string
		expected   bool
	}{
		{
			name: "Direct permission match",
			policy: vdiv1alpha1.UserAccessPolicy{
				Permissions: []string{"view", "connect"},
			},
			permission: "view",
			expected:   true,
		},
		{
			name: "Admin permission grants everything",
			policy: vdiv1alpha1.UserAccessPolicy{
				Permissions: []string{"admin"},
			},
			permission: "manage",
			expected:   true,
		},
		{
			name: "Manage permission grants view and connect",
			policy: vdiv1alpha1.UserAccessPolicy{
				Permissions: []string{"manage"},
			},
			permission: "connect",
			expected:   true,
		},
		{
			name: "View permission grants connect",
			policy: vdiv1alpha1.UserAccessPolicy{
				Permissions: []string{"view"},
			},
			permission: "connect",
			expected:   true,
		},
		{
			name: "No matching permission",
			policy: vdiv1alpha1.UserAccessPolicy{
				Permissions: []string{"view"},
			},
			permission: "manage",
			expected:   false,
		},
		{
			name:       "Empty permissions",
			policy:     vdiv1alpha1.UserAccessPolicy{},
			permission: "view",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hasPermissionInPolicy(tt.policy, tt.permission)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}
