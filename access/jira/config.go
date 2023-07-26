/*
Copyright 2020-2021 Gravitational, Inc.

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

package main

import (
	"strings"

	"github.com/gravitational/teleport/integrations/access/jira"
	"github.com/gravitational/teleport/integrations/lib"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

const exampleConfig = `# example jira plugin configuration TOML file
[teleport]
# Teleport Auth/Proxy Server address.
#
# Should be port 3025 for Auth Server and 3080 or 443 for Proxy.
# For Teleport Cloud, should be in the form of "your-account.teleport.sh:443".
addr = "example.com:3025"

# Credentials.
#
# When using --format=file:
# identity = "/var/lib/teleport/plugins/jira/auth_id"    # Identity file
#
# When using --format=tls:
# client_key = "/var/lib/teleport/plugins/jira/auth.key" # Teleport TLS secret key
# client_crt = "/var/lib/teleport/plugins/jira/auth.crt" # Teleport TLS certificate
# root_cas = "/var/lib/teleport/plugins/jira/auth.cas"   # Teleport CA certs

[jira]
# Jira URL. For Jira Cloud, URL is of the form "https://[your-jira].atlassian.net":
url = "https://example.com/jira"
# Jira User name:
username = "jira-bot"
# Jira API Basic Auth token, or our password in case you're using Jira Server:
api_token = "token"
# Jira Project key:
project = "MYPROJ"
# Jira Issue type:
issue_type = "Task"

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/jira.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
`

func LoadConfig(filepath string) (*jira.Config, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conf := &jira.Config{}
	if err := t.Unmarshal(conf); err != nil {
		return nil, trace.Wrap(err)
	}
	if strings.HasPrefix(conf.Jira.APIToken, "/") {
		conf.Jira.APIToken, err = lib.ReadPassword(conf.Jira.APIToken)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}
