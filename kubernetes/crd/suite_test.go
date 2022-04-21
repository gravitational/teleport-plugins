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

package crd

import (
	. "github.com/gravitational/teleport-plugins/lib/testing"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stretchr/testify/require"
)

type CRDSuite struct {
	Suite
	k8sConfig *rest.Config
	k8sClient kclient.Client
}

func (s *CRDSuite) SetupSuite() {
	var err error
	t := s.T()

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	var testEnv envtest.Environment
	s.k8sConfig, err = testEnv.Start()
	require.NoError(t, err)

	t.Cleanup(func() {
		err := testEnv.Stop()
		require.NoError(t, err)
	})

	s.k8sClient, err = kclient.New(s.k8sConfig, kclient.Options{Scheme: scheme})
	require.NoError(t, err)
}

func (s *CRDSuite) TearDownTest() {
	t := s.T()
	err := s.k8sClient.DeleteAllOf(s.Context(), &apiextv1.CustomResourceDefinition{})
	require.NoError(t, err)
}
