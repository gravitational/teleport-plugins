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

const TestUserName = "user"
const TestRoleName = "foo"
const TestRoleName2 = "bar"

type UserSuite struct {
	ResourceSuite
	counter int
}

func TestUser(t *testing.T) { suite.Run(t, &UserSuite{}) }

func (s *UserSuite) SetupSuite() {
	t := s.T()

	s.ResourceSuite.SetupSuite()

	var bootstrap integration.Bootstrap
	_, err := bootstrap.AddRole(TestRoleName, types.RoleSpecV5{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{
				Roles: []string{"admin"},
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

func (s *UserSuite) TestCreationAndDeletion() {
	t := s.T()

	s.startManager()
	kUser := s.createKubernetesUser("")

	var user types.User
	require.Eventually(t, func() bool {
		var err error
		user, err = s.admin.GetUser(kUser.GetName(), false)
		if err != nil {
			if !trace.IsNotFound(err) {
				require.Fail(t, "Unexpected error", "got an unexpected error: %s", err)
			}
			return false
		}
		return true
	}, time.Second, 100*time.Millisecond)
	require.Equal(t, user.GetRoles(), kUser.Spec.Roles)

	s.k8sClient.Delete(s.Context(), kUser)

	require.Eventually(t, func() bool {
		var err error
		user, err = s.admin.GetUser(kUser.GetName(), false)
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

func (s *UserSuite) TestUpdateExisting() {
	t := s.T()

	s.startManager()
	user := s.createTeleportUser("", TestRoleName)
	require.Equal(t, user.GetRoles(), []string{TestRoleName})

	_ = s.createKubernetesUser(user.GetName(), TestRoleName2)
	require.Eventually(t, func() bool {
		var err error
		user, err = s.admin.GetUser(user.GetName(), false)
		if err != nil {
			require.Fail(t, "Unexpected error", "got an unexpected error: %s", err)
			return false
		}
		roles := user.GetRoles()
		return len(roles) == 1 && roles[0] == TestRoleName2
	}, time.Second, 100*time.Millisecond)
}

func (s *UserSuite) createKubernetesUser(name string, roles ...string) *resources.UserV2 {
	t := s.T()
	t.Helper()

	if name == "" {
		name = s.generateName()
	}
	user := &resources.UserV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestNamespace,
		},
		Spec: resources.UserSpecV2{Roles: roles},
	}

	err := s.k8sClient.Create(s.Context(), user)
	require.NoError(t, err)

	return user
}

func (s *UserSuite) createTeleportUser(name string, roles ...string) types.User {
	t := s.T()
	t.Helper()

	if name == "" {
		name = s.generateName()
	}
	user, err := types.NewUser(name)
	require.NoError(t, err)
	for _, role := range roles {
		user.AddRole(role)
	}
	err = s.admin.CreateUser(s.Context(), user)
	require.NoError(t, err)
	return user
}

func (s *UserSuite) generateName() string {
	s.counter++
	return TestUserName + "-" + strconv.Itoa(s.counter)
}

func (s *UserSuite) startManager() {
	t := s.T()
	t.Helper()

	k8sManager, err := ctrl.NewManager(s.k8sConfig, ctrl.Options{Scheme: s.scheme, MetricsBindAddress: "0"})
	require.NoError(t, err)

	err = (&Reconciler{
		ReconcilerImpl: NewUserReconciler(k8sManager.GetClient(), k8sManager.GetScheme()),
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
