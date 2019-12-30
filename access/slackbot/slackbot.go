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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gravitational/kingpin"
	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/trace"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
)

const (
	// ActionApprove uniquely identifies the approve button in events.
	ActionApprove = "approve_request"
	// ActionDeny uniquely identifies the deny button in events.
	ActionDeny = "deny_request"
)

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
	app := kingpin.New("slackbot", "Teleport plugin for access requests approval via Slack.")
	app.Command("configure", "Prints an example configuration file")
	startCmd := app.Command("start", "Starts a bot daemon")
	path := startCmd.Arg("path", "Configuration file path").
		Required().
		String()
	debug := startCmd.Flag("debug", "Enable verbose logging to stderr").
		Short('d').
		Bool()
	selectedCmd, err := app.Parse(os.Args[1:])
	if err != nil {
		bail("error: %s", err)
	}
	switch selectedCmd {
	case "configure":
		fmt.Print(exampleConfig)
	case "start":
		if *debug {
			log.SetLevel(log.DebugLevel)
			log.Debugf("DEBUG logging enabled")
		}
		if err := run(*path); err != nil {
			bail("error: %s", err)
		}
	}
}

func run(configPath string) error {
	conf, err := LoadConfig(configPath)
	if err != nil {
		return trace.Wrap(err)
	}
	ctx := context.Background()
	app, err := NewApp(*conf)
	if err != nil {
		return trace.Wrap(err)
	}
	serveSignals(app)
	err = app.Start(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	app.Wait()
	return app.Error()
}

func serveSignals(app *App) {
	sigC := make(chan os.Signal)
	signal.Notify(sigC,
		syscall.SIGQUIT, // graceful shutdown
		syscall.SIGTERM, // fast shutdown
		syscall.SIGKILL, // fast shutdown
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
	waitCond     *sync.Cond
	accessClient access.Client
	slackClient  *slack.Client
	httpServer   *http.Server
	cache        *RequestCache
	conf         Config
	tlsConf      *tls.Config
	stop         killSwitch
	cancel       context.CancelFunc
	err          error
}

func NewApp(conf Config) (*App, error) {
	tc, err := conf.LoadTLSConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return NewAppWithTLSConfig(conf, tc)
}

func NewAppWithTLSConfig(conf Config, tlsConf *tls.Config) (*App, error) {
	app := &App{conf: conf, tlsConf: tlsConf}

	slackOptions := []slack.Option{}
	if conf.Slack.APIURL != "" {
		slackOptions = append(slackOptions, slack.OptionAPIURL(conf.Slack.APIURL))
	}
	app.slackClient = slack.New(conf.Slack.Token, slackOptions...)
	return app, nil
}

func (a *App) finish(err error) {
	a.Lock()
	defer a.Unlock()
	a.err = err
	a.cache = nil
	a.httpServer = nil
	a.stop = nil
	a.cancel = nil
	a.waitCond = nil
}

// Close signals the App to shutdown immediately
func (a *App) Close() {
	a.Lock()
	defer a.Unlock()
	if cancel := a.cancel; cancel != nil {
		log.Infof("Force shutdown...")
		cancel()
	}
}

// Stop signals the App to shutdown gracefully
func (a *App) Stop() {
	a.Lock()
	defer a.Unlock()
	if stop := a.stop; stop != nil {
		stop()
	}
}

// Wait blocks until watcher and http server finish
func (a *App) Wait() {
	a.Lock()
	defer a.Unlock()
	if cond := a.waitCond; cond != nil {
		cond.Wait()
	}
}

// Error returns the error returned by watcher/http server or them both
func (a *App) Error() error {
	a.Lock()
	defer a.Unlock()
	return a.err
}

// Start runs a watcher and http server
func (a *App) Start(ctx context.Context) error {
	a.Lock()
	defer a.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	if client, err := access.NewClient(ctx, a.conf.Teleport.AuthServer, a.tlsConf); err == nil {
		a.accessClient = client
	} else {
		cancel()
		return trace.Wrap(err)
	}
	a.cancel = cancel

	a.cache = NewRequestCache(ctx)

	var (
		httpErr, watcherErr error
		workers             sync.WaitGroup
	)
	workers.Add(2)
	cond := sync.NewCond(a)
	a.waitCond = cond
	a.err = nil

	httpServer := a.newHttpServer(ctx)
	a.httpServer = httpServer

	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			defer cancel() // We have to wait first because cancellation breaks cache
			log.Infof("Attempting graceful shutdown...")
			sctx, scancel := context.WithTimeout(ctx, time.Second*20)
			defer scancel()
			if err := httpServer.Shutdown(sctx); err != nil {
				log.Errorf("HTTP server graceful shutdown failed: %s", err)
			}
		})
	}
	a.stop = func() { go shutdown() }

	go func() {
		defer shutdown()
		defer workers.Done()
		log.Infof("Starting server on %s", a.conf.Slack.Listen)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			httpErr = trace.Wrap(err)
		}
	}()
	go func() {
		<-ctx.Done()
		httpServer.Close()
	}()
	go func() {
		defer shutdown()
		defer workers.Done()
		if err := a.watchRequests(ctx); err != nil {
			watcherErr = trace.Wrap(err)
		}
	}()
	go func() {
		defer cond.Broadcast()
		workers.Wait()
		a.finish(trace.NewAggregate(httpErr, watcherErr))
	}()
	return nil
}

