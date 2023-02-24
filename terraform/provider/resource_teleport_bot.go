/*
Copyright 2022 Gravitational, Inc.

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
	"time"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types/wrappers"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
)

func GenSchemaBot(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Type:     types.StringType,
				Computed: true,
			},
			"name": {
				Type:        types.StringType,
				Required:    true,
				Description: "The name of the bot, i.e. the unprefixed User name",
				PlanModifiers: []tfsdk.AttributePlanModifier{
					tfsdk.RequiresReplace(),
				},
			},
			"user_name": {
				Type:        types.StringType,
				Computed:    true,
				Description: "The name of the generated bot user",
			},
			"role_name": {
				Type:        types.StringType,
				Computed:    true,
				Description: "The name of the generated bot role",
			},
			"token_ttl": {
				Type:        types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "The desired TTL for the token if one is created. If unset, a server default is used",
			},
			"token_id": {
				Type: types.StringType,
				// Implementation note: the Terraform provider differs from the
				// actual API here. `token_id` is technically optional and when
				// unset, a new random token is created automatically. This
				// behavior doesn't play nicely with Terraform's resource
				// management, so we'll instead make it required. Users can use
				// a `random_password` resource with `teleport_provision_token`
				// to achieve a similar result. See also:
				// `example/bot.tf.example`
				Required:    true,
				Sensitive:   true,
				Description: "The bot joining token. If unset, a new random token is created and its name returned, otherwise a preexisting Bot token may be provided for IAM/OIDC joining.",

				// TODO: Consider dropping RequiresReplace() in the future if a
				// ResetBot() API becomes available to reset a bot's generation
				// counter.
				// See also:
				// https://github.com/gravitational/teleport/issues/13091
				PlanModifiers: []tfsdk.AttributePlanModifier{
					tfsdk.RequiresReplace(),
				},
			},
			"roles": {
				Type: types.ListType{
					ElemType: types.StringType,
				},
				Required:    true,
				Description: "A list of roles the created bot should be allowed to assume via role impersonation.",

				// TODO: Consider dropping RequiresReplace() in the future if a
				// UpdateBotRoles() API becomes available that can modify the
				// underlying bot user.
				PlanModifiers: []tfsdk.AttributePlanModifier{
					tfsdk.RequiresReplace(),
				},
			},
			// Implementation note: This needs RequiresReplace() to handle
			// updates properly but we aren't able to attach plan modifiers to
			// fields from schema methods here. See ModifyPlan below.
			"traits": tfschema.GenSchemaTraits(ctx),
		},
	}, nil
}

// Bot is a deserializes representation of the terraform state for this
// resource.
type Bot struct {
	ID      types.String   `tfsdk:"id"`
	Name    types.String   `tfsdk:"name"`
	Roles   []types.String `tfsdk:"roles"`
	TokenID types.String   `tfsdk:"token_id"`
	Traits  types.Map      `tfsdk:"traits"`
	TTL     types.String   `tfsdk:"token_ttl"`

	UserName types.String `tfsdk:"user_name"`
	RoleName types.String `tfsdk:"role_name"`
}

// resourceTeleportBotType is the resource metadata type
type resourceTeleportBotType struct{}

// GetSchema returns the resource schema
func (r resourceTeleportBotType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	// It's unusual for this provider, but we'll hand-write the schema here as
	// bots do not have any server-side resources of their own.
	return GenSchemaBot(ctx)
}

// NewResource creates the empty resource
func (r resourceTeleportBotType) NewResource(_ context.Context, p tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	return resourceTeleportBot{
		p: *(p.(*Provider)),
	}, nil
}

// resourceTeleportBot is the resource
type resourceTeleportBot struct {
	p Provider
}

func (r resourceTeleportBot) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan Bot
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var roles []string
	for _, role := range plan.Roles {
		roles = append(roles, role.Value)
	}

	var traits wrappers.Traits = make(wrappers.Traits)
	tfschema.CopyFromTraits(diags, plan.Traits, &traits)

	ttl := time.Duration(0)
	if plan.TTL.Value != "" {
		var err error
		ttl, err = time.ParseDuration(plan.TTL.Value)
		if err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error parsing TTL", trace.Wrap(err), "bot"))
			return
		}
	}

	response, err := r.p.Client.CreateBot(ctx, &proto.CreateBotRequest{
		Name:    plan.Name.Value,
		TTL:     proto.Duration(ttl),
		Roles:   roles,
		TokenID: plan.TokenID.Value,
		Traits:  traits,
	})
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error creating Bot", trace.Wrap(err), "bot"))
		return
	}

	plan.TTL = types.String{Value: response.TokenTTL.Get().String()}
	plan.UserName = types.String{Value: response.UserName}
	plan.RoleName = types.String{Value: response.RoleName}

	// ID is for terraform-plugin-framework's acctests
	plan.ID = types.String{Value: plan.Name.Value}

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r resourceTeleportBot) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	// Not much to do here: bots are currently immutable. We'll just check for
	// deletion.

	var plan Bot
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.p.Client.GetUser(plan.UserName.Value, false)
	if trace.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading Bot", trace.Wrap(err), "bot"))
		return
	}
}

func (r resourceTeleportBot) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	// Nothing to do here: bots are currently immutable. In the future we'd
	// ideally want to add specific RPCs for desired mutable attributes, e.g.
	// UpdateBotRoles(), UpdateBotToken(), etc.
}

func (r resourceTeleportBot) ModifyPlan(ctx context.Context, req tfsdk.ModifyResourcePlanRequest, resp *tfsdk.ModifyResourcePlanResponse) {
	// Add .traits to RequiresReplace to ensure changes to this field trigger a
	// replacement. We can't set it in the schema as the attribute is generated
	// by a helper method.
	resp.RequiresReplace = append(resp.RequiresReplace, path.Root("traits"))
}

func (r resourceTeleportBot) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var name types.String
	diags := req.State.GetAttribute(ctx, path.Root("name"), &name)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.p.Client.DeleteBot(ctx, name.Value)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error deleting Bot", trace.Wrap(err), "bot"))
		return
	}

	resp.State.RemoveResource(ctx)
}
