package config

import (
	"os"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	// fluentdCA is the CLI arg name for fluentd root CA path
	fluentdCA = "fluentd-ca"

	// fluentdCert is the CLI arg name for fluentd cert path
	fluentdCert = "fluentd-cert"

	// fluentdKey is the CLI arg name for fluentd private key path
	fluentdKey = "fluentd-key"

	// fluentdUrl is the CLI arg name for url of fluentd instance
	fluentdURL = "fluentd-url"

	// teleportAddr is the CLI arg name for Teleport address
	teleportAddr = "teleport-addr"

	// teleportIdentityFile is the CLI arg name for Teleport identity file
	teleportIdentityFile = "teleport-identity-file"

	// teleportCA is the CLI arg name for Teleport CA file
	teleportCA = "teleport-ca"

	// teleportCert is the CLI arg name for Teleport cert file
	teleportCert = "teleport-cert"

	// teleportKey is the CLI arg name for Teleport key file
	teleportKey = "teleport-key"

	// teleportProfileName is the CLI arg name for Teleport profile name
	teleportProfileName = "teleport-profile-name"

	// teleportProfileDir is the CLI arg name for Teleport profile dir
	teleportProfileDir = "teleport-profile-dir"

	// storageDir is the CLI arg name for storage dir
	storageDir = "storage-dir"
)

// init initialises viper args
func init() {
	viper.SetEnvPrefix("FDF")
	viper.AutomaticEnv()

	pflag.StringP(teleportAddr, "p", "", "Teleport addr")
	pflag.StringP(teleportIdentityFile, "i", "", "Teleport identity file")
	pflag.String(teleportCA, "", "Teleport TLS CA file")
	pflag.String(teleportCert, "", "Teleport TLS cert file")
	pflag.String(teleportKey, "", "Teleport TLS key file")
	pflag.String(teleportProfileName, "", "Teleport profile name")
	pflag.String(teleportProfileDir, "", "Teleport profile dir")

	pflag.StringP(fluentdURL, "u", "", "fluentd url")
	pflag.StringP(fluentdCA, "a", "", "fluentd TLS CA path")
	pflag.StringP(fluentdCert, "c", "", "fluentd TLS certificate path")
	pflag.StringP(fluentdKey, "k", "", "fluentd TLS key path")

	pflag.StringP(storageDir, "s", "", "Storage directory")

	//https://stackoverflow.com/questions/56129533/tls-with-certificate-private-key-and-pass-phrase
	//pflag.StringP(FluentdPassphrase, "p", "", "fluentd key passphrase")

	pflag.Parse()

	viper.BindPFlags(pflag.CommandLine)
}

// Validate validates that required CLI args are present
func Validate() error {
	err := validateFluentd()
	if err != nil {
		return err
	}

	err = validateTeleport()
	if err != nil {
		return err
	}

	if GetStorageDir() == "" {
		return trace.BadParameter("Pass storage dir using --%s", storageDir)
	}

	return nil
}

// validateFluentd validates Fluentd CLI args
func validateFluentd() error {
	if GetFluentdURL() == "" {
		return trace.BadParameter("Pass fluentd url using --%s", fluentdURL)
	}

	if GetFluentdCA() != "" && !fileExists(GetFluentdCA()) {
		return trace.BadParameter("Fluentd CA file does not exist %s", GetFluentdCA())
	}

	if GetFluentdCert() == "" {
		return trace.BadParameter("HTTPS must be enabled in fluentd. Please, specify TLS certificate --%s", fluentdCert)
	}

	if !fileExists(GetFluentdCert()) {
		return trace.BadParameter("Fluentd cert file does not exist %s", GetFluentdCert())
	}

	if GetFluentdKey() == "" {
		return trace.BadParameter("HTTPS must be enabled in fluentd. Please, specify TLS key --%s", fluentdKey)
	}

	if !fileExists(GetFluentdKey()) {
		return trace.BadParameter("Fluentd key file does not exist %s", GetFluentdKey())
	}

	log.WithFields(log.Fields{"url": GetFluentdURL()}).Debug("Using Fluentd url")
	log.WithFields(log.Fields{"ca": GetFluentdCA()}).Debug("Using Fluentd ca")
	log.WithFields(log.Fields{"cert": GetFluentdCert()}).Debug("Using Fluentd cert")
	log.WithFields(log.Fields{"key": GetFluentdKey()}).Debug("Using Fluentd key")

	return nil
}

