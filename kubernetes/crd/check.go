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
	"context"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hashicorp/go-version"

	"github.com/gravitational/trace"
)

type checker struct {
	base
}

// Check performs a check of installed CRDs in the cluster and their versions.
func Check(ctx context.Context, restConfig *rest.Config, operatorVersion string) error {
	base, err := newBase(restConfig, operatorVersion)
	if err != nil {
		return trace.Wrap(err)
	}

	checker := checker{base: base}

	for _, crd := range crds {
		if err := checker.do(ctx, crd.DeepCopy()); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

func (checker checker) do(ctx context.Context, source *apiextv1.CustomResourceDefinition) error {
	// Index CRD versions.
	crdVersions := getVersionsMap(source)

	var crd apiextv1.CustomResourceDefinition
	if err := checker.client.Get(ctx, client.ObjectKey{Name: source.Name}, &crd); err != nil {
		return trace.Wrap(err)
	}

	// Check that each installed CRD version is up-to-date.
	for _, crdVersion := range crd.Spec.Versions {
		if _, ok := crdVersions[crdVersion.Name]; !ok {
			// Ignore the version we're not aware of.
			continue
		}
		annotation := versionAnnotation(crdVersion.Name)
		operatorVersion := crd.Annotations[annotation]
		version, err := version.NewVersion(operatorVersion)
		if err != nil {
			return trace.Wrap(err,
				"failed to parse operator version annotation %s for CRD %s version %s. annotation value: [%s]",
				annotation,
				crd.Name,
				crdVersion.Name,
				operatorVersion,
			)
		}
		if version.LessThan(checker.version) {
			return trace.CompareFailed("resource version %s of %s is old. please run `teleport-operator install-crds` to update CRDs", crdVersion.Name, crd.Name)
		}
	}

	return nil
}
