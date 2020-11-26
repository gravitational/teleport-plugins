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

	"github.com/gravitational/teleport-plugins/terraform/tfschema"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// dataSourceTeleportUser returns Teleport user data source definition
func dataSourceTeleportUser() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceUserRead,
		Schema:      tfschema.SchemaUserV2,
	}
}

// dataSourceUserRead reads Teleport user
func dataSourceUserRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c, err := getClient(m)
	if err != nil {
		return diagFromErr(err)
	}

	id := d.Id()

	u, err := c.GetUser(id, false)
	if err != nil {
		return diagFromErr(describeErr(err, "user"))
	}

	u2, ok := u.(*types.UserV2)
	if !ok {
		return diagFromErr(trace.Errorf("can not convert %T to *types.UserV2", u))
	}

	err = tfschema.SetUserV2(u2, d)
	if err != nil {
		return diagFromErr(err)
	}

	return diag.Diagnostics{}
}
