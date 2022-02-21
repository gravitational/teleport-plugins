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

// resourceTeleportClusterNetworkingConfig returns Teleport cluster networking resource definition
func resourceTeleportClusterNetworkingConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNetworkingConfigCreate,
		ReadContext:   resourceNetworkingConfigRead,
		UpdateContext: resourceNetworkingConfigUpdate,
		DeleteContext: resourceNetworkingConfigDelete,

		Schema:        tfschema.SchemaClusterNetworkingConfigV2,
		SchemaVersion: 3,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceNetworkingConfigCreate creates Teleport cluster networking config from resource definition
func resourceNetworkingConfigCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n := types.ClusterNetworkingConfigV2{}

	err = tfschema.FromTerraformClusterNetworkingConfigV2(d, &n)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.SetClusterNetworkingConfig(ctx, &n)
	if err != nil {
		return diagFromErr(describeErr(err, types.KindClusterNetworkingConfig))
	}

	d.SetId(types.KindClusterNetworkingConfig)

	return resourceNetworkingConfigRead(ctx, d, m)
}

// resourceNetworkingConfigRead reads Teleport cluster networking config
func resourceNetworkingConfigRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	raw, err := c.GetClusterNetworkingConfig(ctx)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, types.KindClusterNetworkingConfig))
	}

	n, ok := raw.(*types.ClusterNetworkingConfigV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created %v to types.ClusterNetworkingConfigV2 from %T", types.KindClusterNetworkingConfig, n))
	}

	removeOriginLabel(n.Metadata.Labels)

	err = tfschema.ToTerraformClusterNetworkingConfigV2(n, d)
	if err != nil {
		return diagFromErr(err)
	}

	d.SetId(types.KindClusterNetworkingConfig)

	return diag.Diagnostics{}
}

// resourceNetworkingConfigUpdate updates Teleport cluster networking config from resource definition
func resourceNetworkingConfigUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	// It always exists
	raw, err := c.GetClusterNetworkingConfig(ctx)
	if err != nil {
		return diagFromErr(err)
	}

	n, ok := raw.(*types.ClusterNetworkingConfigV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created role to types.ClusterNetworkingConfigV2 from %T", n))
	}

	err = tfschema.FromTerraformClusterNetworkingConfigV2(d, n)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.SetClusterNetworkingConfig(ctx, n)
	if err != nil {
		return diagFromErr(describeErr(err, types.KindClusterNetworkingConfig))
	}

	return resourceNetworkingConfigRead(ctx, d, m)
}

// resourceNetworkingConfigDelete resets cluster networking
func resourceNetworkingConfigDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.ResetClusterNetworkingConfig(ctx)
	if err != nil {
		return diagFromErr(describeErr(err, types.KindClusterNetworkingConfig))
	}

	return diag.Diagnostics{}
}

func removeOriginLabel(l map[string]string) {
	if l["teleport.dev/origin"] == "dynamic" {
		delete(l, "teleport.dev/origin")
	}
}
