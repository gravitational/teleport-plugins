/*
Copyright 2024 Gravitational, Inc.

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

package tfschema

import (
	"context"
	"fmt"

	apitypes "github.com/gravitational/teleport/api/types"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	DefaultRoleKubeModifierErrSummary  = "DefaultRoleKubeResources modifier failed"
	DefaultRoleKubeModifierDescription = `This modifier re-render the role.spec.allow.kubernetes_resources from the user provided config instead of using the state.
The state contains server-generated defaults (in fact they are generated in the pre-apply plan).
However, those defaults become outdated if the version changes.
One way to deal with version change is to force-recreate, but this is too destructive.
The workaround we found was to use this plan modifier.`
)

func DefaultRoleKubeResources() tfsdk.AttributePlanModifier {
	return DefaultRoleKubeResourceModifier{}
}

type DefaultRoleKubeResourceModifier struct {
}

func (d DefaultRoleKubeResourceModifier) Description(ctx context.Context) string {
	return DefaultRoleKubeModifierDescription
}

func (d DefaultRoleKubeResourceModifier) MarkdownDescription(ctx context.Context) string {
	return DefaultRoleKubeModifierDescription
}

func (d DefaultRoleKubeResourceModifier) Modify(ctx context.Context, req tfsdk.ModifyAttributePlanRequest, resp *tfsdk.ModifyAttributePlanResponse) {
	var config tftypes.Object
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to get config.")
		return
	}

	role := &apitypes.RoleV6{}
	diags = CopyRoleV6FromTerraform(ctx, config, role)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to create a role from the config.")
		return
	}

	err := role.CheckAndSetDefaults()
	if err != nil {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, fmt.Sprintf("Failed to set the role defaults: %s", err))
		return
	}

	diags = CopyRoleV6ToTerraform(ctx, role, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to convert back the role into a TF Object.")
		return
	}

	specRaw, ok := config.Attrs["spec"]
	if !ok {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to get 'spec' from TF object.")
		return
	}
	spec, ok := specRaw.(tftypes.Object)
	if !ok {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to cast 'spec' as a TF object.")
		return
	}
	allowRaw, ok := spec.Attrs["allow"]
	if !ok {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to get 'spec' from TF object.")
		return
	}
	allow, ok := allowRaw.(tftypes.Object)
	if !ok {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to cast 'allow' as a TF object.")
		return
	}
	kubeResources, ok := allow.Attrs["kubernetes_resources"]
	if !ok {
		resp.Diagnostics.AddError(DefaultRoleKubeModifierErrSummary, "Failed to get 'kubernetes_resources' from TF object.")
		return
	}
	resp.AttributePlan = kubeResources
}
