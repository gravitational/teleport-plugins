// Contains common subcommands and CLI app management.

package utils

import (
	"fmt"
	"runtime"

	"github.com/gravitational/kingpin"
	log "github.com/sirupsen/logrus"
)

// PrintVersion prints the specified app version to STDOUT
func PrintVersion(appName string, version string, gitref string) {
	if gitref != "" {
		fmt.Printf("%v v%v git:%v %v\n", appName, version, gitref, runtime.Version())
	} else {
		fmt.Printf("%v v%v %v\n", appName, version, runtime.Version())
	}
}

// PluginApp struct contains the plugin name, and the CLI Kingpin
// wrapper around the plugin application.
type PluginApp struct {
	Name          string
	ExampleConfig string
	Version       string
	Gitref        string
	KingpinApp    *kingpin.Application
	Path          string
	Insecure      bool
	Debug         bool
}

// TODO: Maybe extract app runner arguments into a separate struct
// that can be defined by the client application / plugin.
// Build that struct from command line parameters, and then pass that struct
// to the handler function in ParseCommand.
// That way, the util will be more general and flexible.
type AppRunnerFunc func(string, bool, bool) error

// Initializes and returns a new instance of
// PluginApp.
func NewPluginApp(name, exampleConfig, version, gitref string) *PluginApp {
	app := kingpin.New(name, "Teleport plugin for access requst approval workflows.")

	app.Command("configure", "Prints an example .TOML configuration file.")
	app.Command("version", fmt.Sprintf("Prints %s version and exits.", name))

	startCmd := app.Command("start", fmt.Sprintf("Starts a %s plugin.", name))
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

	plugin := &PluginApp{
		Name:          name,
		ExampleConfig: exampleConfig,
		Version:       version,
		Gitref:        gitref,
		KingpinApp:    app,
		Path:          *path,
		Insecure:      *insecure,
		Debug:         *debug,
	}

	return plugin
}

// Parses the CLI command and starts the required command.
func (p *PluginApp) ParseCommand(args []string, run AppRunnerFunc) {
	selectedCmd, err := p.KingpinApp.Parse(args)
	if err != nil {
		Bail(err)
	}

	// TODO: This assumes three specific commands, while the client
	// app might want to define more.
	// We can make this more generic by storing commangs in an
	// map, and switching it's keys here.
	//
	// Additionally, we can provide common command handlers
	// in the util, but let the client app define the mapping
	// between the commands and the specific handler funcs that
	// will be invoked to handle the commands.
	switch selectedCmd {
	case "configure":
		fmt.Print(p.ExampleConfig)
	case "version":
		PrintVersion(p.Name, p.Version, p.Gitref)
	case "start":
		if err := run(p.Path, p.Insecure, p.Debug); err != nil {
			Bail(err)
		} else {
			log.Info("Successfully shut down")
		}
	}
}
