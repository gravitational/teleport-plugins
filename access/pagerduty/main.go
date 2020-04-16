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
	DefaultDir = "/var/lib/teleport/plugins/pagerduty"
)

type RequestData struct {
	User    string
	Roles   []string
	Created time.Time
}

type PagerdutyData struct {
	ID string
}

type PluginData struct {
	RequestData
	PagerdutyData
}

// App contains global application state.
type App struct {
	sync.Mutex

	conf Config

	accessClient access.Client
	bot          *Bot

	*utils.Process
}

type logFields = log.Fields

func main() {
	utils.InitLogger()
	app := kingpin.New("teleport-pagerduty", "Teleport plugin for access requests approval via PagerDuty.")

	app.Command("configure", "Prints an example .TOML configuration file.")

	startCmd := app.Command("start", "Starts a Teleport PagerDuty plugin.")
	path := startCmd.Flag("config", "TOML config file path").
		Short('c').
		Default("/etc/teleport-pagerduty.toml").
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

func NewApp(conf Config) (*App, error) {
	return &App{conf: conf}, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	var err error

	log.Infof("Starting Teleport Access PagerDuty extension %s:%s", Version, Gitref)

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

	a.bot, err = NewBot(&a.conf, a.OnPagerdutyAction)
	if err != nil {
		return trace.Wrap(err)
	}

	a.accessClient, err = access.NewClient(
		ctx,
		"pagerduty",
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
			log.Debug("Starting PagerDuty API health check...")
			if err := a.bot.HealthCheck(ctx); err != nil {
				log.WithError(err).Error("PagerDuty API health check failed")
				a.Terminate()
				return
			}
			log.Debug("PagerDuty API health check finished ok")

			log.Debug("Setting up the webhook extensions")
			if err := a.bot.Setup(ctx); err != nil {
				log.WithError(err).Error("Failed to set up webhook extensions")
				a.Terminate()
				return
			}
			log.Debug("PagerDuty webhook extensions setup finished ok")
		})

		a.Spawn(func(ctx context.Context) {
			defer a.Terminate() // if webhook server failed, shutdown everything

			a.OnTerminate(func(ctx context.Context) {
				if err := a.bot.ShutdownServer(ctx); err != nil {
					log.WithError(err).Error("HTTP server graceful shutdown failed")
				}
			})

			// Run a webhook server
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

	for {
		err := a.watchRequests(ctx)

		switch {
		case trace.IsConnectionProblem(err):
			log.WithError(err).Error("Failed to connect to Teleport Auth server. Reconnecting...")
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

func (a *App) OnPagerdutyAction(ctx context.Context, action WebhookAction) error {
	log := log.WithFields(logFields{
		"pd_http_id": action.HttpID,
		"pd_msg_id":  action.MessageID,
	})

	if action.Event != "incident.custom" {
		log.Debugf("Got %q event, ignoring", action.Event)
		return nil
	}

	keyParts := strings.Split(action.IncidentKey, "/")
	if len(keyParts) != 2 || keyParts[0] != pdIncidentKeyPrefix {
		log.Debugf("Got unsupported incident key %q, ignoring", action.IncidentKey)
		return nil
	}

	reqID := keyParts[1]
	req, err := a.accessClient.GetRequest(ctx, reqID)

	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).WithField("request_id", reqID).Warning("Cannot process expired request")
			return nil
		} else {
			return trace.Wrap(err)
		}
	} else {
		if req.State != access.StatePending {
			return trace.Errorf("cannot process not pending request: %+v", req)
		}

		pluginData, err := a.getPluginData(ctx, reqID)
		if err != nil {
			return trace.Wrap(err)
		}
		if pluginData.PagerdutyData.ID != action.IncidentID {
			log.WithFields(logFields{
				"plugin_data_incident_id": pluginData.PagerdutyData.ID,
				"incident_id":             action.IncidentID,
			}).Debug("plugin_data.incident_id does not match incident.id")
			return trace.Errorf("incident_id from request's plugin_data does not match")
		}

		log = log.WithField("pd_incident_id", action.IncidentID)

		var (
			reqState   access.State
			resolution string
		)

		switch action.Name {
		case pdApproveAction:
			reqState = access.StateApproved
			resolution = "approved"
		case pdDenyAction:
			reqState = access.StateDenied
			resolution = "denied"
		default:
			return trace.BadParameter("unknown action: %q", action.Name)
		}

		if err := a.accessClient.SetRequestState(ctx, req.ID, reqState); err != nil {
			return trace.Wrap(err)
		}
		log.Infof("PagerDuty user %s the request", resolution)

		if err := a.bot.ResolveIncident(ctx, reqID, pluginData.PagerdutyData, resolution); err != nil {
			return trace.Wrap(err)
		}
		log.Infof("Incident %q has been resolved", action.IncidentID)
	}

	return nil
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := RequestData{User: req.User, Roles: req.Roles, Created: req.Created}

	pdData, err := a.bot.CreateIncident(ctx, req.ID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithFields(logFields{
		"incident_id": pdData.ID,
	}).Info("PagerDuty incident created")

	err = a.setPluginData(ctx, req.ID, PluginData{reqData, pdData})

	return trace.Wrap(err)
}

func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
	reqID := req.ID // This is the only available field
	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warn("Cannot expire unknown request")
			return nil
		} else {
			return trace.Wrap(err)
		}
	}

	if err := a.bot.ResolveIncident(ctx, reqID, pluginData.PagerdutyData, "expired"); err != nil {
		return trace.Wrap(err)
	}

	log.WithField("request_id", reqID).Info("Successfully marked request as expired")

	return nil
}

func (a *App) getPluginData(ctx context.Context, reqID string) (data PluginData, err error) {
	dataMap, err := a.accessClient.GetPluginData(ctx, reqID)
	if err != nil {
		return data, trace.Wrap(err)
	}
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	var created int64
	fmt.Sscanf(dataMap["created"], "%d", &created)
	data.Created = time.Unix(created, 0)
	data.ID = dataMap["incident_id"]
	return
}

func (a *App) setPluginData(ctx context.Context, reqID string, data PluginData) error {
	return a.accessClient.UpdatePluginData(ctx, reqID, access.PluginData{
		"incident_id": data.ID,
		"user":        data.User,
		"roles":       strings.Join(data.Roles, ","),
		"created":     fmt.Sprintf("%d", data.Created.Unix()),
	}, nil)
}
