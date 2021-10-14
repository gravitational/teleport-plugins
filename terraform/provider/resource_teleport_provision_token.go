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

// resourceTeleportProvisionToken returns Teleport github_connector resource definition
func resourceTeleportProvisionToken() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceProvisionTokenCreate,
		ReadContext:   resourceProvisionTokenRead,
		UpdateContext: resourceProvisionTokenUpdate,
		DeleteContext: resourceProvisionTokenDelete,

		Schema:        tfschema.SchemaProvisionTokenV2,
		SchemaVersion: 2,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceProvisionTokenCreate creates Teleport token from resource definition
func resourceProvisionTokenCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	name, err := getResourceName(d, "provision_token")
	if err != nil {
		return diagFromErr(err)
	}

	// Check if token already exists
	_, err = c.GetToken(ctx, name)
	if err == nil {
		existErr := "token " + name + " exists in Teleport. Either remove it (tctl tokens rm " + name + ")" +
			" or import it to the existing state (terraform import teleport_provision_token." + name + " " + name + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "token"))
	}

	t := types.ProvisionTokenV2{}
	err = tfschema.FromTerraformProvisionTokenV2(d, &t)
	if err != nil {
		return diagFromErr(err)
	}

	err = t.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertToken(ctx, &t)
	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	d.SetId(t.GetName())

	return resourceProvisionTokenRead(ctx, d, m)
}

// resourceProvisionTokenRead reads Teleport token
func resourceProvisionTokenRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	t, err := c.GetToken(ctx, id)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	tV2, ok := t.(*types.ProvisionTokenV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.ProvisionTokenV2 from %T", t))
	}

	err = tfschema.ToTerraformProvisionTokenV2(tV2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceProvisionTokenUpdate updates Teleport token from resource definition
func resourceProvisionTokenUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	// Check that token exists. This situation is hardly possible because Terraform tries to read a resource before updating it.
	t, err := c.GetToken(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	tV2, ok := t.(*types.ProvisionTokenV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created token to types.GithubConnectorV3 from %T", t))
	}

	err = tfschema.FromTerraformProvisionTokenV2(d, tV2)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertToken(ctx, tV2)
	if err != nil {
		return diagFromErr(err)
	}

	return resourceProvisionTokenRead(ctx, d, m)
}

// resourceProvisionTokenDelete deletes Teleport token from resource definition
func resourceProvisionTokenDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteToken(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	return diag.Diagnostics{}
}
