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

// resourceTeleportTrustedCluster returns Teleport trusted_cluster resource definition
func resourceTeleportTrustedCluster() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceTrustedClusterCreate,
		ReadContext:   resourceTrustedClusterRead,
		UpdateContext: resourceTrustedClusterUpdate,
		DeleteContext: resourceTrustedClusterDelete,

		Schema:        tfschema.SchemaTrustedClusterV2,
		SchemaVersion: 2,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceTrustedClusterCreate creates Teleport TrustedCluster from resource definition
func resourceTrustedClusterCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "trusted_cluster")
	if err != nil {
		return diagFromErr(err)
	}

	// Check if trusted cluster already exists
	_, err = c.GetTrustedCluster(ctx, n)
	if err == nil {
		existErr := "trusted cluster " + n + " exists in Teleport. Either remove it (tctl rm trusted_cluster/" + n + ")" +
			" or import it to the existing state (terraform import teleport_trusted_cluster." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "trusted_cluster"))
	}

	t := types.TrustedClusterV2{}

	err = tfschema.GetTrustedClusterV2(&t, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = t.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(err)
	}

	_, err = c.UpsertTrustedCluster(ctx, &t)
	if err != nil {
		return diagFromErr(describeErr(err, "trusted_cluster"))
	}

	d.SetId(t.GetName())

	return resourceTrustedClusterRead(ctx, d, m)
}

// resourceTrustedClusterRead reads Teleport trusted cluster. This method is required by Terraform to ensure that CRUD
// operation was successful.
func resourceTrustedClusterRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	t, err := c.GetTrustedCluster(ctx, id)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(err)
	}

	t2, ok := t.(*types.TrustedClusterV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.TrustedClusterV2 from %T", t))
	}

	err = tfschema.SetTrustedClusterV2(t2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceTrustedClusterUpdate updates Teleport trusted cluster from resource definition
func resourceTrustedClusterUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	t, err := c.GetTrustedCluster(ctx, id)
	if err != nil {
		return diagFromErr(err)
	}

	t2, ok := t.(*types.TrustedClusterV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created trusted cluster to types.TrustedClusterV2 from %T", t))
	}

	err = tfschema.GetTrustedClusterV2(t2, d)
	if err != nil {
		return diagFromErr(err)
	}

	_, err = c.UpsertTrustedCluster(ctx, t2)
	if err != nil {
		return diagFromErr(err)
	}

	return resourceTrustedClusterRead(ctx, d, m)
}

// resourceTrustedClusterDelete deletes Teleport trusted cluster from resource definition
func resourceTrustedClusterDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteTrustedCluster(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "trusted_cluster"))
	}

	return diag.Diagnostics{}
}
