/*
Copyright 2015-2024 Gravitational, Inc.

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

package token

import (
	"context"
	"fmt"

	apitypes "github.com/gravitational/teleport/api/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
)

// GenSchemaLabels returns Terraform schema for Labels type
func GenSchemaLabels(ctx context.Context) tfsdk.Attribute {
	return tfschema.GenSchemaLabels(ctx)
}

// GenSchemaBoolOptionFixed returns Terraform schema for BoolOption type.
// This differs from the original GenSchemaBoolOption in that it is fixed
// to handle the case where the value is not set.
func GenSchemaBoolOptionFixed(_ context.Context) tfsdk.Attribute {
	return tfsdk.Attribute{
		Optional: true,
		Type:     types.BoolType,
	}
}

// CopyFromBoolOptionFixed converts the tfschema Bool value to a Teleport
// BoolOption value.
func CopyFromBoolOptionFixed(diags diag.Diagnostics, tf attr.Value, o **apitypes.BoolOption) {
	v, ok := tf.(types.Bool)
	if !ok {
		diags.AddError("Error reading from Terraform object", fmt.Sprintf("Can not convert %T to types.Bool", tf))
		return
	}
	if !v.Null && !v.Unknown {
		value := apitypes.BoolOption{Value: v.Value}
		*o = &value
		return
	}
}

// CopyToBoolOptionFixed converts the Teleport BoolOption value to a tfschema
// Bool value.
func CopyToBoolOptionFixed(diags diag.Diagnostics, o *apitypes.BoolOption, t attr.Type, v attr.Value) attr.Value {
	value, ok := v.(types.Bool)
	if !ok {
		value = types.Bool{}
	}

	if o == nil {
		value.Null = true
		return value
	}

	value.Null = false
	value.Value = o.Value

	return value
}

func CopyFromLabels(diags diag.Diagnostics, v attr.Value, o *apitypes.Labels) {
	tfschema.CopyFromLabels(diags, v, o)
}

func CopyToLabels(diags diag.Diagnostics, o apitypes.Labels, t attr.Type, v attr.Value) attr.Value {
	return tfschema.CopyToLabels(diags, o, t, v)
}
