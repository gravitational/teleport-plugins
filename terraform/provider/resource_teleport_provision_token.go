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

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/gravitational/teleport-plugins/terraform/tfschema"
)

func GenSchemaProvisionTokenV2() map[string]*schema.Schema {
	modifiedSchema := tfschema.GenSchemaProvisionTokenV2()
	modifiedSchema["index"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "Non-sensitive Token prefix",
		Required:    true,
		ForceNew:    true,
	}
	modifiedSchema["name"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "Sensitive Token suffix",
		Required:    true,
		ForceNew:    true,
		Sensitive:   true,
	}
	metadataElem := modifiedSchema["metadata"].Elem.(*schema.Resource)
	metadataElem.Schema["name"] = &schema.Schema{
		Type:        schema.TypeString,
		Description: "Computed from prefix & suffix",
		Sensitive:   true,
		Computed:    true,
	}
	modifiedSchema["metadata"].Elem = metadataElem
	return modifiedSchema
}

// resourceTeleportProvisionToken returns Teleport github_connector resource definition
func resourceTeleportProvisionToken() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceProvisionTokenCreate,
		ReadContext:   resourceProvisionTokenRead,
		UpdateContext: resourceProvisionTokenUpdate,
		DeleteContext: resourceProvisionTokenDelete,

		Schema:        GenSchemaProvisionTokenV2(),
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

	index, ok := d.GetOk("index")
	if !ok {
		return diagFromErr(trace.BadParameter("Index is required"))
	}
	name, ok := d.GetOk("name")
	if !ok {
		return diagFromErr(trace.BadParameter("Name is required"))
	}

	i, ok := index.(string)
	if !ok {
		return diagFromErr(trace.BadParameter("Failed to convert %T to string", index))
	}

	n, ok := name.(string)
	if !ok {
		return diagFromErr(trace.BadParameter("Failed to convert %T to string", name))
	}

	token := i + n
	metadata, ok := d.GetOk("metadata")
	var newMetadata []interface{}
	if !ok {
		newMetadata = []interface{}{map[string]interface{}{"name": token}}
	} else {
		newMetadata, ok = metadata.([]interface{})
		if !ok {
			return diagFromErr(trace.BadParameter("Can not convert %T to []interface{}", metadata))
		}
		md, ok := newMetadata[0].(map[string]interface{})
		if !ok {
			return diagFromErr(trace.BadParameter("Can not convert %T to map[string]interface{}", newMetadata))
		}

		md["name"] = token
	}

	d.Set("metadata", newMetadata)

	// Check if token already exists
	_, err = c.GetToken(ctx, token)
	if err == nil {
		existErr := "token " + token + " exists in Teleport. Either remove it (tctl tokens rm " + token + ")" +
			" or import it to the existing state (terraform import teleport_provision_token." + i + " " + token + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "token"))
	}

	// Read token from ResourceData
	tmp := types.ProvisionTokenV2{}
	err = tfschema.GetProvisionTokenV2(&tmp, d)
	if err != nil {
		return diagFromErr(err)
	}

	// Create and validate token
	t, err := types.NewProvisionToken(token, tmp.Spec.Roles, tmp.Metadata.Expiry())
	if err != nil {
		return diagFromErr(err)
	}

	// Fill in token info
	tV2, ok := t.(*types.ProvisionTokenV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.ProvisionTokenV2 from %T", t))
	}

	err = tfschema.GetProvisionTokenV2(tV2, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertToken(ctx, tV2)

	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	d.SetId(i)

	return resourceProvisionTokenRead(ctx, d, m)
}

// resourceProvisionTokenRead reads Teleport token
func resourceProvisionTokenRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	index := d.Id()
	name, ok := d.GetOk("name")
	if !ok {
		return diagFromErr(trace.BadParameter("Name is required"))
	}

	n, ok := name.(string)
	if !ok {
		return diagFromErr(trace.BadParameter("Failed to convert %T to string", name))
	}

	token := index + n
	t, err := c.GetToken(ctx, token)
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

	err = tfschema.SetProvisionTokenV2(tV2, d)
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

	index := d.Id()
	name, ok := d.GetOk("name")
	if !ok {
		return diagFromErr(trace.BadParameter("Name is required"))
	}

	n, ok := name.(string)
	if !ok {
		return diagFromErr(trace.BadParameter("Failed to convert %T to string", name))
	}

	token := index + n
	metadata, ok := d.GetOk("metadata")
	var newMetadata []interface{}
	if !ok {
		newMetadata = []interface{}{map[string]interface{}{"name": token}}
	} else {
		newMetadata, ok = metadata.([]interface{})
		if !ok {
			return diagFromErr(trace.BadParameter("Can not convert %T to []interface{}", metadata))
		}
		md, ok := newMetadata[0].(map[string]interface{})
		if !ok {
			return diagFromErr(trace.BadParameter("Can not convert %T to map[string]interface{}", newMetadata))
		}

		md["name"] = token
	}

	d.Set("metadata", newMetadata)

	// Check that token exists. This situation is hardly possible because Terraform tries to read a resource before updating it.
	t, err := c.GetToken(ctx, token)
	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	tV2, ok := t.(*types.ProvisionTokenV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created token to types.GithubConnectorV3 from %T", t))
	}

	err = tfschema.GetProvisionTokenV2(tV2, d)
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

	index := d.Id()
	name, ok := d.GetOk("name")
	if !ok {
		return diagFromErr(trace.BadParameter("Name is required"))
	}

	n, ok := name.(string)
	if !ok {
		return diagFromErr(trace.BadParameter("Failed to convert %T to string", name))
	}

	token := index + n
	err = c.DeleteToken(ctx, token)
	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	return diag.Diagnostics{}
}
