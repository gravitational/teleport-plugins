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
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/gravitational/teleport-plugins/lib/backoff"
	"github.com/gravitational/teleport-plugins/terraform/tfschema"
	apitypes "github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
)

// resourceTeleportDatabaseType is the resource metadata type
type resourceTeleportDatabaseType struct{}

// resourceTeleportDatabase is the resource
type resourceTeleportDatabase struct {
	p Provider
}

// GetSchema returns the resource schema
func (r resourceTeleportDatabaseType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfschema.GenSchemaDatabaseV3(ctx)
}

// NewResource creates the empty resource
func (r resourceTeleportDatabaseType) NewResource(_ context.Context, p tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	return resourceTeleportDatabase{
		p: *(p.(*Provider)),
	}, nil
}

// Create creates the provision token
func (r resourceTeleportDatabase) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	database := &apitypes.DatabaseV3{}
	diags = tfschema.CopyDatabaseV3FromTerraform(ctx, plan, database)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	

	_, err := r.p.Client.GetDatabase(ctx, database.Metadata.Name)
	if !trace.IsNotFound(err) {
		if err == nil {
			n := database.Metadata.Name
			existErr := fmt.Sprintf("Database exists in Teleport. Either remove it (tctl rm db/%v)"+
				" or import it to the existing state (terraform import teleport_app.%v %v)", n, n, n)

			resp.Diagnostics.Append(diagFromErr("Database exists in Teleport", trace.Errorf(existErr)))
			return
		}

		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Wrap(err), "db"))
		return
	}

	err = database.CheckAndSetDefaults()
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error setting Database defaults", trace.Wrap(err), "db"))
		return
	}

	err = r.p.Client.CreateDatabase(ctx, database)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error creating Database", trace.Wrap(err), "db"))
		return
	}

	id := database.Metadata.Name
	var databaseI apitypes.Database

	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		databaseI, err = r.p.Client.GetDatabase(ctx, id)
		if trace.IsNotFound(err) {
			if bErr := backoff.Do(ctx); bErr != nil {
				resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Wrap(err), "db"))
				return
			}
			if tries >= r.p.RetryConfig.MaxTries {
				diagMessage := fmt.Sprintf("Error reading Database (tried %d times)", tries)
				resp.Diagnostics.Append(diagFromWrappedErr(diagMessage, trace.Wrap(err), "db"))
				return
			}
			continue
		}
		break
	}

	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Wrap(err), "db"))
		return
	}

	database, ok := databaseI.(*apitypes.DatabaseV3)
	if !ok {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Errorf("Can not convert %T to DatabaseV3", databaseI), "db"))
		return
	}

	diags = tfschema.CopyDatabaseV3ToTerraform(ctx, *database, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Attrs["id"] = types.String{Value: database.Metadata.Name}

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read reads teleport Database
func (r resourceTeleportDatabase) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var state types.Object
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var id types.String
	diags = req.State.GetAttribute(ctx, tftypes.NewAttributePath().WithAttributeName("metadata").WithAttributeName("name"), &id)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	databaseI, err := r.p.Client.GetDatabase(ctx, id.Value)
	if trace.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Wrap(err), "db"))
		return
	}

	database := databaseI.(*apitypes.DatabaseV3)
	diags = tfschema.CopyDatabaseV3ToTerraform(ctx, *database, &state)
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

// Update updates teleport Database
func (r resourceTeleportDatabase) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	database := &apitypes.DatabaseV3{}
	diags = tfschema.CopyDatabaseV3FromTerraform(ctx, plan, database)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := database.Metadata.Name

	err := database.CheckAndSetDefaults()
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating Database", err, "db"))
		return
	}

	databaseBefore, err := r.p.Client.GetDatabase(ctx, name)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", err, "db"))
		return
	}

	err = r.p.Client.UpdateDatabase(ctx, database)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating Database", err, "db"))
		return
	}

	var databaseI apitypes.Database

	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		databaseI, err = r.p.Client.GetDatabase(ctx, name)
		if err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", err, "db"))
			return
		}
		if databaseBefore.GetMetadata().ID != databaseI.GetMetadata().ID {
			break
		}

		if bErr := backoff.Do(ctx); bErr != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Wrap(err), "db"))
			return
		}
		if tries >= r.p.RetryConfig.MaxTries {
			diagMessage := fmt.Sprintf("Error reading Database (tried %d times)", tries)
			resp.Diagnostics.Append(diagFromWrappedErr(diagMessage, trace.Wrap(err), "db"))
			return
		}
	}
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Wrap(err), "db"))	
		return
	}

	database = databaseI.(*apitypes.DatabaseV3)
	diags = tfschema.CopyDatabaseV3ToTerraform(ctx, *database, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes Teleport Database
func (r resourceTeleportDatabase) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var id types.String
	diags := req.State.GetAttribute(ctx, tftypes.NewAttributePath().WithAttributeName("metadata").WithAttributeName("name"), &id)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.p.Client.DeleteDatabase(ctx, id.Value)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error deleting DatabaseV3", trace.Wrap(err), "db"))
		return
	}

	resp.State.RemoveResource(ctx)
}

// ImportState imports Database state
func (r resourceTeleportDatabase) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	databaseI, err := r.p.Client.GetDatabase(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Database", trace.Wrap(err), "db"))
		return
	}

	database := databaseI.(*apitypes.DatabaseV3)

	var state types.Object

	diags := resp.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = tfschema.CopyDatabaseV3ToTerraform(ctx, *database, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Attrs["id"] = types.String{Value: database.Metadata.Name}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
