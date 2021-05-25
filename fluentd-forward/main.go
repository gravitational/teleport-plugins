package main

import (
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/trace"
)

type dummy struct {
	A string `json:"a"`
	B string `json:"b"`
}

func init() {
	initConfig()
}

func main() {
	log.WithFields(log.Fields{"version": Version, "sha": Sha}).Printf("Teleport fluentd-forwarder")

	c, err := newConfig()
	if err != nil {
		log.Error(trace.DebugReport(err))
		printUsage()
		os.Exit(-1)
	}

	f, err := NewFluentdClient(c)
	if err != nil {
		log.Error(trace.DebugReport(err))
		os.Exit(-1)
	}

	t, err := NewTeleportClient(c)
	if err != nil {
		log.Fatal(trace.DebugReport(err))
		os.Exit(-1)
	}
	defer t.Close()

	t.Test()

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

	// err = cursor.Init()
	// if err != nil {
	// 	log.Fatal(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }
	// defer cursor.Close()
}
