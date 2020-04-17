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
	DefaultDir = "/var/lib/teleport/plugins/gitlab"
)

type RequestData struct {
	User    string
	Roles   []string
	Created time.Time
}

type GitlabData struct {
	ID        IntID
	IID       IntID
	ProjectID IntID
}

type PluginData struct {
	RequestData
	GitlabData
}

// App contains global application state.
type App struct {
	sync.Mutex

	conf Config

	db           DB
	accessClient access.Client
	bot          *Bot

	*utils.Process
}

type logFields = log.Fields

func main() {
	utils.InitLogger()
	app := kingpin.New("teleport-gitlab", "Teleport plugin for access requests approval via GitLab.")

	app.Command("configure", "Prints an example .TOML configuration file.")

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

	log.Infof("Starting Teleport Access GitLab integration %s:%s", Version, Gitref)

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

	a.bot, err = NewBot(&a.conf, a.OnWebhookEvent)
	if err != nil {
		return trace.Wrap(err)
	}

	a.accessClient, err = access.NewClient(
		ctx,
		"gitlab",
		a.conf.Teleport.AuthServer,
		tlsConf,
	)
	if err != nil {
		return trace.Wrap(err)
	}

	// Initialize the process.
	a.Process = utils.NewProcess(ctx)

	var initErr, httpErr, watcherErr error

	a.Spawn(func(ctx context.Context) {
		if initErr = trace.Wrap(a.checkTeleportVersion(ctx)); initErr != nil {
			a.Terminate()
			return
		}

		// var realProjectID IntID

		log.Debug("Starting GitLab API health check...")
		realProjectID, initErr := a.bot.HealthCheck(ctx)
		if initErr != nil {
			log.Error("GitLab API health check failed")
			a.Terminate()
			return
		}
		log.Debug("GitLab API health check finished ok")

		log.Debug("Opening the database...")
		a.db, initErr = OpenDB(a.conf.DB.Path, realProjectID)
		if initErr != nil {
			log.Error("Failed to open the database...")
			a.Terminate()
			return
		}

		log.Debug("Setting up the project")
		if initErr = a.setup(ctx); initErr != nil {
			log.Error("Failed to set up project")
			a.Terminate()
			return
		}
		log.Debug("GitLab project setup finished ok")

		watcherDone := make(chan struct{})
		httpDone := make(chan struct{})

		a.OnTerminate(func(_ context.Context) {
			<-watcherDone
			<-httpDone
			a.db.Close()
		})

		a.Spawn(func(ctx context.Context) {
			defer a.Terminate() // if webhook server failed, shutdown everything
			defer close(httpDone)
			// Run a webhook server
			err := a.bot.RunServer(ctx)
			httpErr = trace.Wrap(err)
		})
		a.OnTerminate(func(ctx context.Context) {
			if err := a.bot.ShutdownServer(ctx); err != nil {
				log.Error("HTTP server graceful shutdown failed")
			}
		})

		a.Spawn(func(ctx context.Context) {
			defer a.Terminate() // if watcher failed, shutdown everything
			defer close(watcherDone)

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

	return trace.NewAggregate(initErr, httpErr, watcherErr)
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

func (a *App) setup(ctx context.Context) error {
	return a.db.UpdateSettings(func(settings Settings) (err error) {
		webhookID := settings.HookID()
		if webhookID, err = a.bot.SetupProjectHook(ctx, webhookID); err != nil {
			return
		}
		if err = settings.SetHookID(webhookID); err != nil {
			return
		}

		labels := settings.GetLabels(
			"pending",
			"approved",
			"denied",
			"expired",
		)
		if err = a.bot.SetupLabels(ctx, labels); err != nil {
			return
		}
		if err = settings.SetLabels(a.bot.labels); err != nil {
			return
		}
		return
	})
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
						log := log.WithField("request_id", req.ID).WithError(err)
						log.Errorf("Failed to process pending request")
						log.Debugf("%v", trace.DebugReport(err))
					}
				})
			case access.OpDelete:
				a.Spawn(func(ctx context.Context) {
					if err := a.onDeletedRequest(ctx, req); err != nil {
						log := log.WithField("request_id", req.ID).WithError(err)
						log.Errorf("Failed to process deleted request")
						log.Debugf("%v", trace.DebugReport(err))
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

func (a *App) OnWebhookEvent(ctx context.Context, hook Webhook) error {
	log := log.WithFields(logFields{
		"gitlab_http_id": hook.HTTPID,
	})
	// Not an issue event
	event, ok := hook.Event.(IssueEvent)
	if !ok {
		return nil
	}
	// Non-update action
	if eventAction := event.ObjectAttributes.Action; eventAction != "update" {
		return nil
	}
	// No labels changed
	if event.Changes.Labels == nil {
		return nil
	}

	var action ActionID

	for _, label := range event.Changes.Labels.Diff() {
		action = LabelName(label.Title).ToAction()
		if action != NoAction {
			break
		}
	}
	if action == NoAction {
		log.Debug("No approved/denied labels set, ignoring")
		return nil
	}

	issueID := event.ObjectAttributes.ID
	var reqID string
	err := a.db.ViewIssues(func(issues Issues) error {
		reqID = issues.GetRequestID(issueID)
		return nil
	})
	if trace.Unwrap(err) == ErrNoBucket || reqID == "" {
		log.WithError(err).Warning("Failed to find an issue in database")
		if reqID = event.ObjectAttributes.ParseDescriptionRequestID(); reqID == "" {
			// Ignore the issue, probably it wasn't created by us at all.
			return nil
		}
		log.WithField("request_id", reqID).Warning("Request ID was parsed from issue description")
	} else if err != nil {
		return trace.Wrap(err)
	}

	req, err := a.accessClient.GetRequest(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).WithField("request_id", reqID).Warning("Cannot process expired request")
			return nil
		}
		return trace.Wrap(err)
	}
	if req.State != access.StatePending {
		return trace.Errorf("cannot process not pending request: %+v", req)
	}

	pluginData, err := a.GetPluginData(ctx, reqID)
	if err != nil {
		return trace.Wrap(err)
	}

	if pluginData.GitlabData.ID != issueID {
		log.WithFields(logFields{
			"gitlab_issue_id":      issueID,
			"plugin_data_issue_id": pluginData.GitlabData.ID,
		}).Debug("plugin_data.issue_id does not match event.issue_id")
		return trace.Errorf("issue_id from request's plugin_data does not match")
	}

	log = log.WithFields(logFields{
		"gitlab_project_id":    event.ObjectAttributes.ProjectID,
		"gitlab_issue_iid":     event.ObjectAttributes.IID,
		"gitlab_user_name":     event.User.Name,
		"gitlab_user_username": event.User.Username,
		"gitlab_user_email":    event.User.Email,
	})

	var (
		reqState   access.State
		resolution string
	)

	switch action {
	case ApproveAction:
		reqState = access.StateApproved
		resolution = "approved"
	case DenyAction:
		reqState = access.StateDenied
		resolution = "denied"
	default:
		return trace.BadParameter("unknown action: %v", action)
	}

	if err := a.accessClient.SetRequestState(ctx, req.ID, reqState); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("GitLab user %s the request", resolution)

	if err := a.bot.CloseIssue(ctx, event.ObjectAttributes.IID, ""); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Issue successfully closed")
	return nil
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	var err error
	reqData := RequestData{User: req.User, Roles: req.Roles, Created: req.Created}

	gitlabData, err := a.bot.CreateIssue(ctx, req.ID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithFields(logFields{
		"request_id":        req.ID,
		"gitlab_project_id": gitlabData.ProjectID,
		"gitlab_issue_iid":  gitlabData.IID,
	}).Info("GitLab issue created")

	err = a.db.UpdateIssues(func(issues Issues) error {
		return issues.SetRequestID(gitlabData.ID, req.ID)
	})
	if err != nil {
		return trace.Wrap(err)
	}

	err = a.SetPluginData(ctx, req.ID, PluginData{reqData, gitlabData})
	return trace.Wrap(err)
}

func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
	reqID := req.ID // This is the only available field
	pluginData, err := a.GetPluginData(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warn("Cannot expire unknown request")
			return nil
		}
		return trace.Wrap(err)
	}

	if err := a.bot.CloseIssue(ctx, pluginData.GitlabData.IID, "expired"); err != nil {
		return trace.Wrap(err)
	}

	log.WithField("request_id", reqID).Info("Successfully marked request as expired")

	return nil
}

func (a *App) GetPluginData(ctx context.Context, reqID string) (data PluginData, err error) {
	dataMap, err := a.accessClient.GetPluginData(ctx, reqID)
	if err != nil {
		return data, trace.Wrap(err)
	}
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	var created int64
	fmt.Sscanf(dataMap["created"], "%d", &created)
	data.Created = time.Unix(created, 0)
	fmt.Sscanf(dataMap["issue_id"], "%d", &data.ID)
	fmt.Sscanf(dataMap["issue_iid"], "%d", &data.IID)
	fmt.Sscanf(dataMap["project_id"], "%d", &data.ProjectID)
	return
}

func (a *App) SetPluginData(ctx context.Context, reqID string, data PluginData) error {
	return a.accessClient.UpdatePluginData(ctx, reqID, access.PluginData{
		"issue_id":   fmt.Sprintf("%d", data.ID),
		"issue_iid":  fmt.Sprintf("%d", data.IID),
		"project_id": fmt.Sprintf("%d", data.ProjectID),
		"user":       data.User,
		"roles":      strings.Join(data.Roles, ","),
		"created":    fmt.Sprintf("%d", data.Created.Unix()),
	}, nil)
}
