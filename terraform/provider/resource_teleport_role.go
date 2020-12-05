package provider

import (
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceTeleportRole() *schema.Resource {
	return &schema.Resource{
		Create: resourceTeleportRoleUpsert,
		Read:   resourceTeleportRoleRead,
		Update: resourceTeleportRoleUpsert,
		Delete: resourceTeleportRoleDelete,
		Exists: resourceTeleportRoleExists,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceTeleportRoleUpsert(d *schema.ResourceData, m interface{}) error {
	client := m.(*auth.Client)
	name := d.Get("name").(string)

	roleSpec := services.RoleSpecV3{}

	role, err := services.NewRole(name, roleSpec)
	if err != nil {
		return trace.Wrap(err)
	}

	err = client.CreateRole(role)
	if err != nil {
		return trace.Wrap(err)
	}

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
