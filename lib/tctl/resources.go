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

package tctl

import (
	"encoding/json"
	"io"

	"github.com/ghodss/yaml"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

func writeResourcesYAML(w io.Writer, resources []types.Resource) error {
	for i, resource := range resources {
		data, err := yaml.Marshal(resource)
		if err != nil {
			return trace.Wrap(err)
		}
		w.Write(data)
		if i != len(resources) {
			io.WriteString(w, "\n---\n")
		}
	}
	return nil
}

func readResourcesYAMLOrJSON(r io.Reader) ([]types.Resource, error) {
	var resources []types.Resource
	decoder := kyaml.NewYAMLOrJSONDecoder(r, 32768)
	for {
		var res streamResource
		err := decoder.Decode(&res)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, trace.Wrap(err)
		}
		resources = append(resources, res.Resource)
	}
	return resources, nil
}

type streamResource struct{ types.Resource }

func (res *streamResource) UnmarshalJSON(raw []byte) error {
	var header types.ResourceHeader
	if err := json.Unmarshal(raw, &header); err != nil {
		return trace.Wrap(err)
	}

	var resource types.Resource
	switch header.Kind {
	case types.KindUser:
		switch header.Version {
		case types.V2:
			resource = &types.UserV2{}
		default:
			return trace.BadParameter("unsupported resource version %s", header.Version)
		}
	case types.KindRole:
		switch header.Version {
		case types.V4:
			resource = &types.RoleV5{}
		default:
			return trace.BadParameter("unsupported resource version %s", header.Version)
		}
	case types.KindCertAuthority:
		switch header.Version {
		case types.V2:
			resource = &types.CertAuthorityV2{}
		default:
			return trace.BadParameter("unsupported resource version %s", header.Version)
		}
	default:
		return trace.BadParameter("unsupported resource kind %s", header.Kind)
	}

	if err := json.Unmarshal(raw, resource); err != nil {
		return trace.Wrap(err)
	}

	res.Resource = resource
	return nil
}
