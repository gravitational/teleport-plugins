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

// resourceTeleportApp returns Teleport app resource definition
func resourceTeleportApp() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceAppCreate,
		ReadContext:   resourceAppRead,
		UpdateContext: resourceAppUpdate,
		DeleteContext: resourceAppDelete,

		Schema:        tfschema.SchemaAppV3,
		SchemaVersion: 2,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceAppCreate creates Teleport App from resource definition
func resourceAppCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "app")
	if err != nil {
		return diagFromErr(err)
	}

	_, err = c.GetApp(ctx, n)
	if err == nil {
		existErr := "App " + n + " exists in Teleport. Either remove it (tctl rm app/" + n + ")" +
			" or import it to the existing state (terraform import teleport_app." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "app"))
	}

	s := types.AppV3{Spec: types.AppSpecV3{}}
	err = tfschema.FromTerraformAppV3(d, &s)
	if err != nil {
		return diagFromErr(err)
	}

	err = s.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(describeErr(err, "app"))
	}

	err = c.CreateApp(ctx, &s)
	if err != nil {
		return diagFromErr(describeErr(err, "app"))
	}

	d.SetId(s.GetName())

	return resourceAppRead(ctx, d, m)
}

// resourceAppRead reads Teleport App.
func resourceAppRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	s, err := c.GetApp(ctx, id)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, "app"))
	}

	s2, ok := s.(*types.AppV3)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.AppV3 from %T", s))
	}

	err = tfschema.ToTerraformAppV3(s2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceAppUpdate updates Teleport App from resource definition
func resourceAppUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	s, err := c.GetApp(ctx, id)
	if err != nil {
		return diagFromErr(err)
	}

	s2, ok := s.(*types.AppV3)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created app to types.AppV3 from %T", s))
	}

	err = tfschema.FromTerraformAppV3(d, s2)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpdateApp(ctx, s2)
	if err != nil {
		return diagFromErr(describeErr(err, "app"))
	}

	return resourceAppRead(ctx, d, m)
}

// resourceAppDelete deletes Teleport App from resource definition
func resourceAppDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteApp(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "app"))
	}

	return diag.Diagnostics{}
}