// newHttpServer creates a http server bound to the current context
func (a *App) newHttpServer(ctx context.Context) *http.Server {
	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.ActionsHandler(ctx, w, r)
	})
	return &http.Server{
		Addr:    a.conf.Slack.Listen,
		Handler: serverMux,
	}
}

// watchRequests starts a GRPC watcher which monitors access requests
func (a *App) watchRequests(ctx context.Context) error {
	watcher, err := a.accessClient.WatchRequests(ctx, access.Filter{
		State: access.StatePending,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	defer watcher.Close()
Watch:
	for {
		select {
		case event := <-watcher.Events():
			req, op := event.Request, event.Type
			switch op {
			case access.OpInit:
				log.Infof("watcher initialized...")
			case access.OpPut:
				if !req.State.IsPending() {
					log.Warnf("non-pending request event %+v", event)
					continue Watch
				}
				if err := a.OnPendingRequest(req); err != nil {
					return trace.Wrap(err)
				}
			case access.OpDelete:
				entry, err := a.cache.Pop(req.ID)
				if err != nil {
					if trace.IsNotFound(err) {
						log.Warnf("cannot expire unknown request: %s", err)
						continue Watch
					} else {
						return trace.Wrap(err)
					}
				}
				if err := a.expireEntry(entry); err != nil {
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

// loadRequest loads the specified request in order to correctly format
// a response.
func (a *App) loadRequest(ctx context.Context, reqID string) (access.Request, error) {
	if entry, err := a.cache.Pop(reqID); err == nil {
		return entry.Request, nil
	} else {
		if trace.IsNotFound(err) {
			log.Warnf("Cache-miss: %s", err)
		} else {
			return access.Request{}, trace.Wrap(err)
		}
	}
	reqs, err := a.accessClient.GetRequests(ctx, access.Filter{
		ID: reqID,
	})
	if err != nil {
		return access.Request{}, trace.Wrap(err)
	}
	if len(reqs) < 1 {
		return access.Request{}, trace.NotFound("no request matching %q", reqID)
	}
	return reqs[0], nil
}

func (a *App) OnCallback(ctx context.Context, cb slack.InteractionCallback) (slack.Message, error) {
	if len(cb.ActionCallback.BlockActions) != 1 {
		return slack.Message{}, trace.BadParameter("expected exactly one block action")
	}
	action := cb.ActionCallback.BlockActions[0]
	var req access.Request
	var err error
	var status string
	switch action.ActionID {
	case ActionApprove:
		req, err = a.loadRequest(ctx, action.Value)
		if err != nil {
			if trace.IsNotFound(err) {
				return slack.Message{}, nil
			} else {
				return slack.Message{}, trace.Wrap(err)
			}
		}
		log.Infof("Approving request %+v", req)
		if err := a.accessClient.SetRequestState(ctx, req.ID, access.StateApproved); err != nil {
			return slack.Message{}, trace.Wrap(err)
		}
		status = "APPROVED"
	case ActionDeny:
		req, err = a.loadRequest(ctx, action.Value)
		if err != nil {
			if trace.IsNotFound(err) {
				return slack.Message{}, nil
			} else {
				return slack.Message{}, trace.Wrap(err)
			}
		}
		log.Infof("Denying request %+v", req)
		if err := a.accessClient.SetRequestState(ctx, req.ID, access.StateDenied); err != nil {
			return slack.Message{}, trace.Wrap(err)
		}
		status = "DENIED"
	default:
		return slack.Message{}, trace.BadParameter("Unknown ActionID: %s", action.ActionID)
	}
	msg := msgText(
		req.ID,
		req.User,
		req.Roles,
		status,
	)
	message := cb.OriginalMessage
	message.Blocks.BlockSet = []slack.Block { msgSection(msg) }
	return message, nil
}

func (a *App) ActionsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	sv, err := slack.NewSecretsVerifier(r.Header, a.conf.Slack.Secret)
	if err != nil {
		log.Errorf("Failed to initialize secrets verifier: %s", err)
		http.Error(w, "verification failed", 500)
		return
	}
	// tee body into verifier as it is read.
	r.Body = ioutil.NopCloser(io.TeeReader(r.Body, &sv))
	payload := []byte(r.FormValue("payload"))
	// the FormValue method exhausts the reader, so signature
	// verification can now proceed.
	if err := sv.Ensure(); err != nil {
		log.Errorf("Secret verification failed: %s", err)
		http.Error(w, "verification failed", 500)
		return
	}

	var cb slack.InteractionCallback
	if err := json.Unmarshal(payload, &cb); err != nil {
		log.Errorf("Failed to parse json response: %s", err)
		http.Error(w, "failed to parse response", 500)
		return
	}
	if msg, err := a.OnCallback(ctx, cb); err != nil {
		log.Errorf("Failed to process callback: %s", err)
		http.Error(w, "failed to process callback", 500)
	} else {
		w.Header().Add("Content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&msg)
	}
}

func (a *App) OnPendingRequest(req access.Request) error {
	msg := msgText(
		req.ID,
		req.User,
		req.Roles,
		"PENDING",
	)
	channelID, timestamp, err := a.slackClient.PostMessage(
		a.conf.Slack.Channel,
		slack.MsgOptionBlocks(msgSection(msg), actionBlock(req)),
	)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Debugf("Posted to channel %s at time %s", channelID, timestamp)
	a.cache.Put(Entry{
		Request:   req,
		ChannelID: channelID,
		Timestamp: timestamp,
	})
	return nil
}

func (a *App) expireEntry(entry Entry) error {
	msg := msgText(
		entry.Request.ID,
		entry.Request.User,
		entry.Request.Roles,
		"EXPIRED",
	)
	_, _, _, err := a.slackClient.UpdateMessage(entry.ChannelID, entry.Timestamp, slack.MsgOptionBlocks(msgSection(msg)))
	if err != nil {
		return trace.Wrap(err)
	}
	log.Debugf("Successfully marked request %s as expired", entry.Request.ID)
	return nil
}

// msgText builds the message text payload (contains markdown).
func msgText(reqID string, user string, roles []string, status string) string {
	return fmt.Sprintf(
		"```\nRequest %s\nUser    %s\nRole(s) %s\nStatus  %s```\n",
		reqID,
		user,
		strings.Join(roles, ","),
		status,
	)
}

// msgSection builds a slack message section (obeys markdown).
func msgSection(msg string) slack.SectionBlock {
	return slack.SectionBlock{
		Type: slack.MBTSection,
		Text: &slack.TextBlockObject{
			Type: slack.MarkdownType,
			Text: msg,
		},
	}
}

// actionBlock builds a slack action block for a pending request.
func actionBlock(req access.Request) *slack.ActionBlock {
	return slack.NewActionBlock(
		"approve_or_deny",
		&slack.ButtonBlockElement{
			Type:     slack.METButton,
			ActionID: ActionApprove,
			Text:     slack.NewTextBlockObject("plain_text", "Approve", true, false),
			Value:    req.ID,
			Style:    slack.StylePrimary,
		},
		&slack.ButtonBlockElement{
			Type:     slack.METButton,
			ActionID: ActionDeny,
			Text:     slack.NewTextBlockObject("plain_text", "Deny", true, false),
			Value:    req.ID,
			Style:    slack.StyleDanger,
		},
	)
}
