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

package services

import (
	"encoding/json"
	"fmt"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
)

// ValidateReverseTunnel validates the OIDC connector and sets default values
func ValidateReverseTunnel(rt types.ReverseTunnel) error {
	if err := rt.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	for _, addr := range rt.GetDialAddrs() {
		if _, err := utils.ParseAddr(addr); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// ReverseTunnelSpecV2Schema is JSON schema for reverse tunnel spec
const ReverseTunnelSpecV2Schema = `{
	"type": "object",
	"additionalProperties": false,
	"required": ["cluster_name", "dial_addrs"],
	"properties": {
	  "cluster_name": {"type": "string"},
	  "type": {"type": "string"},
	  "dial_addrs": {
		"type": "array",
		"items": {
		  "type": "string"
		}
	  }
	}
  }`

// GetReverseTunnelSchema returns role schema with optionally injected
// schema for extensions
func GetReverseTunnelSchema() string {
	return fmt.Sprintf(V2SchemaTemplate, MetadataSchema, ReverseTunnelSpecV2Schema, DefaultDefinitions)
}

// UnmarshalReverseTunnel unmarshals the ReverseTunnel resource from JSON.
func UnmarshalReverseTunnel(bytes []byte, opts ...MarshalOption) (ReverseTunnel, error) {
	if len(bytes) == 0 {
		return nil, trace.BadParameter("missing tunnel data")
	}
	var h ResourceHeader
	err := json.Unmarshal(bytes, &h)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	cfg, err := CollectOptions(opts)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	switch h.Version {
	case V2:
		var r ReverseTunnelV2
		if cfg.SkipValidation {
			if err := utils.FastUnmarshal(bytes, &r); err != nil {
				return nil, trace.BadParameter(err.Error())
			}
		} else {
			if err := utils.UnmarshalWithSchema(GetReverseTunnelSchema(), &r, bytes); err != nil {
				return nil, trace.BadParameter(err.Error())
			}
		}
		if err := ValidateReverseTunnel(&r); err != nil {
			return nil, trace.Wrap(err)
		}
		if cfg.ID != 0 {
			r.SetResourceID(cfg.ID)
		}
		if !cfg.Expires.IsZero() {
			r.SetExpiry(cfg.Expires)
		}
		return &r, nil
	}
	return nil, trace.BadParameter("reverse tunnel version %v is not supported", h.Version)
}

// MarshalReverseTunnel marshals the ReverseTunnel resource to JSON.
func MarshalReverseTunnel(rt ReverseTunnel, opts ...MarshalOption) ([]byte, error) {
	cfg, err := CollectOptions(opts)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	switch reverseTunnel := rt.(type) {
	case *ReverseTunnelV2:
		if !cfg.PreserveResourceID {
			// avoid modifying the original object
			// to prevent unexpected data races
			copy := *reverseTunnel
			copy.SetResourceID(0)
			reverseTunnel = &copy
		}
		return utils.FastMarshal(reverseTunnel)
	default:
		return nil, trace.BadParameter("unrecognized reversetunnel version %T", rt)
	}
}
