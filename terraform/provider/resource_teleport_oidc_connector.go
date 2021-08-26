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

// resourceTeleportOIDCConnector returns Teleport oidc_connector resource definition
func resourceTeleportOIDCConnector() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceOIDCConnectorCreate,
		ReadContext:   resourceOIDCConnectorRead,
		UpdateContext: resourceOIDCConnectorUpdate,
		DeleteContext: resourceOIDCConnectorDelete,

		Schema:        tfschema.SchemaOIDCConnectorV2,
		SchemaVersion: 2,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceOIDCConnectorCreate creates Teleport OIDCConnector from resource definition
func resourceOIDCConnectorCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "oidc")
	if err != nil {
		return diagFromErr(err)
	}

	_, err = c.GetOIDCConnector(ctx, n, false)
	if err == nil {
		existErr := "OIDC connector " + n + " exists in Teleport. Either remove it (tctl rm oidc/" + n + ")" +
			" or import it to the existing state (terraform import teleport_oidc_connector." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "oidc"))
	}

	cn := types.OIDCConnectorV2{}

	err = tfschema.GetOIDCConnectorV2(&cn, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = cn.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertOIDCConnector(ctx, &cn)
	if err != nil {
		return diagFromErr(describeErr(err, "oidc"))
	}

	d.SetId(cn.GetName())

	return resourceOIDCConnectorRead(ctx, d, m)
}

// resourceOIDCConnectorRead reads Teleport OIDC connector.
func resourceOIDCConnectorRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	cn, err := c.GetOIDCConnector(ctx, id, true)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, "oidc"))
	}

	cnV2, ok := cn.(*types.OIDCConnectorV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.OIDCConnectorV2 from %T", cn))
	}

	err = tfschema.SetOIDCConnectorV2(cnV2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceOIDCConnectorUpdate updates Teleport OIDC connector from resource definition
func resourceOIDCConnectorUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	cn, err := c.GetOIDCConnector(ctx, id, false)
	if err != nil {
		return diagFromErr(err)
	}

	cnV2, ok := cn.(*types.OIDCConnectorV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created oidc connector to types.OIDCConnectorV2 from %T", cn))
	}

	err = tfschema.GetOIDCConnectorV2(cnV2, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertOIDCConnector(ctx, cnV2)
	if err != nil {
		return diagFromErr(describeErr(err, "oidc"))
	}

	return resourceOIDCConnectorRead(ctx, d, m)
}

// resourceOIDCConnectorDelete deletes Teleport OIDC connector from resource definition
func resourceOIDCConnectorDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteOIDCConnector(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "oidc"))
	}

	return diag.Diagnostics{}
}
