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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CheckSuite struct {
	CRDSuite
}

func TestCheck(t *testing.T) { suite.Run(t, &CheckSuite{}) }

func (s *CheckSuite) TestEmpty() {
	t := s.T()

	err := Check(s.Context(), s.k8sConfig, "10.0.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), `"identities.auth.teleport.dev" not found`)
}

func (s *CheckSuite) TestInstalled() {
	t := s.T()

	_, err := Install(s.Context(), s.k8sConfig, "10.0.0", false)
	require.NoError(t, err)
	err = Check(s.Context(), s.k8sConfig, "10.0.0")
	require.NoError(t, err)
}

func (s *CheckSuite) TestInstalledOld() {
	t := s.T()

	_, err := Install(s.Context(), s.k8sConfig, "10.0.0", false)
	require.NoError(t, err)
	err = Check(s.Context(), s.k8sConfig, "10.0.1")
	require.Error(t, err)
	require.Contains(t, err.Error(), `resource version v10 of identities.auth.teleport.dev is old`)
}
