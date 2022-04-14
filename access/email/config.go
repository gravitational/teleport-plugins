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

package main

import (
	_ "embed"

	"github.com/gravitational/teleport-plugins/access/config"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// DeliveryConfig represents email recipients config
type DeliveryConfig struct {
	Sender     string
	Recipients []string
}

// MailgunConfig holds Mailgun-specific configuration options.
type MailgunConfig struct {
	Domain         string
	PrivateKey     string `toml:"private_key"`
	PrivateKeyFile string `toml:"private_key_file"`
	APIBase        string `toml:"-"`
}

// SMTPConfig is SMTP-specific configuration options
type SMTPConfig struct {
	Host         string
	Port         int
	Username     string
	Password     string
	PasswordFile string `toml:"password_file"`
}

// Config stores the full configuration for the teleport-email plugin to run.
type Config struct {
	Teleport         lib.TeleportConfig   `toml:"teleport"`
	Mailgun          *MailgunConfig       `toml:"mailgun"`
	SMTP             *SMTPConfig          `toml:"smtp"`
	Delivery         DeliveryConfig       `toml:"delivery"`
	RoleToRecipients config.RecipientsMap `toml:"role_to_recipients"`
	Log              logger.Config        `toml:"log"`
}

// TODO: Replace auth_server with addr once it is merged
const exampleConfig = `# Example email plugin configuration TOML file

[teleport]
auth_server = "0.0.0.0:3025"                              # Teleport Auth Server GRPC API address

# When using --format=file:
# identity = "/var/lib/teleport/plugins/email/auth_id"    # Identity file
#
# When using --format=tls:
# client_key = "/var/lib/teleport/plugins/email/auth.key" # Teleport TLS secret key
# client_crt = "/var/lib/teleport/plugins/email/auth.crt" # Teleport TLS certificate
# root_cas = "/var/lib/teleport/plugins/email/auth.cas"   # Teleport CA certs

[mailgun]
domain = "your-domain-name"
private_key = "xoxb-11xx"
# private_key_file = "/var/lib/teleport/plugins/email/mailgun_private_key"

[smtp]
host = "smtp.gmail.com"
port = 587
username = "username@gmail.com"
password = ""
# password_file = "/var/lib/teleport/plugins/email/smtp_password"

[delivery]
sender = "noreply@example.com" # From: email address

[role_to_recipients]
"dev" = "dev-manager@example.com" # All requests to 'dev' role will be sent to this address
"*" = ["root@example.com", "admin@example.com"] # These recipients will receive review requests not handled by the roles above

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/email.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
`

// LoadConfig reads the config file, initializes a new Config struct object, and returns it.
// Optionally returns an error if the file is not readable, or if file format is invalid.
func LoadConfig(filepath string) (*Config, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conf := &Config{}
	if err := t.Unmarshal(conf); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}

// CheckAndSetDefaults checks MailgunConfig struct and set defaults if needed
func (c *MailgunConfig) CheckAndSetDefaults() error {
	var err error

	if c.PrivateKey == "" {
		if c.PrivateKeyFile == "" {
			return trace.BadParameter("Please, specify mailgun.private_key or mailgun.private_key_file!")
		}

		c.PrivateKey, err = lib.ReadPassword(c.PrivateKeyFile)
		if err != nil {
			return trace.Wrap(err)
		}

		if c.PrivateKey == "" {
			return trace.BadParameter("Please, provide mailgun.private_key or mailgun.private_key_file to use Mailgun!"+
				"Ensure that password file %v is not empty!", c.PrivateKeyFile)
		}

	}

	if c.Domain == "" {
		return trace.BadParameter("Please, provide mailgun.domain to use Mailgun")
	}

	return nil
}

// CheckAndSetDefaults checks SMTPConfig struct and set defaults if needed
func (c *SMTPConfig) CheckAndSetDefaults() error {
	var err error

	if c.Host == "" {
		return trace.BadParameter("Please, provide smtp.host to use SMTP")
	}

	if c.Port == 0 {
		c.Port = 587
	}

	if c.Username == "" {
		return trace.BadParameter("Please, provide smtp.username to use SMTP")
	}

	if c.Password == "" {
		if c.PasswordFile == "" {
			return trace.BadParameter("Please, specify smtp.password or smtp.password_file!")
		}

		c.Password, err = lib.ReadPassword(c.PasswordFile)
		if err != nil {
			return trace.Wrap(err)
		}

		if c.Password == "" {
			return trace.BadParameter("Please, provide smtp.password or smtp.password_file!"+
				"Ensure that password file %v is not empty!", c.PasswordFile)
		}
	}

	return nil
}

// CheckAndSetDefaults checks the config struct for any logical errors, and sets default values
// if some values are missing.
// If critical values are missing and we can't set defaults for them — this will return an error.
func (c *Config) CheckAndSetDefaults() error {
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}

	if len(c.Delivery.Recipients) > 0 {
		if len(c.RoleToRecipients) > 0 {
			return trace.BadParameter("provide either delivery.recipients or role_to_recipients, not both.")
		}

		c.RoleToRecipients = config.RecipientsMap{
			types.Wildcard: c.Delivery.Recipients,
		}
		c.Delivery.Recipients = nil
	}

	// Validate emails in user aliases
	for _, e := range c.Delivery.Recipients {
		if !lib.IsEmail(e) {
			return trace.BadParameter("Invalid email address %v in delivery.recipients", e)
		}
	}

	for role, recipientsList := range c.RoleToRecipients {
		for _, recipient := range recipientsList {
			if !lib.IsEmail(recipient) {
				return trace.BadParameter("Invalid email address %v in role_to_recipients.%s", recipient, role)
			}
		}
	}

	if len(c.RoleToRecipients) == 0 {
		return trace.BadParameter("missing required value role_to_recipients.")
	} else if len(c.RoleToRecipients[types.Wildcard]) == 0 {
		return trace.BadParameter("missing required value role_to_recipients[%v].", types.Wildcard)
	}

	// Validate mailer settings
	if c.SMTP == nil && c.Mailgun == nil {
		return trace.BadParameter("Provide either [mailgun] or [smtp] sections to work with plugin")
	}

	// Validate Mailgun settings
	if c.Mailgun != nil {
		err := c.Mailgun.CheckAndSetDefaults()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	if c.SMTP != nil {
		err := c.SMTP.CheckAndSetDefaults()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}
