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
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func GenSchemaProvisionTokenV2() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		// Kind is a resource kind
		"kind": {
			Type:        schema.TypeString,
			Description: "Kind is a resource kind",
			Optional:    true,
			Default:     "token",
		},
		// SubKind is an optional resource sub kind, used in some resources
		"sub_kind": {
			Type:        schema.TypeString,
			Description: "SubKind is an optional resource sub kind, used in some resources",
			Optional:    true,
			Default:     "",
		},
		// Version is version
		"version": {
			Type:        schema.TypeString,
			Description: "Version is version",
			Optional:    true,
			Default:     "v2",
		},
		"index": {
			Type:        schema.TypeString,
			Description: "TODO",
			Required:    true,
			ForceNew:    true,
		},
		"name": {
			Type:        schema.TypeString,
			Description: "TODO",
			Required:    true,
			ForceNew:    true,
			Sensitive:   true,
		},
		// Metadata is resource metadata
		"metadata": {
			Type:        schema.TypeList,
			MaxItems:    1,
			Description: "Metadata is resource metadata",
			Optional:    true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					// Name is an object name
					"name": {
						Type:        schema.TypeString,
						Description: "Name is an object name",
						Sensitive:   true,
						Computed:    true,
					},
					// Namespace is object namespace. The field should be called "namespace"
					// when it returns in Teleport 2.4.
					"namespace": {
						Type:        schema.TypeString,
						Description: "Namespace is object namespace. The field should be called \"namespace\"  when it returns in Teleport 2.4.",
						Optional:    true,
						Default:     "default",
					},
					// Description is object description
					"description": {
						Type:        schema.TypeString,
						Description: "Description is object description",
						Optional:    true,
					},
					// Labels is a set of labels
					"labels": {

						Optional:    true,
						Type:        schema.TypeMap,
						Description: "Labels is a set of labels",
						Elem: &schema.Schema{
							Type: schema.TypeString,
						},
					},
					// Expires is a global expiry time header can be set on any resource in the
					// system.
					"expires": {
						Type:         schema.TypeString,
						Description:  "Expires is a global expiry time header can be set on any resource in the  system.",
						ValidateFunc: validation.IsRFC3339Time,
						StateFunc:    tfschema.TruncateMs,
						Required:     true,
					},
				},
			},
		},
		// Spec is a provisioning token V2 spec
		"spec": {
			Type:        schema.TypeList,
			MaxItems:    1,
			Description: "ProvisionTokenSpecV2 is a specification for V2 token",

			Required: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					// Roles is a list of roles associated with the token,
					// that will be converted to metadata in the SSH and X509
					// certificates issued to the user of the token
					"roles": {

						Required:    true,
						Type:        schema.TypeList,
						Description: "Roles is a list of roles associated with the token,  that will be converted to metadata in the SSH and X509  certificates issued to the user of the token",
						Elem: &schema.Schema{
							Type: schema.TypeString,
						},
					},
				},
			},
		},
	}
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
		return diagFromErr(fmt.Errorf("Index is required"))
	}
	name, ok := d.GetOk("name")
	if !ok {
		return diagFromErr(fmt.Errorf("Name is required"))
	}

	token := index.(string) + name.(string)
	metadata, ok := d.GetOk("metadata")
	if !ok {
		md := []interface{}{}
		md0 := map[string]interface{}{"name": token}
		md = append(md, md0)
		d.Set("metadata", md)
	} else {
		md := metadata.([]interface{})
		md0 := md[0].(map[string]interface{})
		md0["name"] = token
		d.Set("metadata", md)
	}

	// Check if token already exists
	_, err = c.GetToken(ctx, token)
	if err == nil {
		existErr := "token " + token + " exists in Teleport. Either remove it (tctl tokens rm " + token + ")" +
			" or import it to the existing state (terraform import teleport_provision_token." + index.(string) + " " + token + ")"

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
		return diagFromErr(fmt.Errorf("API Error: %v", err))
	}

	d.SetId(index.(string))

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

	token := index + name.(string)

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

	token := index + name.(string)
	metadata, ok := d.GetOk("metadata")
	if !ok {
		md := []interface{}{}
		md0 := map[string]interface{}{"name": token}
		md = append(md, md0)
		d.Set("metadata", md)
	} else {
		md := metadata.([]interface{})
		md0 := md[0].(map[string]interface{})
		md0["name"] = token
		d.Set("metadata", md)
	}

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
		return diagFromErr(fmt.Errorf("error"))
	}
	token := index + name.(string)
	err = c.DeleteToken(ctx, token)
	if err != nil {
		return diagFromErr(describeErr(err, "token"))
	}

	return diag.Diagnostics{}
}
