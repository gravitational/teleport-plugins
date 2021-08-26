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

// resourceTeleportGithubConnector returns Teleport github_connector resource definition
func resourceTeleportGithubConnector() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceGithubConnectorCreate,
		ReadContext:   resourceGithubConnectorRead,
		UpdateContext: resourceGithubConnectorUpdate,
		DeleteContext: resourceGithubConnectorDelete,

		Schema:        tfschema.SchemaGithubConnectorV3,
		SchemaVersion: 3,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceGithubConnectorCreate creates Teleport GithubConnector from resource definition
func resourceGithubConnectorCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "github")
	if err != nil {
		return diagFromErr(err)
	}

	_, err = c.GetGithubConnector(ctx, n, true)
	if err == nil {
		existErr := "GitHub connector " + n + " exists in Teleport. Either remove it (tctl rm github/" + n + ")" +
			" or import it to the existing state (terraform import teleport_github_connector." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "github"))
	}

	cn := types.GithubConnectorV3{}

	err = tfschema.GetGithubConnectorV3(&cn, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = cn.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertGithubConnector(ctx, &cn)
	if err != nil {
		return diagFromErr(describeErr(err, "github"))
	}

	d.SetId(cn.GetName())

	return resourceGithubConnectorRead(ctx, d, m)
}

// resourceGithubConnectorRead reads Teleport role. This method is required by Terraform to ensure that CRUD
// operation was successful.
func resourceGithubConnectorRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	cn, err := c.GetGithubConnector(ctx, id, true)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}
	if err != nil {
		return diagFromErr(describeErr(err, "github"))
	}

	cn3, ok := cn.(*types.GithubConnectorV3)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.GithubConnectorV3 from %T", cn))
	}

	err = tfschema.SetGithubConnectorV3(cn3, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceGithubConnectorUpdate updates Teleport role from resource definition
func resourceGithubConnectorUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	// This is the existence check. Since, there is no separate CreateGithubConnector method, we need to
	// check that connector we are updating exists to avoid it's creation via UpsertGithubConnector.
	cn, err := c.GetGithubConnector(ctx, id, true)
	if err != nil {
		return diagFromErr(err)
	}

	cn3, ok := cn.(*types.GithubConnectorV3)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created role to types.GithubConnectorV3 from %T", cn))
	}

	err = tfschema.GetGithubConnectorV3(cn3, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertGithubConnector(ctx, cn3)
	if err != nil {
		return diagFromErr(describeErr(err, "github"))
	}

	return resourceGithubConnectorRead(ctx, d, m)
}

// resourceGithubConnectorDelete deletes Teleport github connector from resource definition
func resourceGithubConnectorDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteGithubConnector(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "github"))
	}

	return diag.Diagnostics{}
}
