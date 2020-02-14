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
	"crypto/tls"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/utils"

	"github.com/gravitational/kingpin"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

const (
	// ActionApprove uniquely identifies the approve button in events.
	ActionApprove = "approve_request"
	// ActionDeny uniquely identifies the deny button in events.
	ActionDeny = "deny_request"

	DefaultDir = "/var/lib/teleport/plugins/jirabot"
)

type requestData struct {
	user  string
	roles []string
}

type jiraData struct {
	ID  string
	Key string
}

type pluginData struct {
	requestData
	jiraData
}

// eprintln prints an optionally formatted string to stderr.
func eprintln(msg string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, a...)
	fmt.Fprintf(os.Stderr, "\n")
}

// bail exits with nonzero exit code and optionally formatted message.
func bail(msg string, a ...interface{}) {
	eprintln(msg, a...)
	os.Exit(1)
}

func main() {
	utils.InitLogger()
	app := kingpin.New("teleport-jirabot", "Teleport plugin for access requests approval via JIRA.")

	app.Command("configure", "Prints an example .TOML configuration file.")

	startCmd := app.Command("start", "Starts a Teleport JIRA plugin.")
	path := startCmd.Flag("config", "TOML config file path").
		Short('c').
		Default("/etc/teleport-jirabot.toml").
		String()
	debug := startCmd.Flag("debug", "Enable verbose logging to stderr").
		Short('d').
		Bool()
	insecure := startCmd.Flag("insecure-no-tls", "Disable TLS for the callback server").
		Default("false").
		Bool()

	selectedCmd, err := app.Parse(os.Args[1:])
	if err != nil {
		bail("error: %s", err)
	}

	switch selectedCmd {
	case "configure":
		fmt.Print(exampleConfig)
	case "start":
		if err := run(*path, *insecure, *debug); err != nil {
			bail("error: %s", trace.DebugReport(err))
		}
	}
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

	serveSignals(app)

	return trace.Wrap(
		app.Run(context.Background()),
	)
}

func serveSignals(app *App) {
	sigC := make(chan os.Signal)
	signal.Notify(sigC,
		syscall.SIGQUIT, // graceful shutdown
		syscall.SIGTERM, // fast shutdown
		syscall.SIGINT,  // graceful-then-fast shutdown
	)
	var alreadyInterrupted bool
	go func() {
		for {
			signal := <-sigC
			switch signal {
			case syscall.SIGQUIT:
				app.Stop()
			case syscall.SIGTERM, syscall.SIGKILL:
				app.Close()
			case syscall.SIGINT:
				if alreadyInterrupted {
					app.Close()
				} else {
					app.Stop()
					alreadyInterrupted = true
				}
			}
		}
	}()
}

type killSwitch func()

// App contains global application state.
type App struct {
	sync.Mutex

	conf Config

	accessClient  access.Client
	bot           *Bot
	webhookServer *WebhookServer

	stop   killSwitch
	cancel context.CancelFunc
}

func NewApp(conf Config) (*App, error) {
	return &App{conf: conf}, nil
}

func (a *App) finish() {
	a.accessClient = nil
	a.bot = nil
	a.webhookServer = nil

	a.stop = nil
	a.cancel = nil
}

// Close signals the App to shutdown immediately
func (a *App) Close() {
	a.Lock()
	defer a.Unlock()

	if a.cancel != nil {
		log.Infof("Force shutdown...")
		a.cancel()
	}
}

// Stop signals the App to shutdown gracefully
func (a *App) Stop() {
	a.Lock()
	defer a.Unlock()

	if a.stop != nil {
		a.stop()
	}
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	a.Lock()
	defer a.Unlock()

	defer a.finish()

	var err error

	log.Infof("Starting Teleport Access JIRAbot %s:%s", Version, Gitref)

	a.bot, err = NewBot(&a.conf)
	if err != nil {
		return trace.Wrap(err)
	}

	clientCert, err := access.LoadX509Cert(a.conf.Teleport.ClientCrt, a.conf.Teleport.ClientKey)
	if err != nil {
		return trace.Wrap(err)
	}
	now := time.Now()
	if now.After(clientCert.Leaf.NotAfter) {
		log.Error("Auth client TLS certificate seems to be expired, you should re-new it.")
	}
	if now.Before(clientCert.Leaf.NotBefore) {
		log.Error("Auth client TLS certificate seems to be invalid, check its notBefore date.")
	}
	caPool, err := access.LoadX509CertPool(a.conf.Teleport.RootCAs)
	if err != nil {
		return trace.Wrap(err)
	}

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	a.accessClient, err = access.NewClient(
		ctx,
		"jirabot",
		a.conf.Teleport.AuthServer,
		&tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      caPool,
		},
	)
	if err != nil {
		cancel()
		return trace.Wrap(err)
	}

	err = a.checkTeleportVersion(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	// Create callback server prividing a.OnJIRAWebhook as a callback function
	a.webhookServer = NewWebhookServer(&a.conf, a.OnJIRAWebhook)

	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			defer cancel()

			log.Infof("Attempting graceful shutdown...")

			if err := a.webhookServer.Shutdown(ctx); err != nil {
				log.WithError(err).Error("HTTP server graceful shutdown failed")
			}
		})
	}
	a.stop = func() { go shutdown() }

	var (
		httpErr, watcherErr error
		workers             sync.WaitGroup
	)

	workers.Add(2)

	go func() {
		defer workers.Done()
		defer shutdown()

		httpErr = trace.Wrap(
			a.webhookServer.Run(ctx),
		)
	}()

	go func() {
		defer workers.Done()
		defer shutdown()

		watcherErr = trace.Wrap(
			// Start watching for request events
			a.WatchRequests(ctx),
		)
	}()

	// Unlock the app, wait for workers, and lock again
	a.Unlock()
	workers.Wait()
	a.Lock()

	return trace.NewAggregate(httpErr, watcherErr)
}

