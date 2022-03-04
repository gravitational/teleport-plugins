/*
Copyright 2015-2021 Gravitational, Inc.

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

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

// resourceTeleportRole returns Teleport role resource definition
func resourceTeleportRole() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceRoleCreate,
		ReadContext:   resourceRoleRead,
		UpdateContext: resourceRoleUpdate,
		DeleteContext: resourceRoleDelete,

		Schema:        tfschema.SchemaRoleV5,
		SchemaVersion: 4,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceRoleCreate creates Teleport role from resource definition
func resourceRoleCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "role")
	if err != nil {
		return diagFromErr(err)
	}

	// Check if role already exists
	_, err = c.GetRole(ctx, n)
	if err == nil {
		existErr := "role " + n + " exists in Teleport. Either remove it (tctl rm role/" + n + ")" +
			" or import it to the existing state (terraform import teleport_role." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "role"))
	}

	r := types.RoleV5{}

	err = tfschema.FromTerraformRoleV5(d, &r)
	if err != nil {
		return diagFromErr(err)
	}

	err = r.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertRole(ctx, &r)
	if err != nil {
		return diagFromErr(err)
	}

	d.SetId(r.GetName())

	return resourceRoleRead(ctx, d, m)
}

// resourceRoleRead reads Teleport role. This method is required by Terraform to ensure that CRUD
// operation was successful.
func resourceRoleRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	r, err := getRole(ctx, d, c)
	if err != nil {
		return diagFromErr(err)
	}
	if r == nil {
		return diag.Diagnostics{}
	}

	err = tfschema.ToTerraformRoleV5(r, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceRoleUpdate updates Teleport role from resource definition
func resourceRoleUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	r, err := getRole(ctx, d, c)
	if err != nil {
		return diagFromErr(describeErr(err, "role"))
	}
	if r == nil {
		return diag.Diagnostics{}
	}

	err = tfschema.FromTerraformRoleV5(d, r)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertRole(ctx, r)
	if err != nil {
		return diagFromErr(err)
	}

	return resourceRoleRead(ctx, d, m)
}

// resourceRoleDelete deletes Teleport role from resource definition
func resourceRoleDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteRole(ctx, id)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// getRole gets role with graceful destroy handling
func getRole(ctx context.Context, d *schema.ResourceData, c *client.Client) (*types.RoleV5, error) {
	id := d.Id()

	r, err := c.GetRole(ctx, id)
	if trace.IsNotFound(err) {
		d.SetId("")
		return nil, nil
	}

	if err != nil {
		return nil, trace.Wrap(err)
	}

	r3, ok := r.(*types.RoleV5)
	if !ok {
		return nil, fmt.Errorf("failed to convert created user to types.RoleV5 from %T", r)
	}

	return r3, nil
}
