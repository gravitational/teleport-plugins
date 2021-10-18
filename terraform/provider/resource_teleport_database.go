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

// resourceTeleportDatabase returns Teleport database resource definition
func resourceTeleportDatabase() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceDatabaseCreate,
		ReadContext:   resourceDatabaseRead,
		UpdateContext: resourceDatabaseUpdate,
		DeleteContext: resourceDatabaseDelete,

		Schema:        tfschema.SchemaDatabaseV3,
		SchemaVersion: 2,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceDatabaseCreate creates Teleport Database from resource definition
func resourceDatabaseCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "db")
	if err != nil {
		return diagFromErr(err)
	}

	_, err = c.GetDatabase(ctx, n)
	if err == nil {
		existErr := "Database " + n + " exists in Teleport. Either remove it (tctl rm db/" + n + ")" +
			" or import it to the existing state (terraform import teleport_database." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "db"))
	}

	s := types.DatabaseV3{Spec: types.DatabaseSpecV3{}}
	err = tfschema.FromTerraformDatabaseV3(d, &s)
	if err != nil {
		return diagFromErr(err)
	}

	err = s.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(describeErr(err, "db"))
	}

	err = c.CreateDatabase(ctx, &s)
	if err != nil {
		return diagFromErr(describeErr(err, "db"))
	}

	d.SetId(s.GetName())

	return resourceDatabaseRead(ctx, d, m)
}

// resourceDatabaseRead reads Teleport Database.
func resourceDatabaseRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	s, err := c.GetDatabase(ctx, id)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, "db"))
	}

	s2, ok := s.(*types.DatabaseV3)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.DatabaseV3 from %T", s))
	}

	err = tfschema.ToTerraformDatabaseV3(s2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceDatabaseUpdate updates Teleport Database from resource definition
func resourceDatabaseUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	s, err := c.GetDatabase(ctx, id)
	if err != nil {
		return diagFromErr(err)
	}

	s2, ok := s.(*types.DatabaseV3)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created database to types.DatabaseV3 from %T", s))
	}

	err = tfschema.FromTerraformDatabaseV3(d, s2)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpdateDatabase(ctx, s2)
	if err != nil {
		return diagFromErr(describeErr(err, "db"))
	}

	return resourceDatabaseRead(ctx, d, m)
}

// resourceDatabaseDelete deletes Teleport Database from resource definition
func resourceDatabaseDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteDatabase(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "db"))
	}

	return diag.Diagnostics{}
}
