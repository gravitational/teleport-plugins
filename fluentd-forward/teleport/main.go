package teleport

import (
	"github.com/gravitational/teleport-plugins/fluentd/config"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"
	"golang.org/x/net/context"
)

var (
	// cl Teleport client
	cl *client.Client
)

// Init initializes Teleport client
func Init() error {
	if config.GetTeleportIdentityFile() != "" {
		err := connectUsingIdentityFile()
		if err != nil {
			return err
		}
		return nil
	}

	return connectUsingKeys()
}

// connectUsingIdentityFile tries to build API client using identity file
func connectUsingIdentityFile() error {
	var err error

	identity := client.LoadIdentityFile(config.GetTeleportIdentityFile())

	config := client.Config{
		Addrs:       []string{config.GetTeleportAddr()},
		Credentials: []client.Credentials{identity},
	}

	cl, err = client.New(context.Background(), config)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// connectUsingKeys tries to build API client using keys
func connectUsingKeys() error {
	var err error

	config := client.Config{
		Addrs: []string{config.GetTeleportAddr()},
		Credentials: []client.Credentials{
			client.LoadKeyPair(config.GetTeleportCert(), config.GetTeleportKey(), config.GetTeleportCA()),
		},
	}

	cl, err = client.New(context.Background(), config)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
