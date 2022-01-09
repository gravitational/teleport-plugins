/*
Copyright 2021 Gravitational, Inc.

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

package auth

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	authv8 "github.com/gravitational/teleport-plugins/kubernetes/apis/auth/v8"
	"github.com/gravitational/teleport-plugins/lib/testing/integration"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	//+kubebuilder:scaffold:imports
)

const TestIdentityUsername = "some-user"
const TestIdentityName = "teleport-testing-id"
const TestIdentitySecretName = "teleport-testing-secret"

type IdentitySuite struct {
	AuthSuite
	counter int
}

func TestIdentity(t *testing.T) { suite.Run(t, &IdentitySuite{}) }

func (s *IdentitySuite) SetupSuite() {
	t := s.T()

	s.AuthSuite.SetupSuite()

	var bootstrap integration.Bootstrap
	_, err := bootstrap.AddRole(TestIdentityUsername, types.RoleSpecV4{
		Options: types.RoleOptions{
			MaxSessionTTL: types.NewDuration(time.Hour),
		},
	})
	require.NoError(t, err)
	_, err = bootstrap.AddUserWithRoles(TestIdentityUsername, TestIdentityUsername)
	require.NoError(t, err)

	s.teleport.Bootstrap(s.Context(), s.auth, bootstrap.Resources())
}

func (s *IdentitySuite) TestSecretGeneration() {
	t := s.T()

	s.startWithManager(false, 0)
	identity := s.createIdentity(time.Hour)

	secret := s.waitSecretCreation(identity)

	identityStr := string(secret.Data[IdentityKey])
	client, err := s.teleport.NewSignedClient(s.Context(), s.auth, identity.Spec.Username, []client.Credentials{client.LoadIdentityFileFromString(identityStr)})
	require.NoError(t, err)
	_, err = client.Ping(s.Context())
	require.NoError(t, err)
}

func (s *IdentitySuite) TestSecretRegenerationOnUpdate() {
	t := s.T()

	s.startWithManager(false, 0)
	identity := s.createIdentity(time.Hour)

	secret := s.waitSecretCreation(identity)
	invalidData := []byte("foo")
	secret.Data[IdentityKey] = invalidData
	err := s.k8sClient.Update(s.Context(), secret)
	require.NoError(t, err)
	secret = s.waitSecretUpdate(identity, invalidData)

	identityStr := string(secret.Data[IdentityKey])
	client, err := s.teleport.NewSignedClient(s.Context(), s.auth, identity.Spec.Username, []client.Credentials{client.LoadIdentityFileFromString(identityStr)})
	require.NoError(t, err)
	_, err = client.Ping(s.Context())
	require.NoError(t, err)
}

func (s *IdentitySuite) TestOwnerRefEnabled() {
	t := s.T()

	s.startWithManager(true, 0)

	identity := s.createIdentity(time.Second)
	secret := s.waitSecretCreation(identity)
	require.NotEmpty(t, secret.OwnerReferences)
	require.Equal(t, secret.OwnerReferences[0].UID, identity.GetUID())
}

func (s *IdentitySuite) TestOwnerRefDisabled() {
	t := s.T()

	s.startWithManager(false, 0)

	identity := s.createIdentity(time.Second)
	secret := s.waitSecretCreation(identity)
	require.Empty(t, secret.OwnerReferences)
}

func (s *IdentitySuite) createIdentity(ttl time.Duration) *authv8.Identity {
	t := s.T()
	t.Helper()

	s.counter++
	identity := &authv8.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestIdentityName + "-" + strconv.Itoa(s.counter),
			Namespace: TestNamespace,
		},
		Spec: authv8.IdentitySpec{
			Username:   TestIdentityUsername,
			SecretName: TestIdentitySecretName + "-" + strconv.Itoa(s.counter),
			TTL:        &metav1.Duration{Duration: ttl},
		},
	}

	err := s.k8sClient.Create(s.Context(), identity)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := s.k8sClient.Delete(s.Context(), identity)
		if err != nil && !kerrors.IsNotFound(err) {
			require.NoError(t, err)
		}

		err = s.k8sClient.Delete(s.Context(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: identity.Spec.SecretName, Namespace: identity.Namespace}})
		if err != nil && !kerrors.IsNotFound(err) {
			require.NoError(t, err)
		}
	})

	return identity
}

func (s *IdentitySuite) startWithManager(ownerRefEnabled bool, refreshRate time.Duration) reconcile.Reconciler {
	t := s.T()
	t.Helper()

	k8sManager, err := ctrl.NewManager(s.k8sConfig, ctrl.Options{Scheme: s.scheme, MetricsBindAddress: "0"})
	require.NoError(t, err)

	err = SetupIndexes(s.Context(), k8sManager.GetCache())
	require.NoError(t, err)

	if refreshRate == 0 {
		refreshRate = time.Hour // assign something "infinite"
	}

	reconciler := &IdentityReconciler{
		Kube:            k8sManager.GetClient(),
		Scheme:          k8sManager.GetScheme(),
		Signer:          s.teleport.Tctl(s.auth),
		OwnerRefEnabled: ownerRefEnabled,
		RefreshRate:     refreshRate,
	}
	err = reconciler.SetupWithManager(k8sManager)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(s.Context())
	go func() {
		if err := k8sManager.Start(ctx); err != nil {
			panic(err) // We can't assert inside a goroutine so lets just panic.
		}
	}()
	t.Cleanup(func() {
		// This isn't really required but controller-runtime forcibly terminates the runnables
		// so termination errors flood test output a lot.
		time.Sleep(time.Second)
		cancel()
	})

	return reconciler
}

func (s *IdentitySuite) waitSecretCreation(identity *authv8.Identity) *corev1.Secret {
	t := s.T()
	t.Helper()

	var secret corev1.Secret
	require.Eventually(t, func() bool {
		err := s.k8sClient.Get(s.Context(), ktypes.NamespacedName{Namespace: identity.Namespace, Name: identity.Spec.SecretName}, &secret)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				require.Fail(t, "Unexpected error", "got an unexpected error: %s", err)
			}
			return false
		}
		return true
	}, 2*time.Second, 100*time.Millisecond)
	return &secret
}

func (s *IdentitySuite) waitSecretUpdate(identity *authv8.Identity, prevValue []byte) *corev1.Secret {
	t := s.T()
	t.Helper()

	var secret corev1.Secret
	require.Eventually(t, func() bool {
		err := s.k8sClient.Get(s.Context(), ktypes.NamespacedName{Namespace: identity.Namespace, Name: identity.Spec.SecretName}, &secret)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				require.Fail(t, "Unexpected error", "got an unexpected error: %s", err)
			}
			return false
		}
		return !bytes.Equal(secret.Data[IdentityKey], prevValue)
	}, 3*time.Second, 100*time.Millisecond)
	return &secret
}
