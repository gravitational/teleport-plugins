/*
Copyright 2015-2022 Gravitational, Inc.

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
// NOTE: This chunk of code is used to facilitate reference documentation updates. Please, do not delete.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
)

var (
	dumps = map[string]func(context.Context) (tfsdk.Schema, diag.Diagnostics){
		"user":               tfschema.GenSchemaUserV2,
		"role":               tfschema.GenSchemaRoleV6,
		"provision_token":    tfschema.GenSchemaProvisionTokenV2,
		"github_connector":   tfschema.GenSchemaGithubConnectorV3,
		"saml_connector":     tfschema.GenSchemaSAMLConnectorV2,
		"oidc_connector":     tfschema.GenSchemaOIDCConnectorV3,
		"trusted_cluster":    tfschema.GenSchemaTrustedClusterV2,
		"app":                tfschema.GenSchemaAppV3,
		"database":           tfschema.GenSchemaDatabaseV3,
		"auth_preference":    tfschema.GenSchemaAuthPreferenceV2,
		"cluster_networking": tfschema.GenSchemaClusterNetworkingConfigV2,
		"session_recording":  tfschema.GenSchemaSessionRecordingConfigV2,
	}
)

func dump() {
	for name, fn := range dumps {
		fmt.Println(name)
		fmt.Println("--------------------------")

		schema, diags := fn(context.Background())
		if diags.HasError() {
			log.Fatalf("%v", diags)
		}
		dumpAttributes("", schema.Attributes)

		fmt.Println()
		fmt.Println()
	}
}

func dumpAttributes(prefix string, attrs map[string]tfsdk.Attribute) {
	for name, attr := range attrs {
		fmt.Printf("| %30s | %20s | %v | %s |\n", "`"+name+"`", typ(attr.Type), req(attr.Required), attr.Description)
	}

	for name, attr := range attrs {
		if attr.Attributes != nil {
			fmt.Println(prefix + name + ": " + attr.Description)
			fmt.Println("--------------------------")

			dumpAttributes(prefix+name+".", attr.Attributes.GetAttributes())
			fmt.Println()
		}
	}
}

func typ(typ attr.Type) string {
	if typ == nil {
		return "object"
	}

	switch typ.String() {
	case "types.StringType":
		return "string"
	case "TimeType(2006-01-02T15:04:05Z07:00)":
		return "RFC3339 time"
	case "DurationType":
		return "duration"
	case "types.BoolType":
		return "bool"
	case "types.MapType[types.StringType]":
		return "map of strings"
	case "types.ListType[types.StringType]":
		return "array of strings"
	case "types.MapType[types.ListType[types.StringType]]":
		return "map of string arrays"
	case "types.Int64Type":
		return "number"
	default:
		return typ.String()
	}
}

func req(r bool) string {
	if r {
		return "*"
	}
	return " "
}
