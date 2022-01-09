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

// Package resources contains API Schema definitions for the resources API group of different versions.
//+kubebuilder:object:generate=true
//+groupName=resources.teleport.dev
package resources

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/gravitational/teleport/api/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// versions is a list of supported Teleport resource versions.
	versions = []string{
		types.V1,
		types.V2,
		types.V3,
		types.V4,
	}

	// groupVersions is a list of group versions used to register these objects.
	groupVersions = make(map[string]schema.GroupVersion, len(versions))

	// schemeBuilder is used to add go types to the GroupVersionKind scheme
	schemeBuilder = runtime.SchemeBuilder{}

	// regexpNameVersion parses a versioned resource name e.g. UserV2, RoleV4.
	regexpNameVersion = regexp.MustCompile(`^(.+)(V\d+)$`)
)

func init() {
	for _, version := range versions {
		groupVersion := schema.GroupVersion{Group: "resources.teleport.dev", Version: version}
		groupVersions[version] = groupVersion
	}
}

// AddToScheme adds the types from all the group-versions to the given scheme.
func AddToScheme(scheme *runtime.Scheme) error {
	return schemeBuilder.AddToScheme(scheme)
}

func register(objects ...runtime.Object) {
	for _, obj := range objects {
		registerObject(obj)
	}
}

func registerObject(obj runtime.Object) {
	objType := reflect.TypeOf(obj)
	if objType.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("object type %s must be pointer to a struct", objType))
	}
	if elemType := objType.Elem(); elemType.Kind() == reflect.Struct {
		objType = elemType
	} else {
		panic(fmt.Sprintf("object type %s must be pointer to a struct", objType))
	}
	submatches := regexpNameVersion.FindStringSubmatch(objType.Name())
	if len(submatches) == 0 {
		panic(fmt.Sprintf("object type %s name is not versioned", objType))
	}
	kind := submatches[1]
	version := strings.ToLower(submatches[2])
	groupVersion, ok := groupVersions[version]
	if !ok {
		panic(fmt.Sprintf("group version %s is unknown", version))
	}
	groupVersionKind := groupVersion.WithKind(kind)
	schemeBuilder = append(schemeBuilder, func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypeWithName(groupVersionKind, obj)
		metav1.AddToGroupVersion(scheme, groupVersion)
		return nil
	})
}
