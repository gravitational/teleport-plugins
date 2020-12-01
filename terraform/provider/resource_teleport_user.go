package provider

import (
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/services"

	"github.com/gravitational/trace"

	"github.com/hashicorp/terraform/helper/schema"
)

// teleport_user resource definition
//
// TODOs:
// - [ ] Verify that this definition works with just username and roles
// - [ ] Write a tf provider test cases for the definition
// - [ ] Support user traits
//
func resourceTeleportUser() *schema.Resource {
	return &schema.Resource{
		Create: resourceTeleportUserUpsert,
		Read:   resourceTeleportUserRead,
		Update: resourceTeleportUserUpsert,
		Delete: resourceTeleportUserDelete,
		Exists: resourceTeleportUserExists,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"roles": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourceTeleportUserUpsert(d *schema.ResourceData, m interface{}) error {
	client := m.(*auth.Client)

	name := d.Get("name").(string)
	tfRoles := d.Get("roles").(*schema.Set)
	roles := make([]string, len(tfRoles))

	for i, tfRole := range tfRoles {
		roles[i] = tfRole.(string)
	}

	user, err := services.NewUser(name)
	if err != nil {
		return trace.Wrap(err)
	}

	user.SetRoles(roles)

	err = client.UpsertUser(user)
	if err != nil {
		return trace.Wrap(err)
	}

	d.SetId(name)
	return nil
}

func resourceTeleportUserRead(d *schema.ResourceData, m interface{}) error {
	client := m.(*auth.Client)
	name := d.Get("name").(string)

	u, err := client.GetUser(name, false)
	if err != nil {
		return trace.Wrap(err)
	}

	user := u.(services.User)

	//nolint:errcheck
	{
		d.Set("roles", user.GetRoles())
	}

	return nil
}

func resourceTeleportUserDelete(d *schema.ResourceData, m interface{}) error {
	return nil
}

func resourceTeleportUserExists(d *schema.ResourceData, m interface{}) (bool, error) {
	client := m.(*auth.Client)
	name := d.Get("name").(string)

	user, err := client.GetUser(name, false)

	if err != nil {
		return false, err
	}

	return user != nil, nil
}
