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
	"embed" // enabled embed
	"fmt"

	"github.com/gravitational/trace"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/hashicorp/go-version"
)

var scheme = runtime.NewScheme()

var crds []apiextv1.CustomResourceDefinition

//go:embed *.teleport.dev_*.yaml
var crdFS embed.FS

type base struct {
	client  client.Client
	version *version.Version
	vMajor  string
}

func init() {
	// Initialize scheme.
	utilruntime.Must(apiextv1.AddToScheme(scheme))

	// Decode embedded CRDs.
	entries, err := crdFS.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			panic("no directories are expected to be embedded")
		}
		contents, err := crdFS.ReadFile(entry.Name())
		if err != nil {
			panic(err)
		}
		var crd apiextv1.CustomResourceDefinition
		if err := yaml.Unmarshal(contents, &crd); err != nil {
			panic(err)
		}
		crds = append(crds, crd)
	}
}

func newBase(restConfig *rest.Config, teleportVersion string) (base, error) {
	var (
		base base
		err  error
	)

	if base.client, err = client.New(restConfig, client.Options{Scheme: scheme}); err != nil {
		return base, trace.Wrap(err)
	}

	if base.version, err = version.NewVersion(teleportVersion); err != nil {
		return base, trace.Wrap(err)
	}

	verSegments := base.version.Segments()
	if nSegments := len(verSegments); nSegments != 3 {
		return base, trace.Errorf("teleport version %v contains wrong number of segments: %v instead of 3", base.version, nSegments)
	}

	base.vMajor = fmt.Sprintf("v%v", verSegments[0]) // take major part of teleport version

	return base, nil
}

func versionAnnotation(name string) string {
	return fmt.Sprintf("%s.teleport-operator-version", name)
}

func getCRDsMap() map[string]*apiextv1.CustomResourceDefinition {
	result := make(map[string]*apiextv1.CustomResourceDefinition, len(crds))
	for _, crd := range crds {
		result[crd.Name] = crd.DeepCopy()
	}
	return result
}

func getVersionsMap(crd *apiextv1.CustomResourceDefinition) map[string]apiextv1.CustomResourceDefinitionVersion {
	result := make(map[string]apiextv1.CustomResourceDefinitionVersion, len(crd.Spec.Versions))
	for _, crdVersion := range crd.Spec.Versions {
		result[crdVersion.Name] = crdVersion
	}
	return result
}
