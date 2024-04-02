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

	apitypes "github.com/gravitational/teleport/api/types"
	
	"github.com/gravitational/teleport/integrations/lib/backoff"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonboulle/clockwork"

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
)

// resourceTeleportSAMLConnectorType is the resource metadata type
type resourceTeleportSAMLConnectorType struct{}

// resourceTeleportSAMLConnector is the resource
type resourceTeleportSAMLConnector struct {
	p Provider
}

// GetSchema returns the resource schema
func (r resourceTeleportSAMLConnectorType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfschema.GenSchemaSAMLConnectorV2(ctx)
}

// NewResource creates the empty resource
func (r resourceTeleportSAMLConnectorType) NewResource(_ context.Context, p tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	return resourceTeleportSAMLConnector{
		p: *(p.(*Provider)),
	}, nil
}

// Create creates the SAMLConnector
func (r resourceTeleportSAMLConnector) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var err error
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	samlConnector := &apitypes.SAMLConnectorV2{}
	diags = tfschema.CopySAMLConnectorV2FromTerraform(ctx, plan, samlConnector)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	
	samlConnectorResource := samlConnector

	err = samlConnectorResource.CheckAndSetDefaults()
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error setting SAMLConnector defaults", trace.Wrap(err), "saml"))
		return
	}

	id := samlConnectorResource.Metadata.Name

	_, err = r.p.Client.GetSAMLConnector(ctx, id, true)
	if !trace.IsNotFound(err) {
		if err == nil {
			existErr := fmt.Sprintf("SAMLConnector exists in Teleport. Either remove it (tctl rm saml/%v)"+
				" or import it to the existing state (terraform import teleport_saml_connector.%v %v)", id, id, id)

			resp.Diagnostics.Append(diagFromErr("SAMLConnector exists in Teleport", trace.Errorf(existErr)))
			return
		}

		resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Wrap(err), "saml"))
		return
	}

	_, err = r.p.Client.CreateSAMLConnector(ctx, samlConnectorResource)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error creating SAMLConnector", trace.Wrap(err), "saml"))
		return
	}
		
	// Not really an inferface, just using the same name for easier templating.
	var samlConnectorI apitypes.SAMLConnector
	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		samlConnectorI, err = r.p.Client.GetSAMLConnector(ctx, id, true)
		if trace.IsNotFound(err) {
			if bErr := backoff.Do(ctx); bErr != nil {
				resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Wrap(bErr), "saml"))
				return
			}
			if tries >= r.p.RetryConfig.MaxTries {
				diagMessage := fmt.Sprintf("Error reading SAMLConnector (tried %d times) - state outdated, please import resource", tries)
				resp.Diagnostics.AddError(diagMessage, "saml")
			}
			continue
		}
		break
	}

	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Wrap(err), "saml"))
		return
	}

	samlConnectorResource, ok := samlConnectorI.(*apitypes.SAMLConnectorV2)
	if !ok {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Errorf("Can not convert %T to SAMLConnectorV2", samlConnectorI), "saml"))
		return
	}
	samlConnector = samlConnectorResource

	diags = tfschema.CopySAMLConnectorV2ToTerraform(ctx, samlConnector, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Attrs["id"] = types.String{Value: samlConnector.Metadata.Name}

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read reads teleport SAMLConnector
func (r resourceTeleportSAMLConnector) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var state types.Object
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var id types.String
	diags = req.State.GetAttribute(ctx, path.Root("metadata").AtName("name"), &id)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	samlConnectorI, err := r.p.Client.GetSAMLConnector(ctx, id.Value, true)
	if trace.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Wrap(err), "saml"))
		return
	}
	
	samlConnector := samlConnectorI.(*apitypes.SAMLConnectorV2)
	diags = tfschema.CopySAMLConnectorV2ToTerraform(ctx, samlConnector, &state)
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

// Update updates teleport SAMLConnector
func (r resourceTeleportSAMLConnector) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	samlConnector := &apitypes.SAMLConnectorV2{}
	diags = tfschema.CopySAMLConnectorV2FromTerraform(ctx, plan, samlConnector)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	samlConnectorResource := samlConnector


	if err := samlConnectorResource.CheckAndSetDefaults(); err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating SAMLConnector", err, "saml"))
		return
	}
	name := samlConnectorResource.Metadata.Name

	samlConnectorBefore, err := r.p.Client.GetSAMLConnector(ctx, name, true)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", err, "saml"))
		return
	}

	_, err = r.p.Client.UpsertSAMLConnector(ctx, samlConnectorResource)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating SAMLConnector", err, "saml"))
		return
	}
		
	// Not really an inferface, just using the same name for easier templating.
	var samlConnectorI apitypes.SAMLConnector

	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		samlConnectorI, err = r.p.Client.GetSAMLConnector(ctx, name, true)
		if err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", err, "saml"))
			return
		}
		if samlConnectorBefore.GetMetadata().Revision != samlConnectorI.GetMetadata().Revision || true {
			break
		}

		if err := backoff.Do(ctx); err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Wrap(err), "saml"))
			return
		}
		if tries >= r.p.RetryConfig.MaxTries {
			diagMessage := fmt.Sprintf("Error reading SAMLConnector (tried %d times) - state outdated, please import resource", tries)
			resp.Diagnostics.AddError(diagMessage, "saml")
			return
		}
	}

	samlConnectorResource, ok := samlConnectorI.(*apitypes.SAMLConnectorV2)
	if !ok {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Errorf("Can not convert %T to SAMLConnectorV2", samlConnectorI), "saml"))
		return
	}
	diags = tfschema.CopySAMLConnectorV2ToTerraform(ctx, samlConnector, &plan)
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

// Delete deletes Teleport SAMLConnector
func (r resourceTeleportSAMLConnector) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var id types.String
	diags := req.State.GetAttribute(ctx, path.Root("metadata").AtName("name"), &id)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.p.Client.DeleteSAMLConnector(ctx, id.Value)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error deleting SAMLConnectorV2", trace.Wrap(err), "saml"))
		return
	}

	resp.State.RemoveResource(ctx)
}

// ImportState imports SAMLConnector state
func (r resourceTeleportSAMLConnector) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	samlConnector, err := r.p.Client.GetSAMLConnector(ctx, req.ID, true)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading SAMLConnector", trace.Wrap(err), "saml"))
		return
	}

	
	samlConnectorResource := samlConnector.(*apitypes.SAMLConnectorV2)

	var state types.Object

	diags := resp.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = tfschema.CopySAMLConnectorV2ToTerraform(ctx, samlConnectorResource, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := samlConnectorResource.GetName()

	state.Attrs["id"] = types.String{Value: id}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
