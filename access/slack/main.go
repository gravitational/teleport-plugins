/*
Copyright 2019 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"

	"github.com/gravitational/kingpin"
	"github.com/gravitational/trace"
)

func main() {
	logger.Init()
	app := kingpin.New("teleport-slack", "Teleport plugin for access requests approval via Slack.")

	app.Command("configure", "Prints an example .TOML configuration file.")
	app.Command("version", "Prints teleport-slack version and exits.")

	startCmd := app.Command("start", "Starts a the Teleport Slack plugin.")
	path := startCmd.Flag("config", "TOML config file path").
		Short('c').
		Default("/etc/teleport-slack.toml").
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
		fmt.Print(exampleConfig)
	case "version":
		lib.PrintVersion(app.Name, Version, Gitref)
	case "start":
		if err := run(*path, *debug); err != nil {
			lib.Bail(err)
		} else {
			logger.Standard().Info("Successfully shut down")
		}
	}
}

// a very medicore way of handling pre-aproval
var preapprovedList = []string{}

func run(configPath string, debug bool) error {
	if os.Getenv("RESTRICTED_ENV") == "" {
		return errors.New("refusing to run without a restricted environment (RESTRICTED_ENV)")
	}

	if os.Getenv("PREAPPROVED_LIST") == "" {
		return errors.New("refusing to run with an empty preapproval list (PREAPPROVED_LIST)")
	}

	preapprovedList = strings.Split(os.Getenv("PREAPPROVED_LIST"), ",")
	for _, email := range preapprovedList {
		if !strings.Contains(email, "@") {
			return fmt.Errorf("Item in preapprove was not a valid email: '%s'", email)
		}
	}

	fmt.Println("Preapproved list:")
	i := 0
	for _, email := range preapprovedList {
		i++
		if i%4 == 0 {
			fmt.Println()
		}
		fmt.Printf("%-40s ", email)
	}
	fmt.Println()

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

	http.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*15)
		defer cancel()
		_, ping := app.apiClient.Ping(ctx)
		check := app.bot.HealthCheck(ctx)

		if ping != nil || check != nil {
			rw.WriteHeader(http.StatusInternalServerError)
		} else {
			rw.WriteHeader(http.StatusOK)
		}
		rw.Header().Add("content-type", "text/plain")
		fmt.Fprintf(rw, "ping err=%s; ", ping)
		fmt.Fprintf(rw, "check err=%s; ", check)
	})
	diagAddr := os.Getenv("DIAG_ADDR")
	if diagAddr == "" {
		diagAddr = "localhost:9000"
	}
	go http.ListenAndServe(diagAddr, nil)

	go lib.ServeSignals(app, 15*time.Second)

	return trace.Wrap(
		app.Run(context.Background()),
	)
}
