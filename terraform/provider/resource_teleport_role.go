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
	"reflect"

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

var (
	// default value for BPF field
	defaultBPF = []string{"command", "network"}
)

// resourceTeleportRole returns Teleport role resource definition
func resourceTeleportRole() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceRoleCreate,
		ReadContext:   resourceRoleRead,
		UpdateContext: resourceRoleUpdate,
		DeleteContext: resourceRoleDelete,

		Schema:        tfschema.SchemaRoleV4,
		SchemaVersion: 4,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

// resourceRoleCreate creates Teleport role from resource definition
func resourceRoleCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	n, err := getResourceName(d, "role")
	if err != nil {
		return diagFromErr(err)
	}

	// Check if role already exists
	_, err = c.GetRole(ctx, n)
	if err == nil {
		existErr := "role " + n + " exists in Teleport. Either remove it (tctl rm role/" + n + ")" +
			" or import it to the existing state (terraform import teleport_role." + n + " " + n + ")"

		return diagFromErr(trace.Errorf(existErr))
	}
	if err != nil && !trace.IsNotFound(err) {
		return diagFromErr(describeErr(err, "role"))
	}

	r := types.RoleV4{}

	err = tfschema.FromTerraformRoleV4(d, &r)
	if err != nil {
		return diagFromErr(err)
	}

	err = r.CheckAndSetDefaults()
	if err != nil {
		return diagFromErr(err)
	}

	removeMapAndListDefaults(&r)
	err = checkOptionsProvided(d)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertRole(ctx, &r)
	if err != nil {
		return diagFromErr(err)
	}

	d.SetId(r.GetName())

	return resourceRoleRead(ctx, d, m)
}

// resourceRoleRead reads Teleport role. This method is required by Terraform to ensure that CRUD
// operation was successful.
func resourceRoleRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	r, err := getRole(ctx, d, c)
	if err != nil {
		return diagFromErr(err)
	}
	if r == nil {
		return diag.Diagnostics{}
	}

	err = tfschema.ToTerraformRoleV4(r, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// resourceRoleUpdate updates Teleport role from resource definition
func resourceRoleUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	r, err := getRole(ctx, d, c)
	if err != nil {
		return diagFromErr(describeErr(err, "role"))
	}
	if r == nil {
		return diag.Diagnostics{}
	}

	removeMapAndListDefaults(r)

	err = tfschema.FromTerraformRoleV4(d, r)
	if err != nil {
		return diagFromErr(err)
	}

	err = c.UpsertRole(ctx, r)
	if err != nil {
		return diagFromErr(err)
	}

	return resourceRoleRead(ctx, d, m)
}

// resourceRoleDelete deletes Teleport role from resource definition
func resourceRoleDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()
	err = c.DeleteRole(ctx, id)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}

// getRole gets role with graceful destroy handling
func getRole(ctx context.Context, d *schema.ResourceData, c *client.Client) (*types.RoleV4, error) {
	id := d.Id()

	r, err := c.GetRole(ctx, id)
	if trace.IsNotFound(err) {
		d.SetId("")
		return nil, nil
	}

	if err != nil {
		return nil, trace.Wrap(err)
	}

	r3, ok := r.(*types.RoleV4)
	if !ok {
		return nil, fmt.Errorf("failed to convert created user to types.RoleV4 from %T", r)
	}

	removeMapAndListDefaults(r3)

	return r3, nil
}

// removeMapAndListDefaults removes invalid default items
func removeMapAndListDefaults(r *types.RoleV4) {
	removeDefaultLabel(r.Spec.Allow.AppLabels)
	removeDefaultLabel(r.Spec.Allow.DatabaseLabels)
	removeDefaultLabel(r.Spec.Allow.NodeLabels)
	removeDefaultLabel(r.Spec.Allow.KubernetesLabels)

	if reflect.DeepEqual(r.Spec.Options.BPF, defaultBPF) {
		r.Spec.Options.BPF = []string{}
	}
}

// removeDefaultLabel removes label "*":"*" which is mistakenly assigned on Teleport side
func removeDefaultLabel(l types.Labels) {
	v := types.Labels{
		"*": []string{"*"},
	}

	if reflect.DeepEqual(l, v) {
		delete(l, "*")
	}
}

// spec.0.options must be provided even it's empty
func checkOptionsProvided(d *schema.ResourceData) error {
	_, ok := d.GetOk("spec.0.options")
	if !ok {
		return trace.Errorf("You must provide spec.options for Role. If there are no options you need to specify, leave it empty: options { }")
	}

	return nil
}
