package main

import (
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/fluentd/config"
	_ "github.com/gravitational/teleport-plugins/fluentd/config"
	"github.com/gravitational/teleport-plugins/fluentd/fluentd"
	"github.com/gravitational/trace"
)

type dummy struct {
	A string `json:"a"`
	B string `json:"b"`
}

func main() {
	log.WithFields(log.Fields{"version": Version, "sha": Sha}).Printf("Teleport fluentd-forwarder")

	c, err := config.New()
	if err != nil {
		log.Error(trace.DebugReport(err))
		config.PrintUsage()
		os.Exit(-1)
	}

	f, err := fluentd.New(c)
	if err != nil {
		log.Error(trace.DebugReport(err))
		os.Exit(-1)
	}

	err = f.Send(dummy{A: "1", B: "2"})
	if err != nil {
		log.Fatal(trace.DebugReport(err))
		os.Exit(-1)
	}

	// err = fluentd.Init()
	// if err != nil {
	// 	log.Fatal(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }

	// err = teleport.Init()
	// if err != nil {
	// 	log.Fatal(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }
	// defer teleport.Close()

	// teleport.Test()

	// err = cursor.Init()
	// if err != nil {
	// 	log.Fatal(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }
	// defer cursor.Close()
}
