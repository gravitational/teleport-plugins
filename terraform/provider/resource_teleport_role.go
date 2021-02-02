package provider

import (
	"github.com/gravitational/teleport-plugins/terraform/tfschema"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceTeleportRole() *schema.Resource {
	return &schema.Resource{
		Create: resourceTeleportRoleUpsert,
		Read:   resourceTeleportRoleRead,
		Update: resourceTeleportRoleUpsert,
		Delete: resourceTeleportRoleDelete,
		Exists: resourceTeleportRoleExists,

		Schema: tfschema.SchemaRoleV3(),
	}
}

func resourceTeleportRoleUpsert(d *schema.ResourceData, m interface{}) error {
	// client := m.(*client.Client)
	name := d.Get("name").(string)

	// roleSpec := services.RoleSpecV3{}

	// role, err := services.NewRole(name, roleSpec)
	// if err != nil {
	// 	return trace.Wrap(err)
	// }

	// FIXME: Client.CreateRole() is not implemented yet in api/client
	// err = client.CreateRole(role)
	// if err != nil {
	// 	return trace.Wrap(err)
	// }

	d.SetId(name)
	return nil
}

func resourceTeleportRoleRead(d *schema.ResourceData, m interface{}) error {
	// client := m.(*auth.Client)
	// name := d.Get("name").(string)

	return nil
}

func resourceTeleportRoleDelete(d *schema.ResourceData, m interface{}) error {
	// client := m.(*auth.Client)
	// name := d.Get("name").(string)

	return nil
}

func resourceTeleportRoleExists(d *schema.ResourceData, m interface{}) (bool, error) {
	// client := m.(*auth.Client)
	// name := d.Get("name").(string)

	return true, nil
}
