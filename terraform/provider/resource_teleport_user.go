package provider

import (
	"github.com/gravitational/teleport-plugins/terraform/tfschema"

	"github.com/hashicorp/terraform/helper/schema"
)

// teleport_user resource definition
func resourceTeleportUser() *schema.Resource {
	return &schema.Resource{
		Create: resourceTeleportUserUpsert,
		Read:   resourceTeleportUserRead,
		Update: resourceTeleportUserUpsert,
		Delete: resourceTeleportUserDelete,
		Exists: resourceTeleportUserExists,

		Schema: tfschema.SchemaUserV2(),
	}
}

func resourceTeleportUserUpsert(d *schema.ResourceData, m interface{}) error {
	// client := m.(*client.Client)

	// name := d.Get("name").(string)

	// tfRoles := d.Get("roles").([]interface{})
	// roles := make([]string, len(tfRoles))

	// for i, tfRole := range tfRoles {
	// 	roles[i] = tfRole.(string)
	// }

	// tfTraits := d.Get("trait").(*schema.Set).List()
	// traits := map[string][]string{}

	// for _, tfTrait := range tfTraits {
	// 	traitMap := tfTrait.(map[string]interface{})
	// 	name := traitMap["name"].(string)

	// 	tfValues := traitMap["value"].([]interface{})
	// 	values := make([]string, len(tfValues))

	// 	for i, value := range tfValues {
	// 		values[i] = value.(string)
	// 	}

	// 	traits[name] = values
	// }

	// user, err := types.NewUser(name)
	// if err != nil {
	// 	return trace.Wrap(err)
	// }

	// user.SetRoles(roles)
	// user.SetTraits(traits)

	// err = client.CreateUser(context.Background(), user)
	// if err != nil {
	// 	return trace.Wrap(err)
	// }

	// d.SetId(name)
	return nil
}

func resourceTeleportUserRead(d *schema.ResourceData, m interface{}) error {
	// 	client := m.(*client.Client)
	// 	name := d.Get("name").(string)

	// 	u, err := client.GetUser(name, false)
	// 	if err != nil {
	// 		return trace.Wrap(err)
	// 	}

	// 	user := u.(types.User)

	// 	err = client.UpdateUser(context.Background(), user)
	// 	if err != nil {
	// 		return trace.Wrap(err)
	// 	}

	// 	// traits := user.GetTraits()
	// 	// tfTraits := map[string]string{}

	// 	// for k, trait := range traits {
	// 	// 	tfTraits[k] = strings.Join(trait, " ")
	// 	// }

	// 	// d.Set("roles", user.GetRoles())
	// 	// d.Set("traits", tfTraits)

	return nil
}

func resourceTeleportUserDelete(d *schema.ResourceData, m interface{}) error {
	// 	client := m.(*client.Client)
	// 	name := d.Get("name").(string)

	// 	err := client.DeleteUser(context.Background(), name)
	// 	if err != nil {
	// 		return trace.Wrap(err)
	// 	}

	return nil
}

func resourceTeleportUserExists(d *schema.ResourceData, m interface{}) (bool, error) {
	// client := m.(*client.Client)
	// name := d.Get("name").(string)

	// user, err := client.GetUser(name, false)

	// if err != nil {
	// 	return false, err
	// }

	// return user != nil, nil

	return true, nil
}