func (a *App) checkTeleportVersion(ctx context.Context) error {
	log.Info("Checking Teleport server version")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pong, err := a.accessClient.Ping(ctx)
	if err != nil {
		log.Error("Unable to get Teleport server version")
		return trace.Wrap(err)
	}
	a.bot.clusterName = pong.ClusterName
	err = pong.AssertServerVersion()
	return trace.Wrap(err)
}

func (a *App) watchRequests(ctx context.Context) error {
	watcher := a.accessClient.WatchRequests(ctx, access.Filter{
		State: access.StatePending,
	})
	defer watcher.Close()

	if err := watcher.WaitInit(ctx, 5*time.Second); err != nil {
		return trace.Wrap(err)
	}

	log.Info("Watcher connected")

	for {
		select {
		case event := <-watcher.Events():
			req, op := event.Request, event.Type
			switch op {
			case access.OpPut:
				if !req.State.IsPending() {
					log.WithField("event", event).Warn("non-pending request event")
					continue
				}

				if err := a.onPendingRequest(ctx, req); err != nil {
					return trace.Wrap(err)
				}
			case access.OpDelete:
				if err := a.onDeletedRequest(ctx, req); err != nil {
					return trace.Wrap(err)
				}
			default:
				return trace.BadParameter("unexpected event operation %s", op)
			}
		case <-watcher.Done():
			return trace.Wrap(watcher.Error())
		}
	}
}

// WatchRequests starts a GRPC watcher which monitors access requests and restarts it on expected errors.
// It calls onPendingRequest when new pending event is added and onDeletedRequest when request is deleted
func (a *App) WatchRequests(ctx context.Context) error {
	log.Info("Starting a request watcher...")

	for {
		err := a.watchRequests(ctx)

		switch {
		case trace.IsConnectionProblem(err):
			log.WithError(err).Error("Cannot connect to Teleport Auth server. Reconnecting...")
		case trace.IsEOF(err):
			log.WithError(err).Error("Watcher stream closed. Reconnecting...")
		case utils.IsCanceled(err):
			// Context cancellation is not an error
			return nil
		default:
			return trace.Wrap(err)
		}
	}
}

// OnJIRAWebhook processes JIRA webhook and updates the status of an issue
func (a *App) OnJIRAWebhook(ctx context.Context, webhook Webhook) error {
	if webhook.WebhookEvent != "jira:issue_updated" {
		return nil
	}
	if webhook.IssueEventTypeName != "issue_updated" {
		return nil
	}

	if webhook.Issue == nil {
		return trace.Errorf("got webhook without issue info")
	}

	issueId := webhook.Issue.ID

	issue, reqID, err := a.bot.GetIssue(issueId)
	if err != nil {
		return trace.Wrap(err)
	}

	req, err := a.accessClient.GetRequest(ctx, reqID)

	if err != nil {
		if trace.IsNotFound(err) {
			// TODO: what to do with expired issues? nothing?...
			return nil
		} else {
			return trace.Wrap(err)
		}
	} else {
		if req.State != access.StatePending {
			return trace.Errorf("Can't process not pending request: %+v", req)
		}

		logFields := log.Fields{
			"issue_id":   issueId,
			"request_id": req.ID,
		}

		var reqState access.State
		statusName := strings.ToLower(issue.Fields.Status.Name)

		switch statusName {
		case "approved":
			log.WithFields(logFields).Info("JIRA user approved the request")
			reqState = access.StateApproved
		case "denied":
			log.WithFields(logFields).Info("JIRA user denied the request")
			reqState = access.StateDenied
		default:
			return trace.BadParameter("Unknown JIRA status: %s", statusName)
		}

		if err := a.accessClient.SetRequestState(ctx, req.ID, reqState); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := requestData{user: req.User, roles: req.Roles}
	jiraData, err := a.bot.CreateIssue(req.ID, reqData)

	if err != nil {
		return trace.Wrap(err)
	}

	log.WithFields(log.Fields{
		"issue_id":  jiraData.ID,
		"issue_key": jiraData.Key,
	}).Debug("JIRA Issue created")

	err = a.setPluginData(ctx, req.ID, pluginData{reqData, jiraData})

	return trace.Wrap(err)
}

func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
	reqID := req.ID // This is the only available field
	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warnf("cannot expire unknown request")
			return nil
		} else {
			return trace.Wrap(err)
		}
	}

	if err := a.bot.ExpireIssue(reqID, pluginData.requestData, pluginData.jiraData); err != nil {
		return trace.Wrap(err)
	}

	log.WithField("request_id", reqID).Debug("Successfully marked request as expired")

	return nil
}

func (a *App) getPluginData(ctx context.Context, reqID string) (data pluginData, err error) {
	_, err = a.accessClient.GetPluginData(ctx, reqID)
	if err != nil {
		return data, trace.Wrap(err)
	}
	// TODO: set data fields
	return
}

func (a *App) setPluginData(ctx context.Context, reqID string, data pluginData) error {
	return a.accessClient.UpdatePluginData(ctx, reqID, access.PluginData{
		"issue_id":  data.ID,
		"issue_key": data.Key,
		"user":      data.user,
		"roles":     strings.Join(data.roles, ","),
	}, nil)
}
