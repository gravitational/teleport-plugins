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

// resourceTeleportSessionRecordingConfig returns Teleport session recording config resource definition
func resourceTeleportSessionRecordingConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceRecordingConfigCreate,
		ReadContext:   resourceRecordingConfigRead,
		UpdateContext: resourceRecordingConfigUpdate,
		DeleteContext: resourceRecordingConfigDelete,

		Schema:        tfschema.SchemaSessionRecordingConfigV2,
		SchemaVersion: 3,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceRecordingConfigCreate creates Teleport session recording config from resource definition
func resourceRecordingConfigCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n := types.SessionRecordingConfigV2{}

	err = tfschema.GetSessionRecordingConfigV2(&n, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.SetSessionRecordingConfig(ctx, &n)
	if err != nil {
		return diagFromErr(describeErr(err, types.KindSessionRecordingConfig))
	}

	d.SetId(types.KindSessionRecordingConfig)

	return resourceRecordingConfigRead(ctx, d, m)
}

// resourceRecordingConfigRead reads Teleport session recording config
func resourceRecordingConfigRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	raw, err := c.GetSessionRecordingConfig(ctx)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, types.KindSessionRecordingConfig))
	}

	n, ok := raw.(*types.SessionRecordingConfigV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created %v to types.SessionRecordingConfigV2 from %T", types.KindSessionRecordingConfig, n))
	}

	removeOriginLabel(n.Metadata.Labels)

	err = tfschema.SetSessionRecordingConfigV2(n, d)
	if err != nil {
		return diagFromErr(err)
	}

	d.SetId(types.KindSessionRecordingConfig)

	return diag.Diagnostics{}
}

// resourceRecordingConfigUpdate updates Teleport session recording config from resource definition
func resourceRecordingConfigUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	// It always exists
	raw, err := c.GetSessionRecordingConfig(ctx)
	if err != nil {
		return diagFromErr(err)
	}

	n, ok := raw.(*types.SessionRecordingConfigV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created role to types.SessionRecordingConfigV2 from %T", n))
	}

	err = tfschema.GetSessionRecordingConfigV2(n, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.SetSessionRecordingConfig(ctx, n)
	if err != nil {
		return diagFromErr(describeErr(err, types.KindSessionRecordingConfig))
	}

	return resourceRecordingConfigRead(ctx, d, m)
}

// resourceRecordingConfigDelete resets session recording config
func resourceRecordingConfigDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.ResetSessionRecordingConfig(ctx)
	if err != nil {
		return diagFromErr(describeErr(err, types.KindSessionRecordingConfig))
	}

	return diag.Diagnostics{}
}
