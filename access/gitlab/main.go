package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"

	"github.com/gravitational/kingpin"
	"github.com/gravitational/trace"
)

const (
	DefaultDir = "/var/lib/teleport/plugins/gitlab"
)

func main() {
	logger.Init()
	app := kingpin.New("teleport-gitlab", "Teleport plugin for access requests approval via GitLab.")

	app.Command("configure", "Prints an example .TOML configuration file.")
	app.Command("version", "Prints teleport-gitlab version and exits.")

	startCmd := app.Command("start", "Starts a Teleport GitLab plugin.")
	path := startCmd.Flag("config", "TOML config file path").
		Short('c').
		Default("/etc/teleport-gitlab.toml").
		String()
	debug := startCmd.Flag("debug", "Enable verbose logging to stderr").
		Short('d').
		Bool()
	insecure := startCmd.Flag("insecure-no-tls", "Disable TLS for the callback server").
		Default("false").
		Bool()

	selectedCmd, err := app.Parse(os.Args[1:])
	if err != nil {
		lib.Bail(err)
	}

	switch selectedCmd {
	case "configure":
		fmt.Print(exampleConfig)
	case "version":
		lib.PrintVersion(app.Name, Version, Gitref)
	case "start":
		if err := run(*path, *insecure, *debug); err != nil {
			lib.Bail(err)
		} else {
			logger.Standard().Info("Successfully shut down")
		}
	}
}

func run(configPath string, insecure bool, debug bool) error {
	conf, err := LoadConfig(configPath)
	if err != nil {
		return trace.Wrap(err)
	}

	logConfig := conf.Log
	if debug {
		logConfig.Severity = "debug"
	}
	if err = logger.Setup(logConfig); err != nil {
		return err
	}
	if debug {
		logger.Standard().Debugf("DEBUG logging enabled")
	}

	conf.HTTP.Insecure = insecure
	app, err := NewApp(*conf)
	if err != nil {
		return trace.Wrap(err)
	}

	go lib.ServeSignals(app, 15*time.Second)

	return trace.Wrap(
		app.Run(context.Background()),
	)
}
