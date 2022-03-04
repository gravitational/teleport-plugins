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
	"net"
	"os"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/grpclog"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	// addrKey is the key of addr field
	addrKey = "addr"

	// certPathKey is the key of path to certificate file
	certPathKey = "cert_path"

	// keyPathKey is the key of path to key file
	keyPathKey = "key_path"

	// rootCaPathKey is the key of path to root ca file
	rootCaPathKey = "root_ca_path"

	// profileName is the key of profile name in Teleport config
	profileName = "profile_name"

	// profileDir is the key of profile dir in Teleport config
	profileDir = "profile_dir"

	// identityFilePath is the key of identity file path in Teleport config
	identityFilePath = "identity_file_path"

	// identityFile is the key of identity file content in Teleport config
	identityFile = "identity_file"

	// namePath path to resource name in ResourceData
	namePath = "metadata.0.name"
)

// init initialises loggers, grpc specifically requires to do it here
func init() {
	level, err := log.ParseLevel(os.Getenv("TF_LOG"))
	if err != nil {
		log.SetLevel(log.ErrorLevel)
	} else {
		log.SetLevel(level)
	}

	log.SetFormatter(&log.TextFormatter{})

	// show GRPC debug logs only if TF_LOG=DEBUG
	if log.GetLevel() >= log.DebugLevel {
		l := grpclog.NewLoggerV2(log.StandardLogger().Out, log.StandardLogger().Out, log.StandardLogger().Out)
		grpclog.SetLoggerV2(l)
	}
}

// Provider builds Teleport Terraform Resource Provider
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			addrKey: {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_ADDR", ""),
				Description: "host:port where Teleport Auth server is running.",
			},
			certPathKey: {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_CERT", ""),
				Description: "Path to Teleport auth certificate file.",
			},
			keyPathKey: {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_KEY", ""),
				Description: "Path to Teleport auth key file.",
			},
			rootCaPathKey: {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_ROOT_CA", ""),
				Description: "Path to Teleport Root CA",
			},
			profileName: {
				Type:        schema.TypeString,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_PROFILE_NAME", ""),
				Optional:    true,
				Description: "Teleport profile name",
			},
			profileDir: {
				Type:        schema.TypeString,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_PROFILE_PATH", ""),
				Optional:    true,
				Description: "Teleport profile path",
			},
			identityFilePath: {
				Type:        schema.TypeString,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_IDENTITY_FILE_PATH", ""),
				Optional:    true,
				Description: "Teleport identity file path",
			},
			identityFile: {
				Type:        schema.TypeString,
				DefaultFunc: schema.EnvDefaultFunc("TF_TELEPORT_IDENTITY_FILE", ""),
				Optional:    true,
				Description: "Teleport identity file content",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"teleport_role":                      resourceTeleportRole(),
			"teleport_user":                      resourceTeleportUser(),
			"teleport_github_connector":          resourceTeleportGithubConnector(),
			"teleport_oidc_connector":            resourceTeleportOIDCConnector(),
			"teleport_saml_connector":            resourceTeleportSAMLConnector(),
			"teleport_provision_token":           resourceTeleportProvisionToken(),
			"teleport_trusted_cluster":           resourceTeleportTrustedCluster(),
			"teleport_auth_preference":           resourceTeleportAuthPreference(),
			"teleport_cluster_networking_config": resourceTeleportClusterNetworkingConfig(),
			"teleport_session_recording_config":  resourceTeleportSessionRecordingConfig(),
			"teleport_app":                       resourceTeleportApp(),
			"teleport_database":                  resourceTeleportDatabase(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"teleport_role":                      dataSourceTeleportRole(),
			"teleport_user":                      dataSourceTeleportUser(),
			"teleport_github_connector":          dataSourceGithubConnector(),
			"teleport_oidc_connector":            dataSourceTeleportOIDCConnector(),
			"teleport_saml_connector":            dataSourceTeleportSAMLConnector(),
			"teleport_provision_token":           dataSourceTeleportProvisionToken(),
			"teleport_trusted_cluster":           dataSourceTeleportTrustedCluster(),
			"teleport_auth_preference":           dataSourceTeleportAuthPreference(),
			"teleport_cluster_networking_config": dataSourceTeleportClusterNetworkingConfig(),
			"teleport_session_recording_config":  dataSourceTeleportSessionRecordingConfig(),
			"teleport_app":                       dataSourceTeleportApp(),
			"teleport_database":                  dataSourceTeleportDatabase(),
		},
		ConfigureContextFunc: configureContext,
	}
}

// configureContext accepts provider settings schema,
// initializes a Teleport's `auth.Client` and returns it.
func configureContext(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	config, err := getConfig(d)
	if err != nil {
		return nil, diagFromErr(err)
	}

	client, err := client.New(ctx, *config)
	if err != nil {
		return nil, diagFromErr(err)
	}

	return client, diag.Diagnostics{}
}

