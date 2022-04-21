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
	"context"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/hashicorp/go-version"

	"github.com/gravitational/trace"
)

type InstallResult struct {
	CRDName            string
	OperationResult    string
	NewOperatorVersion string
	UpdatedCRDVersions map[string]string // mapping "updated version name" => "previous operator version"
	AddedCRDVersions   []string          // newly added versions
}

type installer struct {
	base
	force bool
}

// Install creates or updates CRDs in the cluster.
func Install(ctx context.Context, restConfig *rest.Config, operatorVersion string, force bool) ([]InstallResult, error) {
	base, err := newBase(restConfig, operatorVersion)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	installer := installer{base: base, force: force}

	var errs []error
	var results []InstallResult
	for _, crd := range crds {
		if result, err := installer.do(ctx, &crd); err != nil {
			errs = append(errs, trace.Wrap(err, "unable to install %s", crd.Name))
		} else {
			results = append(results, result)
		}
	}

	return results, trace.NewAggregate(errs...)
}

func (installer *installer) do(ctx context.Context, source *apiextv1.CustomResourceDefinition) (InstallResult, error) {
	// Index CRD versions.
	crdVersions := getVersionsMap(source)

	var (
		crd             apiextv1.CustomResourceDefinition
		updatedVersions map[string]string
		addedVersions   []string
	)

	crd.Name = source.Name
	operatorVersion := installer.version.String()
	result, err := controllerutil.CreateOrPatch(ctx, installer.client, &crd, func() error {
		// Reset the version sets.
		updatedVersions = make(map[string]string, len(crdVersions))
		addedVersions = make([]string, 0, len(crdVersions))

		if crd.Annotations == nil {
			crd.Annotations = make(map[string]string)
		}

		// If it's a new resource just write the CRD contents and set the operator version in annotations.
		if crd.ResourceVersion == "" {
			crd.Spec = *source.Spec.DeepCopy()
			for _, crdVersion := range crd.Spec.Versions {
				addedVersions = append(addedVersions, crdVersion.Name)
				crd.Annotations[versionAnnotation(crdVersion.Name)] = operatorVersion
			}
			return nil
		}

		// Otherwise, perform the patch of versions array.
		versions := make([]apiextv1.CustomResourceDefinitionVersion, len(crd.Spec.Versions))
		for i, crdVersion := range crd.Spec.Versions {
			annotation := versionAnnotation(crdVersion.Name)
			oldOperatorVersion := crd.Annotations[annotation]

			if !installer.force {
				// Check that our version is more recent than the one stored in cluster.

				version, err := version.NewVersion(oldOperatorVersion)
				if err != nil {
					return trace.Wrap(err,
						"failed to parse operator version annotation %s for CRD version %s. annotation value: [%s]",
						annotation,
						crdVersion.Name,
						oldOperatorVersion,
					)
				}

				if version.GreaterThan(installer.version) {
					// More recent version is already installed, lets keep it as is
					versions[i] = crdVersion
					continue
				}
			}

			if ourVersion, ok := crdVersions[crdVersion.Name]; ok {
				versions[i] = *ourVersion.DeepCopy()
				updatedVersions[crdVersion.Name] = oldOperatorVersion
				crd.Annotations[annotation] = operatorVersion
			} else {
				versions[i] = crdVersion // we don't know this version, lets keep it as is.
			}
		}
		for _, ourVersion := range crdVersions {
			if _, ok := updatedVersions[ourVersion.Name]; !ok {
				versions = append(versions, *ourVersion.DeepCopy())
				addedVersions = append(addedVersions, ourVersion.Name)
				crd.Annotations[versionAnnotation(ourVersion.Name)] = installer.version.String()
			}
		}
		crd.Spec.Versions = versions
		return nil
	})
	if err != nil {
		return InstallResult{}, trace.Wrap(err)
	}

	return InstallResult{
		CRDName:            crd.Name,
		OperationResult:    string(result),
		NewOperatorVersion: operatorVersion,
		UpdatedCRDVersions: updatedVersions,
		AddedCRDVersions:   addedVersions,
	}, nil
}
