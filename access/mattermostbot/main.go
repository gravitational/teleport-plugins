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
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/utils"

	"github.com/gravitational/kingpin"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

const (
	DefaultDir = "/var/lib/teleport/plugins/mattermostbot"
)

type RequestData struct {
	User  string
	Roles []string
}

type MattermostData struct {
	PostID    string
	ChannelID string
}

type pluginData struct {
	RequestData
	MattermostData
}

type logFields = log.Fields

func main() {
	utils.InitLogger()
	app := kingpin.New("teleport-mattermostbot", "Teleport plugin for access requests approval via Mattermost.")

	app.Command("configure", "Prints an example .TOML configuration file.")

	startCmd := app.Command("start", "Starts a Teleport Mattermost plugin.")
	path := startCmd.Flag("config", "TOML config file path").
		Short('c').
		Default("/etc/teleport-mattermostbot.toml").
		String()
	debug := startCmd.Flag("debug", "Enable verbose logging to stderr").
		Short('d').
		Bool()
	insecure := startCmd.Flag("insecure-no-tls", "Disable TLS for the callback server").
		Default("false").
		Bool()

	selectedCmd, err := app.Parse(os.Args[1:])
	if err != nil {
		utils.Bail(err)
	}

	switch selectedCmd {
	case "configure":
		fmt.Print(exampleConfig)
	case "start":
		if err := run(*path, *insecure, *debug); err != nil {
			utils.Bail(err)
		} else {
			log.Info("Successfully shut down")
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

	go utils.ServeSignals(app, 15*time.Second)

	return trace.Wrap(
		app.Run(context.Background()),
	)
}

// App contains global application state.
type App struct {
	sync.Mutex

	conf Config

	accessClient access.Client
	bot          *Bot

	*utils.Process
}

func NewApp(conf Config) (*App, error) {
	return &App{conf: conf}, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	var err error

	log.Infof("Starting Teleport Access Mattermost Bot %s:%s", Version, Gitref)

	tlsConf, err := access.LoadTLSConfig(
		a.conf.Teleport.ClientCrt,
		a.conf.Teleport.ClientKey,
		a.conf.Teleport.RootCAs,
	)
	if trace.Unwrap(err) == access.ErrInvalidCertificate {
		log.WithError(err).Warning("Auth client TLS configuration error")
	} else if err != nil {
		return err
	}

	a.Lock()
	defer a.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.bot, err = NewBot(&a.conf, a.OnMattermostAction)
	if err != nil {
		return trace.Wrap(err)
	}

	a.accessClient, err = access.NewClient(
		ctx,
		"mattermostbot",
		a.conf.Teleport.AuthServer,
		tlsConf,
	)
	if err != nil {
		return trace.Wrap(err)
	}

	// Initialize the process.
	a.Process = utils.NewProcess(ctx)

	var versionErr, httpErr, watcherErr error

	a.Spawn(func(ctx context.Context) {
		if versionErr = trace.Wrap(a.checkTeleportVersion(ctx)); versionErr != nil {
			a.Terminate()
			return
		}

		a.Spawn(func(ctx context.Context) {
			defer a.Terminate() // if bot server failed, shutdown everything

			a.OnTerminate(func(ctx context.Context) {
				if err := a.bot.ShutdownServer(ctx); err != nil {
					log.WithError(err).Error("HTTP server graceful shutdown failed")
				}
			})

			// Run a bot server
			err := a.bot.RunServer(ctx)
			httpErr = trace.Wrap(err)
		})

		a.Spawn(func(ctx context.Context) {
			defer a.Terminate() // if watcher failed, shutdown everything

			// Create separate context for Watcher in order to terminate it individually.
			wctx, wcancel := context.WithCancel(ctx)

			a.OnTerminate(func(_ context.Context) { wcancel() })

			watcherErr = trace.Wrap(
				// Start watching for request events
				a.WatchRequests(wctx),
			)
		})
	})

	// Unlock the app, wait for jobs, and lock again
	process := a.Process // Save variable to access when unlocked
	a.Unlock()
	<-process.Done()
	a.Lock()

	return trace.NewAggregate(versionErr, httpErr, watcherErr)
}

func (a *App) checkTeleportVersion(ctx context.Context) error {
	log.Debug("Checking Teleport server version")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pong, err := a.accessClient.Ping(ctx)
	if err != nil {
		if trace.IsNotImplemented(err) {
			return trace.Wrap(err, "server version must be at least %s", access.MinServerVersion)
		}
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

	log.Debug("Watcher connected")

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

				a.Spawn(func(ctx context.Context) {
					if err := a.onPendingRequest(ctx, req); err != nil {
						log.WithError(err).WithField("request_id", req.ID).Errorf("Failed to process pending request")
					}
				})
			case access.OpDelete:
				a.Spawn(func(ctx context.Context) {
					if err := a.onDeletedRequest(ctx, req); err != nil {
						log.WithError(err).WithField("request_id", req.ID).Errorf("Failed to process deleted request")
					}
				})
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
	log.Debug("Starting a request watcher...")
	defer log.Debug("Request watcher terminated")

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

func (a *App) OnMattermostAction(ctx context.Context, data BotAction) (BotActionResponse, error) {
	log := log.WithField("mm_http_id", data.HttpRequestID)

	action := data.Action
	reqID := data.ReqID

	var mmStatus string

	req, err := a.accessClient.GetRequest(ctx, reqID)
	var reqData RequestData

	if err != nil {
		if trace.IsNotFound(err) {
			// Request wasn't found, need to expire it's post in Mattermost
			mmStatus = "EXPIRED"

			// And try to fetch its request data if it exists
			var pluginData pluginData
			pluginData, _ = a.getPluginData(ctx, reqID)
			reqData = pluginData.RequestData
		} else {
			return BotActionResponse{}, trace.Wrap(err)
		}
	} else {
		if req.State != access.StatePending {
			return BotActionResponse{}, trace.Errorf("can't process not pending request: %+v", req)
		}

		log = log.WithFields(logFields{
			"mm_channel_id": data.ChannelID,
			"mm_post_id":    data.PostID,
			"mm_user_id":    data.UserID,
		})
		user, err := a.bot.GetUser(ctx, data.UserID)
		if err != nil {
			log.WithError(err).Warning("Cannot get user info")
		}
		log = log.WithFields(logFields{
			"mm_user_name":  user.Username,
			"mm_user_email": user.Email,
		})

		var (
			reqState   access.State
			resolution string
		)

		switch action {
		case "approve":
			reqState = access.StateApproved
			mmStatus = "APPROVED"
			resolution = "approved"
		case "deny":
			reqState = access.StateDenied
			mmStatus = "DENIED"
			resolution = "denied"
		default:
			return BotActionResponse{}, trace.BadParameter("Unknown Action: %s", action)
		}

		if err := a.accessClient.SetRequestState(ctx, req.ID, reqState); err != nil {
			return BotActionResponse{}, trace.Wrap(err)
		}
		log.Infof("Mattermost user %s the request", resolution)

		// Simply fill reqData from the request itself.
		reqData = RequestData{User: req.User, Roles: req.Roles}
	}

	return BotActionResponse{
		ReqData: reqData,
		Status:  mmStatus,
	}, nil
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := RequestData{User: req.User, Roles: req.Roles}
	mmData, err := a.bot.CreatePost(ctx, req.ID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithField("mattermost_post_id", mmData.PostID).Info("Successfully posted to Mattermost")

	err = a.setPluginData(ctx, req.ID, pluginData{reqData, mmData})
	return trace.Wrap(err)
}

func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
	reqID := req.ID // This is the only available field
	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		return trace.Wrap(err)
	}

	reqData, mmData := pluginData.RequestData, pluginData.MattermostData
	if mmData.PostID == "" {
		return trace.NotFound("plugin data was expired")
	}

	if err := a.bot.ExpirePost(ctx, reqID, reqData, mmData); err != nil {
		return trace.Wrap(err)
	}

	log.WithField("request_id", reqID).Info("Successfully marked request as expired")

	return nil
}

func (a *App) getPluginData(ctx context.Context, reqID string) (data pluginData, err error) {
	dataMap, err := a.accessClient.GetPluginData(ctx, reqID)
	if err != nil {
		return data, trace.Wrap(err)
	}
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	data.PostID = dataMap["post_id"]
	data.PostID = dataMap["channel_id"]
	return
}

func (a *App) setPluginData(ctx context.Context, reqID string, data pluginData) error {
	return a.accessClient.UpdatePluginData(ctx, reqID, access.PluginData{
		"user":       data.User,
		"roles":      strings.Join(data.Roles, ","),
		"post_id":    data.PostID,
		"channel_id": data.ChannelID,
	}, nil)
}
