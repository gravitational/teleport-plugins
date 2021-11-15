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

// Set is not implemented in the API: https://github.com/gravitational/teleport/blob/1944e62cc55d74d26b337945000b192528674ef3/api/client/client.go#L1776
//
// https://github.com/gravitational/teleport/pull/7465

// resourceTeleportClusterAuditConfig returns Teleport cluster audit resource definition
func resourceTeleportClusterAuditConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceAuditConfigCreate,
		ReadContext:   resourceAuditConfigRead,
		UpdateContext: resourceAuditConfigUpdate,
		DeleteContext: resourceAuditConfigDelete,

		Schema:        tfschema.SchemaClusterAuditConfigV2,
		SchemaVersion: 3,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceAuditConfigCreate creates Teleport cluster audit config from resource definition
func resourceAuditConfigCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n := types.ClusterAuditConfigV2{}

	err = tfschema.FromTerraformClusterAuditConfigV2(d, &n)
	if err != nil {
		return diagFromErr(err)
	}

	// Linter generates false positive here because the API always returns error (see comment above)
	err = c.SetClusterAuditConfig(ctx, &n) //nolint
	if err != nil {                        //nolint
		return diagFromErr(describeErr(err, types.KindClusterAuditConfig))
	}

	d.SetId(types.KindClusterAuditConfig)

	return resourceAuditConfigRead(ctx, d, m)
}

// resourceAuditConfigRead reads Teleport cluster audit config
func resourceAuditConfigRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	raw, err := c.GetClusterAuditConfig(ctx)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, types.KindClusterAuditConfig))
	}

	n, ok := raw.(*types.ClusterAuditConfigV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created %v to types.ClusterAuditConfigV2 from %T", types.KindClusterAuditConfig, n))
	}

	removeOriginLabel(n.Metadata.Labels)

	err = tfschema.ToTerraformClusterAuditConfigV2(n, d)
	if err != nil {
		return diagFromErr(err)
	}

	d.SetId(types.KindClusterAuditConfig)

	return diag.Diagnostics{}
}

// resourceAuditConfigUpdate updates Teleport cluster audit config from resource definition
func resourceAuditConfigUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	// It always exists
	raw, err := c.GetClusterAuditConfig(ctx)
	if err != nil {
		return diagFromErr(err)
	}

	n, ok := raw.(*types.ClusterAuditConfigV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created role to types.ClusterAuditConfigV2 from %T", n))
	}

	err = tfschema.FromTerraformClusterAuditConfigV2(d, n)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.SetClusterAuditConfig(ctx, n) //nolint
	if err != nil {                       //nolint
		return diagFromErr(describeErr(err, types.KindClusterAuditConfig))
	}

	return resourceAuditConfigRead(ctx, d, m)
}

// resourceAuditConfigDelete resets cluster audit
func resourceAuditConfigDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.DeleteClusterAuditConfig(ctx) //nolint
	if err != nil {                       //nolint
		return diagFromErr(describeErr(err, types.KindClusterAuditConfig))
	}

	return diag.Diagnostics{}
}
