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
	"path/filepath"
	"time"

	. "github.com/gravitational/teleport-plugins/lib/testing"
	"github.com/gravitational/teleport-plugins/lib/testing/integration"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stretchr/testify/require"

	authv8 "github.com/gravitational/teleport-plugins/kubernetes/apis/auth/v8"
	//+kubebuilder:scaffold:imports
)

const TestNamespace = "teleport-testing"

type AuthSuite struct {
	Suite
	teleport  *integration.Integration
	admin     *integration.Client
	auth      *integration.AuthService
	scheme    *runtime.Scheme
	k8sConfig *rest.Config
	k8sClient kclient.Client
}

func (s *AuthSuite) SetupSuite() {
	var err error
	t := s.T()

	// We set such a big timeout because integration.NewFromEnv could start
	// downloading a Teleport *-bin.tar.gz file which can take a long time.
	ctx := s.SetContextTimeout(2 * time.Minute)

	s.teleport, err = integration.NewFromEnv(ctx)
	require.NoError(t, err)
	t.Cleanup(s.teleport.Close)

	s.auth, err = s.teleport.NewAuthService()
	require.NoError(t, err)
	s.StartApp(s.auth)

	s.admin, err = s.teleport.MakeAdmin(ctx, s.auth, "integration-admin")
	require.NoError(t, err)

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "crd")},
		ErrorIfCRDPathMissing: true,
	}

	s.k8sConfig, err = testEnv.Start()
	require.NoError(t, err)

	t.Cleanup(func() {
		err := testEnv.Stop()
		require.NoError(t, err)
	})

	s.scheme = runtime.NewScheme()

	err = corev1.AddToScheme(s.scheme)
	require.NoError(t, err)

	err = authv8.AddToScheme(s.scheme)
	require.NoError(t, err)

	s.k8sClient, err = kclient.New(s.k8sConfig, kclient.Options{Scheme: s.scheme})
	require.NoError(t, err)

	err = s.k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: TestNamespace}})
	require.NoError(t, err)
}
