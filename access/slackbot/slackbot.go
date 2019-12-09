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
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/trace"
	"github.com/nlopes/slack"
	"github.com/pelletier/go-toml"
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
	pgrm := os.Args[0]
	args := os.Args[1:]
	if len(args) < 1 {
		bail("USAGE: %s (configure | <config-path>)", pgrm)
	}
	if args[0] == "configure" {
		fmt.Print(exampleConfig)
		return
	}
	if err := run(args[0]); err != nil {
		bail("ERROR: %s", err)
	}
}

func run(configPath string) error {
	conf, err := LoadConfig(configPath)
	if err != nil {
		return trace.Wrap(err)
	}
	ctx := context.Background()
	app, err := NewApp(ctx, *conf)
	if err != nil {
		return trace.Wrap(err)
	}
	app.Start()
	app.Wait()
	return nil
}

type killSwitch func()

// App contains global application state.
type App struct {
	accessClient access.Client
	slackClient  *slack.Client
	httpServer   *http.Server
	cache        *RequestCache
	conf         Config
	ctx          context.Context
	stop         killSwitch
}

func NewApp(ctx context.Context, conf Config) (*App, error) {
	tc, err := conf.LoadTLSConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	accessClient, err := access.NewClient(ctx, conf.Teleport.AuthServer, tc)
	if err != nil {
		cancel()
		return nil, trace.Wrap(err)
	}
	slackClient := slack.New(conf.Slack.Token)
	if err != nil {
		cancel()
		return nil, trace.Wrap(err)
	}
	httpServer := &http.Server{
		Addr: conf.Slack.Listen,
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			log.Infof("Attempting graceful shutdown...")
			cancel()
			sctx, _ := context.WithTimeout(context.TODO(), time.Second*20)
			if err := httpServer.Shutdown(sctx); err != nil {
				log.Errorf("Server shutdown failed: %s", err)
			}
		})
	}
	cache := NewRequestCache(ctx)
	app := &App{
		accessClient: accessClient,
		slackClient:  slackClient,
		httpServer:   httpServer,
		cache:        cache,
		conf:         conf,
		ctx:          ctx,
		stop:         stop,
	}
	http.HandleFunc("/", app.ActionsHandler)
	return app, nil
}

func (a *App) Stop() {
	a.stop()
}

// TODO: add error consolidation
func (a *App) Wait() {
	<-a.ctx.Done()
}

func (a *App) Start() {
	go func() {
		defer a.Stop()
		log.Infof("Starting server on %s", a.conf.Slack.Listen)
		if err := a.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server failed: %s", err)
		}
	}()
	go func() {
		defer a.Stop()
		if err := a.watchRequests(); err != nil {
			log.Fatalf("Watcher failed: %s", err)
		}
	}()
}

func (a *App) watchRequests() error {
	watcher, err := a.accessClient.WatchRequests(a.ctx, access.Filter{
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
				entry, ok := a.cache.Pop(req.ID)
				if !ok {
					log.Warnf("cannot expire unknown request %q", req.ID)
					continue Watch
				}
				if err := a.expireEntry(entry); err != nil {
					return trace.Wrap(err)
				}
			default:
				return trace.BadParameter("unexpected event operation %s", op)
			}
		case <-watcher.Done():
			return watcher.Error()
		}
	}
}

