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

package crd

import (
	"testing"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type InstallSuite struct {
	CRDSuite
}

func TestInstall(t *testing.T) { suite.Run(t, &InstallSuite{}) }

func (s *InstallSuite) TestAddNew() {
	t := s.T()

	results, err := Install(s.Context(), s.k8sConfig, "8.0.0", false)
	require.NoError(t, err)

	require.Len(t, results, 3)
	persistedCRDs := s.getPersistedCRDs()
	sourceCRDs := getCRDsMap()
	for _, result := range results {
		crd, ok := persistedCRDs[result.CRDName]
		require.True(t, ok)
		delete(persistedCRDs, result.CRDName)

		crdSrc, ok := sourceCRDs[crd.Name]
		require.True(t, ok)
		delete(sourceCRDs, crd.Name)

		// Check that versions' contents are persisted well.
		require.ElementsMatch(t, crdSrc.Spec.Versions, crd.Spec.Versions)

		// Check that we have an annotation for each CRD version we added and collect their names.
		versionNames := make([]string, len(crdSrc.Spec.Versions))
		for i, crdVersion := range crdSrc.Spec.Versions {
			require.Equal(t, "8.0.0", crd.Annotations[versionAnnotation(crdVersion.Name)])
			versionNames[i] = crdVersion.Name
		}
		require.ElementsMatch(t, versionNames, result.AddedCRDVersions)
		require.Empty(t, result.UpdatedCRDVersions)
	}
	require.Len(t, persistedCRDs, 0)
}

func (s *InstallSuite) TestUpdateExisting() {
	t := s.T()

	sourceCRDs := getCRDsMap()
	crdExisting := sourceCRDs["identities.auth.teleport.dev"].DeepCopy()
	crdExisting.Annotations[versionAnnotation("v8")] = "8.0.0"
	// Create CRD with empty schema
	crdExisting.Spec.Versions[0].Schema.OpenAPIV3Schema = &apiextv1.JSONSchemaProps{
		Type: "object",
	}
	err := s.k8sClient.Create(s.Context(), crdExisting)
	require.NoError(t, err)

	results, err := Install(s.Context(), s.k8sConfig, "8.0.1", false)
	require.NoError(t, err)

	require.Len(t, results, 3)
	persistedCRDs := s.getPersistedCRDs()
	for _, result := range results {
		crd, ok := persistedCRDs[result.CRDName]
		require.True(t, ok)
		delete(persistedCRDs, result.CRDName)

		crdSrc, ok := sourceCRDs[crd.Name]
		require.True(t, ok)
		delete(sourceCRDs, crd.Name)

		// Check that versions' contents are persisted well.
		require.ElementsMatch(t, crdSrc.Spec.Versions, crd.Spec.Versions)

		// Check that we have an annotation for each CRD version we wrote.
		for _, crdVersion := range crdSrc.Spec.Versions {
			require.Equal(t, "8.0.1", crd.Annotations[versionAnnotation(crdVersion.Name)])
		}

		if result.CRDName != crdExisting.Name {
			// Check that missing CRDs are marked as newly added.
			require.Len(t, result.AddedCRDVersions, 1)
			require.Empty(t, result.UpdatedCRDVersions)
		} else {
			// Check that existing one is marked as updated.
			require.Equal(t, map[string]string{"v8": "8.0.0"}, result.UpdatedCRDVersions)
			require.Empty(t, result.AddedCRDVersions)
		}
	}
	require.Len(t, persistedCRDs, 0)
}

func (s *InstallSuite) getPersistedCRDs() map[string]*apiextv1.CustomResourceDefinition {
	t := s.T()
	t.Helper()

	persisted := make(map[string]*apiextv1.CustomResourceDefinition)
	var crd apiextv1.CustomResourceDefinition

	err := s.k8sClient.Get(s.Context(), kclient.ObjectKey{Name: "identities.auth.teleport.dev"}, &crd)
	require.NoError(t, err)
	persisted[crd.Name] = crd.DeepCopy()

	err = s.k8sClient.Get(s.Context(), kclient.ObjectKey{Name: "roles.resources.teleport.dev"}, &crd)
	require.NoError(t, err)
	persisted[crd.Name] = crd.DeepCopy()

	err = s.k8sClient.Get(s.Context(), kclient.ObjectKey{Name: "users.resources.teleport.dev"}, &crd)
	require.NoError(t, err)
	persisted[crd.Name] = crd.DeepCopy()

	return persisted
}