// getConfig loads client config from a given identity source
func getConfig(d *schema.ResourceData) (*client.Config, error) {
	var creds []client.Credentials = make([]client.Credentials, 0)

	addr, err := getAddr(d, addrKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.WithFields(log.Fields{"addr": addr}).Debug("Addr provided")

	_, okKey := d.GetOk(keyPathKey)
	_, okIdentity := d.GetOk(identityFile)
	_, okIdentityPath := d.GetOk(identityFilePath)
	_, okProfile := d.GetOk(profileName)

	if okKey {
		log.Debug("Certificate files provided")

		c, err := getConfigFromCerts(d)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		creds = append(creds, c)
	}

	if okIdentity {
		log.Debug("Identity provided")

		c, err := getConfigFromIdentity(d)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		creds = append(creds, c)
	}

	if okIdentityPath {
		log.Debug("Identity file provided")

		c, err := getConfigFromIdentityFile(d)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		creds = append(creds, c)
	}

	if okProfile || len(creds) == 0 {
		log.Debug("Using profile as the default auth method")

		c, err := getConfigFromProfile(d)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		creds = append(creds, c)
	}

	return &client.Config{
		Addrs:       []string{addr},
		Credentials: creds,
	}, nil

}

// getConfigFromIdentityFile returns client configuration which uses identity file
func getConfigFromIdentityFile(d *schema.ResourceData) (client.Credentials, error) {
	p := d.Get(identityFilePath)
	path, ok := p.(string)
	if !ok {
		return nil, trace.BadParameter("can not convert Teleport config value %s %s to string", identityFilePath, p)
	}

	log.WithField("path", path).Debug("Identity file is set")

	return client.LoadIdentityFile(path), nil
}

// getConfigFromIdentity returns client configuration which uses identity file
func getConfigFromIdentity(d *schema.ResourceData) (client.Credentials, error) {
	c := d.Get(identityFile)
	content, ok := c.(string)
	if !ok {
		return nil, trace.BadParameter("can not convert Teleport config value %s %s to string", identityFile, c)
	}

	log.Debug("Identity file is read from a value (env or config)")

	return client.LoadIdentityFileFromString(content), nil
}

// getConfigFromProfile returns client configuration which uses tsh profile
func getConfigFromProfile(d *schema.ResourceData) (client.Credentials, error) {
	var name, dir string
	var ok bool

	n := d.Get(profileName)
	if n != nil {
		name, ok = n.(string)
		if !ok {
			return nil, trace.BadParameter("can not convert Teleport config value %s %s to string", profileName, n)
		}
	}

	v := d.Get(profileDir)
	if v != nil {
		dir, ok = v.(string)
		if !ok {
			return nil, trace.BadParameter("can not convert Teleport config value %s %s to string", profileDir, v)
		}
	}

	log.WithFields(log.Fields{"name": name, "dir": dir}).Debug("Profile is set")

	return client.LoadProfile(name, dir), nil
}

// getConfigFromCerts returns client configuration which uses certificates
func getConfigFromCerts(d *schema.ResourceData) (client.Credentials, error) {
	certPath, err := getPath(d, certPathKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	keyPath, err := getPath(d, keyPathKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	rootCAsPath, err := getPath(d, rootCaPathKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.WithFields(
		log.Fields{"certPath": certPath, "keyPath": keyPath, "rootCAsPath": rootCAsPath},
	).Debug("Key files are set")

	return client.LoadKeyPair(certPath, keyPath, rootCAsPath), nil
}

// getPath reads path with specified key from provider configuration and checks if it exists
func getPath(d *schema.ResourceData, key string) (string, error) {
	const notFoundErrorMsg = "file %v referenced by %v in Teleport provider configuration not found." +
		" Use 'tctl user auth' to generate it."

	p, ok := d.GetOk(key)
	if !ok {
		return "", trace.BadParameter("set %v in Teleport provider configuration", key)
	}

	path, ok := p.(string)
	if !ok {
		return "", trace.BadParameter("value for %v in Teleport provider configuration is not a string", key)
	}

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return path, trace.BadParameter(notFoundErrorMsg, path, key)
	}

	if err != nil {
		return "", trace.Wrap(err)
	}

	return path, nil
}

// getAddr reads API address from config, validates and returns it
func getAddr(d *schema.ResourceData, key string) (string, error) {
	a, ok := d.GetOk(key)
	if !ok {
		return "", trace.BadParameter("set %v in Teleport provider configuration", key)
	}

	addr, ok := a.(string)
	if !ok {
		return "", trace.BadParameter("value for %v in Teleport provider configuration is not a string", key)
	}

	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "", trace.BadParameter(
			"invalid format of Teleport provider %v variable %v, must be host:port (%v)", key, addr, err.Error(),
		)
	}

	return addr, nil
}

// getClient gets client from metdata, is used in specific providers
func getClient(m interface{}) (*client.Client, error) {
	c, ok := m.(*client.Client)
	if !ok {
		return nil, trace.BadParameter("wrong client instance passed: %T, should be client.Client", m)
	}

	return c, nil
}

// getResourceName gets resource name from Terraform ResourceData
func getResourceName(d *schema.ResourceData, r string) (string, error) {
	n, ok := d.GetOk(namePath)
	if !ok {
		return "", trace.BadParameter(r+"%v is blank, set it in .tf file", namePath)
	}

	name, ok := n.(string)
	if !ok {
		return "", trace.BadParameter(r+"%v must be string, set in in .tf file", namePath)
	}

	return name, nil
}
