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

// resourceTeleportAuthPreference returns Teleport auth_preference resource definition
func resourceTeleportAuthPreference() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceAuthPreferenceCreate,
		ReadContext:   resourceAuthPreferenceRead,
		UpdateContext: resourceAuthPreferenceUpdate,
		DeleteContext: resourceAuthPreferenceDelete,

		Schema:        tfschema.SchemaAuthPreferenceV2,
		SchemaVersion: 3,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceAuthPreferenceCreate creates Teleport AuthPreference from resource definition
func resourceAuthPreferenceCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	cn, err := types.NewAuthPreference(types.AuthPreferenceSpecV2{})
	if err != nil {
		return diagFromErr(describeErr(err, "cluster_auth_preference"))
	}

	cn3, ok := cn.(*types.AuthPreferenceV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created role to types.AuthPreferenceV3 from %T", cn))
	}

	err = tfschema.GetAuthPreferenceV2(cn3, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.SetAuthPreference(ctx, cn3)
	if err != nil {
		return diagFromErr(describeErr(err, "cluster_auth_preference"))
	}

	d.SetId(cn3.GetName())

	return resourceAuthPreferenceRead(ctx, d, m)
}

// resourceAuthPreferenceRead reads Teleport role. This method is required by Terraform to ensure that CRUD
// operation was successful.
func resourceAuthPreferenceRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	cn, err := c.GetAuthPreference(ctx)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, "cluster_auth_preference"))
	}

	cn3, ok := cn.(*types.AuthPreferenceV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created cluster_auth_preference to types.AuthPreferenceV3 from %T", cn))
	}

	err = tfschema.SetAuthPreferenceV2(cn3, d)
	if err != nil {
		return diagFromErr(err)
	}

	d.SetId("cluster_auth_preference")

	return diag.Diagnostics{}
}

// resourceAuthPreferenceUpdate updates Teleport role from resource definition
func resourceAuthPreferenceUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	// It always exists
	cn, err := c.GetAuthPreference(ctx)
	if err != nil {
		return diagFromErr(err)
	}

	cn3, ok := cn.(*types.AuthPreferenceV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created role to types.AuthPreferenceV3 from %T", cn))
	}

	err = tfschema.GetAuthPreferenceV2(cn3, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.SetAuthPreference(ctx, cn3)
	if err != nil {
		return diagFromErr(describeErr(err, "cluster_auth_preference"))
	}

	return resourceAuthPreferenceRead(ctx, d, m)
}

// resourceAuthPreferenceDelete resets cluster_auth_preference resource
func resourceAuthPreferenceDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.ResetAuthPreference(ctx)
	if err != nil {
		return diagFromErr(describeErr(err, "cluster_auth_preference"))
	}

	return diag.Diagnostics{}
}