// loadRequest loads the specified request in order to correctly format
// a response.
func (a *App) loadRequest(reqID string) (access.Request, error) {
	entry, ok := a.cache.Pop(reqID)
	if ok {
		return entry.Request, nil
	}
	log.Warnf("Cache-miss for request %s", reqID)
	reqs, err := a.accessClient.GetRequests(a.ctx, access.Filter{
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

func (a *App) OnCallback(cb slack.InteractionCallback) error {
	if len(cb.ActionCallback.BlockActions) != 1 {
		return trace.BadParameter("expected exactly one block action")
	}
	action := cb.ActionCallback.BlockActions[0]
	var req access.Request
	var err error
	var status string
	switch action.ActionID {
	case ActionApprove:
		req, err = a.loadRequest(action.Value)
		if err != nil {
			return trace.Wrap(err)
		}
		log.Infof("Approving request %+v", req)
		if err := a.accessClient.SetRequestState(a.ctx, req.ID, access.StateApproved); err != nil {
			return trace.Wrap(err)
		}
		status = "APPROVED"
	case ActionDeny:
		req, err = a.loadRequest(action.Value)
		if err != nil {
			return trace.Wrap(err)
		}
		log.Infof("Denying request %+v", req)
		if err := a.accessClient.SetRequestState(a.ctx, req.ID, access.StateDenied); err != nil {
			return trace.Wrap(err)
		}
		status = "DENIED"
	default:
		return trace.BadParameter("Unknown ActionID: %s", action.ActionID)
	}
	go func() {
		msg := msgText(
			req.ID,
			req.User,
			req.Roles,
			status,
		)
		sec := msgSection(msg)
		secJson, err := json.Marshal(&sec)
		if err != nil {
			log.Errorf("Failed to serialize msg block: %s", err)
			return
		}
		// I am literally at a loss for how to achieve this using nlopes/slack,
		// so we're just hard-coding the json and manually POSTing it...
		// Janky? yes. Better than nothing? Also yes.
		rjson := fmt.Sprintf(`{"blocks":[%s],"replace_original":"true"}`, secJson)
		rsp, err := http.Post(cb.ResponseURL, "application/json", strings.NewReader(rjson))
		if err != nil {
			log.Errorf("Failed to send update: %s", err)
			return
		}
		rbody, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			log.Errorf("Failed to read update response: %s", err)
			return
		}
		var ursp struct {
			Ok bool `json:"ok"`
		}
		if err := json.Unmarshal(rbody, &ursp); err != nil {
			log.Errorf("Failed to parse response body: %s", err)
			return
		}
		if !ursp.Ok {
			log.Errorf("Failed to update msg for %+v", req)
			return
		}
		log.Debug("Successfully updated msg for %+v", req)
	}()
	return nil
}

func (a *App) ActionsHandler(w http.ResponseWriter, r *http.Request) {
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
	if err := a.OnCallback(cb); err != nil {
		log.Errorf("Failed to process callback: %s", err)
		http.Error(w, "failed to process callback", 500)
		return
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
	log.Debugf("Posted to channel %s at time %s\n", channelID, timestamp)
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

type Config struct {
	Teleport struct {
		AuthServer string `toml:"auth-server"`
		ClientKey  string `toml:"client-key"`
		ClientCrt  string `toml:"client-crt"`
		RootCAs    string `toml:"root-cas"`
	} `toml:"teleport"`
	Slack struct {
		Token   string `toml:"token"`
		Secret  string `toml:"secret"`
		Channel string `toml:"channel"`
		Listen  string `toml:"listen"`
	} `toml:"slack"`
}

const exampleConfig = `# example slackbot configuration file
[teleport]
auth-server = "example.com:3025"  # Auth GRPC API address
client-key = "path/to/client.key" # Teleport GRPC client secret key
client-crt = "path/to/client.crt" # Teleport GRPC client certificate 
root-cas = "path/to/root.cas"     # Teleport cluster CA certs

[slack]
token = "api-token"       # Slack Bot OAuth token
secret = "secret-value"   # Slack API Signing Secret
channel = "channel-name"  # Message delivery channel
listen = ":8080"          # Slack interaction callback listener
`

func LoadConfig(filepath string) (*Config, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conf := &Config{}
	if err := t.Unmarshal(conf); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}

func (c *Config) CheckAndSetDefaults() error {
	if c.Teleport.AuthServer == "" {
		c.Teleport.AuthServer = "localhost:3025"
	}
	if c.Teleport.ClientKey == "" {
		c.Teleport.ClientKey = "client.key"
	}
	if c.Teleport.ClientCrt == "" {
		c.Teleport.ClientCrt = "client.pem"
	}
	if c.Teleport.RootCAs == "" {
		c.Teleport.RootCAs = "cas.pem"
	}
	if c.Slack.Token == "" {
		return trace.BadParameter("missing required value slack.token")
	}
	if c.Slack.Secret == "" {
		return trace.BadParameter("missing required value slack.secret")
	}
	if c.Slack.Channel == "" {
		return trace.BadParameter("missing required value slack.channel")
	}
	if c.Slack.Listen == "" {
		c.Slack.Listen = ":8080"
	}
	return nil
}

// LoadTLSConfig loads client crt/key files and root authorities, and
// generates a tls.Config suitable for use with a GRPC client.
func (c *Config) LoadTLSConfig() (*tls.Config, error) {
	var tc tls.Config
	clientCert, err := tls.LoadX509KeyPair(c.Teleport.ClientCrt, c.Teleport.ClientKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tc.Certificates = append(tc.Certificates, clientCert)
	caFile, err := os.Open(c.Teleport.RootCAs)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	caCerts, err := ioutil.ReadAll(caFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caCerts); !ok {
		return nil, trace.BadParameter("invalid CA cert PEM")
	}
	tc.RootCAs = pool
	return &tc, nil
}
