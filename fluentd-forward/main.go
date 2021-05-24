package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/fluentd/config"
	_ "github.com/gravitational/teleport-plugins/fluentd/config"
	"github.com/gravitational/teleport-plugins/fluentd/cursor"
	"github.com/gravitational/teleport-plugins/fluentd/fluentd"
	"github.com/gravitational/teleport-plugins/fluentd/teleport"
	"github.com/gravitational/trace"
	"github.com/spf13/pflag"
)

type dummy struct {
	A string `json:"a"`
	B string `json:"b"`
}

func main() {
	log.WithFields(log.Fields{"version": Version, "sha": Sha}).Printf("Teleport fluentd-forwarder")

	err := config.Validate()
	if err != nil {
		fmt.Println()
		pflag.PrintDefaults()
		fmt.Println()
		fmt.Println(err.Error())
		os.Exit(-1)
	}

	err = fluentd.Init()
	if err != nil {
		log.Fatal(trace.DebugReport(err))
		os.Exit(-1)
	}

	err = teleport.Init()
	if err != nil {
		log.Fatal(trace.DebugReport(err))
		os.Exit(-1)
	}

	err = cursor.Init()
	if err != nil {
		log.Fatal(trace.DebugReport(err))
		os.Exit(-1)
	}

	err = fluentd.Send(dummy{A: "1", B: "2"})
	if err != nil {
		log.Fatal(trace.DebugReport(err))
		os.Exit(-1)
	}
}
