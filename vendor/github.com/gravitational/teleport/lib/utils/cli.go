/*
Copyright 2016-2019 Gravitational, Inc.

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

package utils

import (
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/gravitational/teleport"

	"github.com/gravitational/kingpin"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

type LoggingPurpose int

const (
	LoggingForDaemon LoggingPurpose = iota
	LoggingForCLI
	LoggingForTests
)

// InitLogger configures the global logger for a given purpose / verbosity level
func InitLogger(purpose LoggingPurpose, level log.Level, verbose ...bool) {
	log.StandardLogger().SetHooks(make(log.LevelHooks))
	log.SetLevel(level)

	switch purpose {
	case LoggingForCLI:
		// If debug logging was asked for on the CLI, then write logs to stderr.
		// Otherwise discard all logs.
		if level == log.DebugLevel {
			log.SetFormatter(&trace.TextFormatter{
				DisableTimestamp: true,
				EnableColors:     trace.IsTerminal(os.Stderr),
			})
			log.SetOutput(os.Stderr)
		} else {
			log.SetOutput(ioutil.Discard)
		}
	case LoggingForDaemon:
		log.SetFormatter(&trace.TextFormatter{
			DisableTimestamp: true,
			EnableColors:     trace.IsTerminal(os.Stderr),
		})
		log.SetOutput(os.Stderr)
	case LoggingForTests:
		log.SetFormatter(&trace.TextFormatter{
			DisableTimestamp: true,
			EnableColors:     true,
		})
		log.SetLevel(level)
		log.SetOutput(os.Stderr)
		if len(verbose) != 0 && verbose[0] {
			return
		}
		val, _ := strconv.ParseBool(os.Getenv(teleport.VerboseLogsEnvVar))
		if val {
			return
		}
		val, _ = strconv.ParseBool(os.Getenv(teleport.DebugEnvVar))
		if val {
			return
		}
		log.SetLevel(log.WarnLevel)
		log.SetOutput(ioutil.Discard)
	}
}

func InitLoggerForTests(verbose ...bool) {
	InitLogger(LoggingForTests, log.DebugLevel, verbose...)
}

// FatalError is for CLI front-ends: it detects gravitational/trace debugging
// information, sends it to the logger, strips it off and prints a clean message to stderr
func FatalError(err error) {
	fmt.Fprintln(os.Stderr, UserMessageFromError(err))
	os.Exit(1)
}

// GetIterations provides a simple way to add iterations to the test
// by setting environment variable "ITERATIONS", by default it returns 1
func GetIterations() int {
	out := os.Getenv(teleport.IterationsEnvVar)
	if out == "" {
		return 1
	}
	iter, err := strconv.Atoi(out)
	if err != nil {
		panic(err)
	}
	log.Debugf("Starting tests with %v iterations.", iter)
	return iter
}

// UserMessageFromError returns user friendly error message from error
func UserMessageFromError(err error) string {
	// untrusted cert?
	switch innerError := trace.Unwrap(err).(type) {
	case x509.HostnameError:
		return fmt.Sprintf("Cannot establish https connection to %s:\n%s\n%s\n",
			innerError.Host,
			innerError.Error(),
			"try a different hostname for --proxy or specify --insecure flag if you know what you're doing.")
	case x509.UnknownAuthorityError:
		return `WARNING:

  The proxy you are connecting to has presented a certificate signed by a
  unknown authority. This is most likely due to either being presented
  with a self-signed certificate or the certificate was truly signed by an
  authority not known to the client.

  If you know the certificate is self-signed and would like to ignore this
  error use the --insecure flag.

  If you have your own certificate authority that you would like to use to
  validate the certificate chain presented by the proxy, set the
  SSL_CERT_FILE and SSL_CERT_DIR environment variables respectively and try
  again.

  If you think something malicious may be occurring, contact your Teleport
  system administrator to resolve this issue.
`
	case x509.CertificateInvalidError:
		return fmt.Sprintf(`WARNING:

  The certificate presented by the proxy is invalid: %v.

  Contact your Teleport system administrator to resolve this issue.`, innerError)
	}
	if log.GetLevel() == log.DebugLevel {
		return trace.DebugReport(err)
	}
	if err != nil {
		// If the error is a trace error, check if it has a user message embedded in
		// it, if it does, print it, otherwise escape and print the original error.
		if er, ok := err.(*trace.TraceErr); ok {
			if er.Message != "" {
				return fmt.Sprintf("error: %v", EscapeControl(er.Message))
			}
		}
		return fmt.Sprintf("error: %v", EscapeControl(err.Error()))
	}
	return ""
}

// Consolef prints the same message to a 'ui console' (if defined) and also to
// the logger with INFO priority
func Consolef(w io.Writer, component string, msg string, params ...interface{}) {
	entry := log.WithFields(log.Fields{
		trace.Component: component,
	})
	msg = fmt.Sprintf(msg, params...)
	entry.Info(msg)
	if w != nil {
		component := strings.ToUpper(component)
		// 13 is the length of "[KUBERNETES]", which is the longest component
		// name prefix we have *today*. Use a Max function here to avoid
		// negative spacing, in case we add longer component names.
		spacing := int(math.Max(float64(12-len(component)), 0))
		fmt.Fprintf(w, "[%v]%v %v\n", strings.ToUpper(component), strings.Repeat(" ", spacing), msg)
	}
}

// InitCLIParser configures kingpin command line args parser with
// some defaults common for all Teleport CLI tools
func InitCLIParser(appName, appHelp string) (app *kingpin.Application) {
	app = kingpin.New(appName, appHelp)

	// hide "--help" flag
	app.HelpFlag.Hidden()
	app.HelpFlag.NoEnvar()

	// set our own help template
	return app.UsageTemplate(defaultUsageTemplate)
}

// EscapeControl escapes all ANSI escape sequences from string and returns a
// string that is safe to print on the CLI. This is to ensure that malicious
// servers can not hide output. For more details, see:
//   * https://sintonen.fi/advisories/scp-client-multiple-vulnerabilities.txt
func EscapeControl(s string) string {
	if needsQuoting(s) {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// needsQuoting returns true if any non-printable characters are found.
func needsQuoting(text string) bool {
	for _, r := range text {
		if !strconv.IsPrint(r) {
			return true
		}
	}
	return false
}

// Usage template with compactly formatted commands.
var defaultUsageTemplate = `{{define "FormatCommand"}}\
{{if .FlagSummary}} {{.FlagSummary}}{{end}}\
{{range .Args}} {{if not .Required}}[{{end}}<{{.Name}}>{{if .Value|IsCumulative}}...{{end}}{{if not .Required}}]{{end}}{{end}}\
{{end}}\

{{define "FormatCommands"}}\
{{range .FlattenedCommands}}\
{{if not .Hidden}}\
  {{.FullCommand | printf "%-12s" }}{{if .Default}} (Default){{end}} {{ .Help }}
{{end}}\
{{end}}\
{{end}}\

{{define "FormatUsage"}}\
{{template "FormatCommand" .}}{{if .Commands}} <command> [<args> ...]{{end}}
{{if .Help}}
{{.Help|Wrap 0}}\
{{end}}\

{{end}}\

{{if .Context.SelectedCommand}}\
usage: {{.App.Name}} {{.Context.SelectedCommand}}{{template "FormatUsage" .Context.SelectedCommand}}
{{else}}\
Usage: {{.App.Name}}{{template "FormatUsage" .App}}
{{end}}\
{{if .Context.Flags}}\
Flags:
{{.Context.Flags|FlagsToTwoColumnsCompact|FormatTwoColumns}}
{{end}}\
{{if .Context.Args}}\
Args:
{{.Context.Args|ArgsToTwoColumns|FormatTwoColumns}}
{{end}}\
{{if .Context.SelectedCommand}}\

{{ if .Context.SelectedCommand.Commands}}\
Commands:
{{if .Context.SelectedCommand.Commands}}\
{{template "FormatCommands" .Context.SelectedCommand}}
{{end}}\
{{end}}\

{{else if .App.Commands}}\
Commands:
{{template "FormatCommands" .App}}
Try '{{.App.Name}} help [command]' to get help for a given command.
{{end}}\

{{ if .Context.SelectedCommand }}\
Aliases:
{{ range .Context.SelectedCommand.Aliases}}\
{{ . }}
{{end}}\
{{end}}
`
