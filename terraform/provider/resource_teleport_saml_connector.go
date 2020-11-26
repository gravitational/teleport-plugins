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

// resourceTeleportSAMLConnector returns Teleport saml_connector resource definition
func resourceTeleportSAMLConnector() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceSAMLConnectorCreate,
		ReadContext:   resourceSAMLConnectorRead,
		UpdateContext: resourceSAMLConnectorUpdate,
		DeleteContext: resourceSAMLConnectorDelete,

		Schema:        tfschema.SchemaSAMLConnectorV2,
		SchemaVersion: 2,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceSAMLConnectorCreate creates Teleport SAMLConnector from resource definition
func resourceSAMLConnectorCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "saml")
	if err != nil {
		return diagFromErr(err)
	}

	_, err = c.GetSAMLConnector(ctx, n, false)
	if err == nil {
		existErr := "SAML connector " + n + " exists in Teleport. Either remove it (tctl rm saml/" + n + ")" +
			" or import it to the existing state (terraform import teleport_saml_connector." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "saml"))
	}

	s := types.NewSAMLConnector(n, types.SAMLConnectorSpecV2{})

	s2, ok := s.(*types.SAMLConnectorV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created saml to types.SAMLConnectorV2 from %T", s))
	}

	err = tfschema.GetSAMLConnectorV2(s2, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertSAMLConnector(ctx, s2)
	if err != nil {
		return diagFromErr(describeErr(err, "saml"))
	}

	d.SetId(s2.GetName())

	return resourceSAMLConnectorRead(ctx, d, m)
}

// resourceSAMLConnectorRead reads Teleport SAML connector.
func resourceSAMLConnectorRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	s, err := c.GetSAMLConnector(ctx, id, true)
	if trace.IsNotFound(err) {
		d.SetId("")
		return diag.Diagnostics{}
	}

	if err != nil {
		return diagFromErr(describeErr(err, "saml"))
	}

	s2, ok := s.(*types.SAMLConnectorV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created user to types.SAMLConnectorV2 from %T", s))
	}

	err = tfschema.SetSAMLConnectorV2(s2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceSAMLConnectorUpdate updates Teleport SAML connector from resource definition
func resourceSAMLConnectorUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	s, err := c.GetSAMLConnector(ctx, id, false)
	if err != nil {
		return diagFromErr(err)
	}

	s2, ok := s.(*types.SAMLConnectorV2)
	if !ok {
		return diagFromErr(fmt.Errorf("failed to convert created saml to types.SAMLConnectorV2 from %T", s))
	}

	err = tfschema.GetSAMLConnectorV2(s2, d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertSAMLConnector(ctx, s2)
	if err != nil {
		return diagFromErr(describeErr(err, "saml"))
	}

	return resourceSAMLConnectorRead(ctx, d, m)
}

// resourceSAMLConnectorDelete deletes Teleport SAML connector from resource definition
func resourceSAMLConnectorDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteSAMLConnector(ctx, id)
	if err != nil {
		return diagFromErr(describeErr(err, "saml"))
	}

	return diag.Diagnostics{}
}
