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

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
	apitypes "github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// dataSourceTeleportTrustedClusterType is the data source metadata type
type dataSourceTeleportTrustedClusterType struct{}

// dataSourceTeleportTrustedCluster is the resource
type dataSourceTeleportTrustedCluster struct {
	p Provider
}

// GetSchema returns the data source schema
func (r dataSourceTeleportTrustedClusterType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfschema.GenSchemaTrustedClusterV2(ctx)
}

// NewDataSource creates the empty data source
func (r dataSourceTeleportTrustedClusterType) NewDataSource(_ context.Context, p tfsdk.Provider) (tfsdk.DataSource, diag.Diagnostics) {
	return dataSourceTeleportTrustedCluster{
		p: *(p.(*Provider)),
	}, nil
}

// Read reads teleport TrustedCluster
func (r dataSourceTeleportTrustedCluster) Read(ctx context.Context, req tfsdk.ReadDataSourceRequest, resp *tfsdk.ReadDataSourceResponse) {
	var id types.String
	diags := req.Config.GetAttribute(ctx, path.Root("metadata").AtName("name"), &id)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	trustedClusterI, err := r.p.Client.GetTrustedCluster(ctx, id.Value)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading TrustedCluster", trace.Wrap(err), "trusted_cluster"))
		return
	}

    var state types.Object
	trustedCluster := trustedClusterI.(*apitypes.TrustedClusterV2)
	diags = tfschema.CopyTrustedClusterV2ToTerraform(ctx, *trustedCluster, &state)
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