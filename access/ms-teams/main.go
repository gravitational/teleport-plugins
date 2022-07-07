package main

import (
	"context"
	"os"
	"time"

	"github.com/gravitational/kingpin"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

var (
	appName                 = "teleport-ms-teams"
	gracefulShutdownTimeout = 15 * time.Second
	configPath              = "/etc/teleport-ms-teams.toml"
)

func main() {
	logger.Init()
	app := kingpin.New(appName, "Teleport MS Teams plugin")

	app.Command("version", "Prints teleport-ms-teams version and exits")
	configureCmd := app.Command("configure", "Generates plugin and bot configuration")

	targetDir := configureCmd.Arg("dir", "Path to target directory").Required().String()
	appID := configureCmd.Flag("appID", "MS App ID").Required().String()
	appSecret := configureCmd.Flag("appSecret", "MS App Secret").Required().String()
	tenantID := configureCmd.Flag("tenantID", "MS App Tenant ID").Required().String()

	validateCmd := app.Command("validate", "Validate bot installation")
	validateConfigPath := validateCmd.Flag("config", "TOML config file path").
		Short('c').
		Default(configPath).
		String()

	validateUserID := validateCmd.Arg("userID", "Your User ID").Required().String()

	startCmd := app.Command("start", "Starts Teleport MS Teams plugin")
	startConfigPath := startCmd.Flag("config", "TOML config file path").
		Short('c').
		Default(configPath).
		String()
	debug := startCmd.Flag("debug", "Enable verbose logging to stderr").
		Short('d').
		Bool()

	selectedCmd, err := app.Parse(os.Args[1:])
	if err != nil {
		lib.Bail(err)
	}

	switch selectedCmd {
	case "configure":
		err := configure(*targetDir, *appID, *appSecret, *tenantID)
		if err != nil {
			lib.Bail(err)
		}

	case "validate":
		err := validate(*validateConfigPath, *validateUserID)
		if err != nil {
			lib.Bail(err)
		}

	case "version":
		lib.PrintVersion(app.Name, Version, Gitref)
	case "start":
		if err := run(*startConfigPath, *debug); err != nil {
			lib.Bail(err)
		} else {
			logger.Standard().Info("Successfully shut down")
		}
	}

}

func run(configPath string, debug bool) error {
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

	app, err := NewApp(*conf)
	if err != nil {
		return trace.Wrap(err)
	}

	go lib.ServeSignals(app, gracefulShutdownTimeout)

	return trace.Wrap(
		app.Run(context.Background()),
	)
}
