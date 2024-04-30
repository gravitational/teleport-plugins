// Code generated by _gen/main.go DO NOT EDIT
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

package provider

import (
	"context"

	apitypes "github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	token "github.com/gravitational/teleport-plugins/terraform/tfschema/token"
)

// dataSourceTeleportProvisionTokenType is the data source metadata type
type dataSourceTeleportProvisionTokenType struct{}

// dataSourceTeleportProvisionToken is the resource
type dataSourceTeleportProvisionToken struct {
	p Provider
}

// GetSchema returns the data source schema
func (r dataSourceTeleportProvisionTokenType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return token.GenSchemaProvisionTokenV2(ctx)
}

// NewDataSource creates the empty data source
func (r dataSourceTeleportProvisionTokenType) NewDataSource(_ context.Context, p tfsdk.Provider) (tfsdk.DataSource, diag.Diagnostics) {
	return dataSourceTeleportProvisionToken{
		p: *(p.(*Provider)),
	}, nil
}

// Read reads teleport ProvisionToken
func (r dataSourceTeleportProvisionToken) Read(ctx context.Context, req tfsdk.ReadDataSourceRequest, resp *tfsdk.ReadDataSourceResponse) {
	var id types.String
	diags := req.Config.GetAttribute(ctx, path.Root("metadata").AtName("name"), &id)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	provisionTokenI, err := r.p.Client.GetToken(ctx, id.Value)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading ProvisionToken", trace.Wrap(err), "token"))
		return
	}

    var state types.Object
	
	provisionToken := provisionTokenI.(*apitypes.ProvisionTokenV2)
	diags = token.CopyProvisionTokenV2ToTerraform(ctx, provisionToken, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
