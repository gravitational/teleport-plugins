package provider

import (
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider builds Teleport Terraform Resource Provider
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"addr": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_ADDR", ""),
				Description: "host:port where Teleport Auth server is running.",
			},
			"cert_path": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_CERT", ""),
				Description: "Path to Teleport auth certificate file.",
			},
			"key_path": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_KEY", ""),
				Description: "Path to Teleport auth key file.",
			},
			"root_ca_path": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_ROOT_CA", ""),
				Description: "Path to Teleport Root CA",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"teleport_user": resourceTeleportUser(),
			"teleport_role": resourceTeleportRole(),
		},
		ConfigureFunc: providerConfigure,
	}
}

// providerConfigure accepts provider settings schema,
// initializes a Teleport's `auth.Client` and returns it.
func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	certPath := d.Get("cert_path").(string)
	keyPath := d.Get("key_path").(string)
	rootCAsPath := d.Get("root_ca_path").(string)

	// TODO: Verify Addr format is [protocol://]host:port
	addr := d.Get("addr").(string)

	tlsConfig, err := loadTLSConfig(certPath, keyPath, rootCAsPath)
	if err != nil {
		// TODO decorate the error into a Terraform-friendly Diags
		return nil, trace.Wrap(err)
	}

	config := client.Config{
		Addrs:           []string{addr},
		Dialer:          nil,
		DialTimeout:     0,
		KeepAlivePeriod: 0,
		KeepAliveCount:  0,
		TLS:             tlsConfig,
	}

	client, err := client.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return client, nil
}
