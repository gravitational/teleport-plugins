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

	"github.com/gravitational/teleport/api/types"
)

// resourceTeleportUser returns Teleport user resource definition
func resourceTeleportUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceUserCreate,
		ReadContext:   resourceUserRead,
		UpdateContext: resourceUserUpdate,
		DeleteContext: resourceUserDelete,

		Schema:        tfschema.SchemaUserV2,
		SchemaVersion: 2,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceUserCreate creates Teleport user from resource definition
func resourceUserCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	name, err := getResourceName(d, "user")
	if err != nil {
		return diagFromErr(err)
	}

	// Check if user already exists
	_, err = c.GetUser(name, false)
	if err == nil {
		existErr := "user " + name + " exists in Teleport. Either remove it (tctl users rm " + name + ")" +
			" or import it to the existing state (terraform import teleport_user." + name + " " + name + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "token"))
	}

	u, err := types.NewUser(name)
	if err != nil {
		return diagFromErr(err)
	}

	u2, ok := u.(*types.UserV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to *types.UserV2 from %T", u))
	}

	err = tfschema.FromTerraformUserV2(d, u2)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.CreateUser(ctx, u2)
	if err != nil {
		return diagFromErr(describeErr(err, "user"))
	}

	d.SetId(u2.GetName())

	return resourceUserRead(ctx, d, m)
}

// resourceUserRead reads Teleport user. This method is required by Terraform to ensure that CRUD
// operation was successful.
func resourceUserRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	// Graceful destroy
	u, err := c.GetUser(id, false)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, "user"))
	}

	u2, ok := u.(*types.UserV2)
	if !ok {
		return diagFromErr(trace.Errorf("can not convert %T to *types.TrustedClusterV2", u))
	}

	err = tfschema.ToTerraformUserV2(u2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceUserUpdate updates Teleport user from resource definition
func resourceUserUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	// Check that user exists. This situation is hardly possible because Terraform tries to read a resource before updating it.
	_, err = c.GetUser(id, false)
	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	u, err := types.NewUser(id)
	if err != nil {
		return diagFromErr(err)
	}

	u2, ok := u.(*types.UserV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.UserV2 from %T", u))
	}

	err = tfschema.FromTerraformUserV2(d, u2)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpdateUser(ctx, u2)
	if err != nil {
		return diagFromErr(describeErr(err, "user"))
	}

	return resourceUserRead(ctx, d, m)
}

// resourceUserDelete deletes Teleport user from resource definition
func resourceUserDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteUser(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "user"))
	}

	return diag.Diagnostics{}
}