// validateTeleport validates Teleport CLI args
func validateTeleport() error {
	// If any of key files are specified
	if GetTeleportCA() != "" || GetTeleportCert() != "" || GetTeleportKey() != "" {
		// Then addr becomes required
		if GetTeleportAddr() == "" {
			return trace.BadParameter("Please, specify Teleport addr using --%s", teleportAddr)
		}

		// And all of the files must be specified
		if GetTeleportCA() == "" {
			return trace.BadParameter("Please, provide Teleport TLS CA --%s", teleportCA)
		}

		if !fileExists(GetTeleportCA()) {
			return trace.BadParameter("Teleport TLS CA file does not exist %s", GetTeleportCA())
		}

		if GetTeleportCert() == "" {
			return trace.BadParameter("Please, provide Teleport TLS certificate --%s", teleportCert)
		}

		if !fileExists(GetTeleportCert()) {
			return trace.BadParameter("Teleport TLS certificate file does not exist %s", GetTeleportCert())
		}

		if GetTeleportKey() == "" {
			return trace.BadParameter("Please, provide Teleport TLS key --%s", teleportKey)
		}

		if !fileExists(GetTeleportKey()) {
			return trace.BadParameter("Teleport TLS key file does not exist %s", GetTeleportKey())
		}

		log.WithFields(log.Fields{"addr": GetTeleportAddr()}).Debug("Using Teleport addr")
		log.WithFields(log.Fields{"ca": GetTeleportCA()}).Debug("Using Teleport CA")
		log.WithFields(log.Fields{"cert": GetTeleportCert()}).Debug("Using Teleport cert")
		log.WithFields(log.Fields{"key": GetTeleportKey()}).Debug("Using Teleport key")
	} else {
		if GetTeleportIdentityFile() == "" {
			// Otherwise, we need identity file
			return trace.BadParameter("Please, specify either identity file or certificates to connect to Teleport")
		}

		if !fileExists(GetTeleportIdentityFile()) {
			return trace.BadParameter("Teleport identity file does not exist %s", GetTeleportIdentityFile())
		}
	}

	return nil
}

// GetFluentdURL returns fluentd url
func GetFluentdURL() string {
	return viper.GetString(fluentdURL)
}

// GetFluentdUrl returns path to fluentd cert
func GetFluentdCert() string {
	return viper.GetString(fluentdCert)
}

// GetFluentdUrl returns path to fluentd key
func GetFluentdKey() string {
	return viper.GetString(fluentdKey)
}

// GetFluentdUrl returns path to fluentd CA
func GetFluentdCA() string {
	return viper.GetString(fluentdCA)
}

// GetTeleportAddr returns Teleport addr
func GetTeleportAddr() string {
	return viper.GetString(teleportAddr)
}

// GetTeleportIdentityFile returns Teleport identity file
func GetTeleportIdentityFile() string {
	return viper.GetString(teleportIdentityFile)
}

// GetTeleportProfileName returns Teleport profile name
func GetTeleportProfileName() string {
	return viper.GetString(teleportProfileName)
}

// GetTeleportProfileDir returns Teleport profile dir
func GetTeleportProfileDir() string {
	return viper.GetString(teleportProfileDir)
}

// GetTeleportCA returns Teleport CA file path
func GetTeleportCA() string {
	return viper.GetString(teleportCA)
}

// GetTeleportCert returns Teleport cert file path
func GetTeleportCert() string {
	return viper.GetString(teleportCert)
}

// GetTeleportKey returns Teleport cert file path
func GetTeleportKey() string {
	return viper.GetString(teleportKey)
}

// GetStorageDir returns storage dir
func GetStorageDir() string {
	return viper.GetString(storageDir)
}

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
