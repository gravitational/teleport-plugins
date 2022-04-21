/*
Copyright 2021-2022 Gravitational, Inc.

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

package resources

import (
	"context"
	"strconv"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/gravitational/teleport-plugins/kubernetes/apis/resources"
	"github.com/gravitational/teleport-plugins/lib/testing/integration"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	//+kubebuilder:scaffold:imports
)

type RoleSuite struct {
	ResourceSuite
	counter int
}

func TestRole(t *testing.T) { suite.Run(t, &RoleSuite{}) }

func (s *RoleSuite) SetupSuite() {
	t := s.T()

	s.ResourceSuite.SetupSuite()

	var bootstrap integration.Bootstrap
	_, err := bootstrap.AddRole(TestRoleName, types.RoleSpecV5{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{
				Roles: []string{"editor"},
			},
		},
	})
	require.NoError(t, err)
	_, err = bootstrap.AddRole(TestRoleName2, types.RoleSpecV5{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{
				Roles: []string{"admin"},
			},
		},
	})
	require.NoError(t, err)
	err = s.teleport.Bootstrap(s.Context(), s.auth, bootstrap.Resources())
	require.NoError(t, err)
}

func (s *RoleSuite) TestCreationAndDeletion() {
	t := s.T()

	s.startManager()
	kRole := s.createKubernetesRole("")

	var role types.Role
	require.Eventually(t, func() bool {
		var err error
		role, err = s.admin.GetRole(s.Context(), kRole.GetName())
		if err != nil {
			if !trace.IsNotFound(err) {
				require.Fail(t, "Unexpected error", "got an unexpected error: %s", err)
			}
			return false
		}
		return true
	}, time.Second, 100*time.Millisecond)
	require.Equal(t, role.GetAccessRequestConditions(types.Allow).Roles, kRole.Spec.Allow.Request.Roles)

	s.k8sClient.Delete(s.Context(), kRole)

	require.Eventually(t, func() bool {
		var err error
		role, err = s.admin.GetRole(s.Context(), kRole.GetName())
		if err != nil {
			if !trace.IsNotFound(err) {
				require.Fail(t, "Unexpected error", "got an unexpected error: %s", err)
				return false
			}
			return true
		}
		return false
	}, time.Second, 100*time.Millisecond)
}

func (s *RoleSuite) TestUpdateExisting() {
	t := s.T()

	s.startManager()
	role := s.createTeleportRole("", "admin")
	require.Equal(t, role.GetAccessRequestConditions(types.Allow).Roles, []string{"admin"})

	_ = s.createKubernetesRole(role.GetName(), TestRoleName2)
	require.Eventually(t, func() bool {
		var err error
		role, err = s.admin.GetRole(s.Context(), role.GetName())
		if err != nil {
			require.Fail(t, "Unexpected error", "got an unexpected error: %s", err)
			return false
		}
		roles := role.GetAccessRequestConditions(types.Allow).Roles
		return len(roles) == 1 && roles[0] == TestRoleName2
	}, time.Second, 100*time.Millisecond)
}

func (s *RoleSuite) createKubernetesRole(name string, roles ...string) *resources.RoleV5 {
	t := s.T()
	t.Helper()

	if name == "" {
		name = s.generateName()
	}
	role := &resources.RoleV5{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestNamespace,
		},
		Spec: resources.RoleSpecV5{
			Allow: types.RoleConditions{
				Request: &types.AccessRequestConditions{Roles: roles},
			},
		},
	}

	err := s.k8sClient.Create(s.Context(), role)
	require.NoError(t, err)

	return role
}

func (s *RoleSuite) createTeleportRole(name string, roles ...string) types.Role {
	t := s.T()
	t.Helper()

	if name == "" {
		name = s.generateName()
	}
	role, err := types.NewRole(name, types.RoleSpecV5{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{Roles: roles},
		},
	})
	require.NoError(t, err)
	err = s.admin.UpsertRole(s.Context(), role)
	require.NoError(t, err)
	return role
}

func (s *RoleSuite) generateName() string {
	s.counter++
	return TestRoleName + "-" + strconv.Itoa(s.counter)
}

func (s *RoleSuite) startManager() {
	t := s.T()
	t.Helper()

	k8sManager, err := ctrl.NewManager(s.k8sConfig, ctrl.Options{Scheme: s.scheme, MetricsBindAddress: "0"})
	require.NoError(t, err)

	err = (&Reconciler{
		ReconcilerImpl: NewRoleReconciler(k8sManager.GetClient(), k8sManager.GetScheme()),
		Client:         s.admin.Client,
	}).SetupWithManager(k8sManager)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(s.Context())
	go func() {
		if err := k8sManager.Start(ctx); err != nil {
			panic(err) // We can't assert inside a goroutine so lets just panic.
		}
	}()
	t.Cleanup(cancel)
}
