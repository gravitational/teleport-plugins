package main

import (
	"context"
	"os"
	"time"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

const (
	DefaultDir = "/var/lib/teleport/plugins/gitlab"
	PluginName = "teleport-gitlab"
)

func main() {
	utils.InitLogger()
	runner := utils.NewPluginApp(PluginName, exampleConfig, Version, Gitref)
	runner.ParseCommand(os.Args[1:], run)
}

func run(configPath string, insecure bool, debug bool) error {
	conf, err := LoadConfig(configPath)
	if err != nil {
		return trace.Wrap(err)
	}

	err = utils.SetupLogger(conf.Log)
	if err != nil {
		return err
	}
	if debug {
		log.SetLevel(log.DebugLevel)
		log.Debugf("DEBUG logging enabled")
	}

	conf.HTTP.Insecure = insecure
	app, err := NewApp(*conf)
	if err != nil {
		return trace.Wrap(err)
	}

	go utils.ServeSignals(app, 15*time.Second)

	return trace.Wrap(
		app.Run(context.Background()),
	)
}
