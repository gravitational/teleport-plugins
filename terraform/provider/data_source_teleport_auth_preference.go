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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
	apitypes "github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// dataSourceTeleportAuthPreferenceType is the data source metadata type
type dataSourceTeleportAuthPreferenceType struct{}

// dataSourceTeleportAuthPreference is the resource
type dataSourceTeleportAuthPreference struct {
	p Provider
}

// GetSchema returns the data source schema
func (r dataSourceTeleportAuthPreferenceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfschema.GenSchemaAuthPreferenceV2(ctx)
}

// NewDataSource creates the empty data source
func (r dataSourceTeleportAuthPreferenceType) NewDataSource(_ context.Context, p provider.Provider) (datasource.DataSource, diag.Diagnostics) {
	return dataSourceTeleportAuthPreference{
		p: *(p.(*Provider)),
	}, nil
}

// Read reads teleport AuthPreference
func (r dataSourceTeleportAuthPreference) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	authPreferenceI, err := r.p.Client.GetAuthPreference(ctx)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading AuthPreference", trace.Wrap(err), "cluster_auth_preference"))
		return
	}

    var state types.Object
	authPreference := authPreferenceI.(*apitypes.AuthPreferenceV2)
	diags := tfschema.CopyAuthPreferenceV2ToTerraform(ctx, *authPreference, &state)
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