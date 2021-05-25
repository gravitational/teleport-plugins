package teleport

// import (
// 	"log"
// 	"time"

// 	"github.com/gravitational/teleport-plugins/fluentd/config"
// 	"github.com/gravitational/teleport/api/client"
// 	"github.com/gravitational/trace"
// 	"github.com/sirupsen/logrus"
// 	"golang.org/x/net/context"
// )

// var (
// 	// cl Teleport client
// 	cl *client.Client
// )

// // Init initializes Teleport client
// func Init() error {
// 	if config.GetTeleportIdentityFile() != "" {
// 		err := connectUsingIdentityFile()
// 		if err != nil {
// 			return err
// 		}
// 		return nil
// 	}

// 	return connectUsingKeys()
// }

// func Close() {
// 	cl.Close()
// }

// func Test() {
// 	e, cursor, err := cl.SearchEvents(context.Background(), time.Now().AddDate(-5, 0, 0), time.Now().UTC(), "default", []string{}, 50, "")
// 	if err != nil {
// 		log.Fatalf("%v", err)
// 	}

// 	logrus.Printf("%v", len(e))
// 	logrus.Printf("%v", cursor)
// }

// // connectUsingIdentityFile tries to build API client using identity file
// func connectUsingIdentityFile() error {
// 	var err error

// 	identity := client.LoadIdentityFile(config.GetTeleportIdentityFile())

// 	config := client.Config{
// 		Addrs:       []string{config.GetTeleportAddr()},
// 		Credentials: []client.Credentials{identity},
// 	}

// 	cl, err = client.New(context.Background(), config)
// 	if err != nil {
// 		return trace.Wrap(err)
// 	}

// 	return nil
// }

// // connectUsingKeys tries to build API client using keys
// func connectUsingKeys() error {
// 	var err error

// 	config := client.Config{
// 		Addrs: []string{config.GetTeleportAddr()},
// 		Credentials: []client.Credentials{
// 			client.LoadKeyPair(config.GetTeleportCert(), config.GetTeleportKey(), config.GetTeleportCA()),
// 		},
// 	}

// 	cl, err = client.New(context.Background(), config)
// 	if err != nil {
// 		return trace.Wrap(err)
// 	}

// 	return nil
// }
