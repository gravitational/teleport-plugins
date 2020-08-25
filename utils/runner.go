// Contains common subcommands and CLI app management.

package utils

import (
	"fmt"
	"runtime"

	"github.com/gravitational/kingpin"
	log "github.com/sirupsen/logrus"
)

// Plugin struct contains the plugin name, and the CLI Kingpin
// wrapper around the plugin application.
type Plugin struct {
	name          string
	exampleConfig string
	version       string
	gitref        string
	app           *kingpin.Application
	path          string
	insecure      bool
	debug         bool
}

// AppRunner describes a function that takes
// configPath:string, insecure: bool, debug: bool
// and launches the plugin app.
//
// This function has to:
// 1. Load the config `LoadConfig()`
// 2. Instantiate `NewApp()` and run it.
//
// It's tied to the plugin implementation too much,
// so it's on the plugin designer to define it.
type AppRunner func(string, bool, bool) error

// NewPlugin initializes and returns a new Plugin wrapper struct.
func NewPlugin(name, exampleConfig, version, gitref string) *Plugin {
	// Create the CLI app
	app := kingpin.New(name, "Teleport plugin for access requst approval workflows.")

	// Commands
	app.Command("configure", "Prints an example .TOML configuration file.")
	app.Command("version", fmt.Sprintf("Prints %s version and exits.", name))
	startCmd := app.Command("start", fmt.Sprintf("Starts a %s plugin.", name))

	// Flags for the start command
	path := startCmd.Flag("config", "TOML config file path").
		Short('c').
		Default(fmt.Sprintf("/etc/%s.toml", name)).
		String()
	debug := startCmd.Flag("debug", "Enable verbose logging to stderr").
		Short('d').
		Bool()
	insecure := startCmd.Flag("insecure-no-tls", "Disable TLS for the callback server").
		Default("false").
		Bool()

	// Create the Plugin structure and return
	plugin := &Plugin{
		name:          name,
		exampleConfig: exampleConfig,
		version:       version,
		gitref:        gitref,
		app:           app,
		path:          *path,
		insecure:      *insecure,
		debug:         *debug,
	}

	return plugin
}

// ParseCommand parses the CLI command and starts the invoked command.
// ParseCommand expects command line arguments, and a plugin-specific
// implementation of `run` that takes config arguments and runs the plugin.
func (p *Plugin) ParseCommand(args []string, run AppRunner) {
	selectedCmd, err := p.app.Parse(args)
	if err != nil {
		Bail(err)
	}

	switch selectedCmd {
	case "configure":
		fmt.Print(p.exampleConfig)
	case "version":
		p.versionCmd()
	case "start":
		if err := run(p.path, p.insecure, p.debug); err != nil {
			Bail(err)
		} else {
			log.Infof("Successfully shut down %v", p.name)
		}
	}
}

// PrintVersion prints the specified app version to STDOUT
func (p *Plugin) versionCmd() {
	if p.gitref != "" {
		fmt.Printf("%v v%v git:%v %v\n", p.name, p.version, p.gitref, runtime.Version())
	} else {
		fmt.Printf("%v v%v %v\n", p.name, p.version, runtime.Version())
	}
}
