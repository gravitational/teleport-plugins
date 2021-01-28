package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/access/integration"
	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"
	"github.com/nlopes/slack"

	. "gopkg.in/check.v1"
)

const (
	Host        = "localhost"
	HostID      = "00000000-0000-0000-0000-000000000000"
	Site        = "local-site"
	SlackSecret = "f9e77a2814566fe23d33dee5b853955b"
)

type SlackSuite struct {
	ctx        context.Context
	cancel     context.CancelFunc
	appConfig  Config
	app        *App
	publicURL  string
	raceNumber int
	me         *user.User
	fakeSlack  *FakeSlack
	slackUser  slack.User
	teleport   *integration.TeleInstance
	tmpFiles   []*os.File
}

var _ = Suite(&SlackSuite{})

func TestSlackbot(t *testing.T) { TestingT(t) }

func (s *SlackSuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	t := integration.NewInstance(integration.InstanceConfig{ClusterName: Site, HostID: HostID, NodeName: Host, Priv: priv, Pub: pub})

	s.raceNumber = runtime.GOMAXPROCS(0)
	s.me, err = user.Current()
	c.Assert(err, IsNil)
	userRole, err := services.NewRole("foo", services.RoleSpecV3{
		Allow: services.RoleConditions{
			Logins:  []string{s.me.Username}, // cannot be empty
			Request: &services.AccessRequestConditions{Roles: []string{"admin"}},
		},
	})
	c.Assert(err, IsNil)
	t.AddUserWithRole(s.me.Username, userRole)

	accessPluginRole, err := services.NewRole("access-plugin", services.RoleSpecV3{
		Allow: services.RoleConditions{
			Logins: []string{"access-plugin"}, // cannot be empty
			Rules: []services.Rule{
				services.NewRule("access_request", []string{"list", "read", "update"}),
			},
		},
	})
	c.Assert(err, IsNil)
	t.AddUserWithRole("plugin", accessPluginRole)

	err = t.Create(nil, nil)
	c.Assert(err, IsNil)
	if err := t.Start(); err != nil {
		c.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = t
}

func (s *SlackSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Second)
	s.publicURL = ""
	s.fakeSlack = NewFakeSlack(slack.User{Name: "slackbot"}, s.raceNumber)
	s.slackUser = s.fakeSlack.StoreUser(slack.User{
		Name: s.me.Username,
		Profile: slack.UserProfile{
			Email: s.me.Username + "@example.com",
		},
	})

	auth := s.teleport.Process.GetAuthServer()
	certAuthorities, err := auth.GetCertAuthorities(services.HostCA, false)
	c.Assert(err, IsNil)
	pluginKey := s.teleport.Secrets.Users["plugin"].Key

	keyFile := s.newTmpFile(c, "auth.*.key")
	_, err = keyFile.Write(pluginKey.Priv)
	c.Assert(err, IsNil)
	keyFile.Close()

	certFile := s.newTmpFile(c, "auth.*.crt")
	_, err = certFile.Write(pluginKey.TLSCert)
	c.Assert(err, IsNil)
	certFile.Close()

	casFile := s.newTmpFile(c, "auth.*.cas")
	for _, ca := range certAuthorities {
		for _, keyPair := range ca.GetTLSKeyPairs() {
			_, err = casFile.Write(keyPair.Cert)
			c.Assert(err, IsNil)
		}
	}
	casFile.Close()

	authAddr, err := s.teleport.Process.AuthSSHAddr()
	c.Assert(err, IsNil)

	var conf Config
	conf.Teleport.AuthServer = authAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.Slack.Secret = SlackSecret
	conf.Slack.Token = "000000"
	conf.Slack.Channel = "test"
	conf.Slack.APIURL = s.fakeSlack.URL() + "/"
	conf.HTTP.ListenAddr = ":0"
	conf.HTTP.Insecure = true

	s.appConfig = conf
}

func (s *SlackSuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakeSlack.Close()
	s.cancel()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *SlackSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *SlackSuite) startApp(c *C) {
	var err error

	if s.publicURL != "" {
		s.appConfig.HTTP.PublicAddr = s.publicURL
	}
	s.app, err = NewApp(s.appConfig)
	c.Assert(err, IsNil)

	go func() {
		if err := s.app.Run(s.ctx); err != nil {
			panic(err)
		}
	}()
	ok, err := s.app.WaitReady(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)
	if s.publicURL == "" {
		s.publicURL = s.app.PublicURL().String()
	}
}

func (s *SlackSuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(s.app.Err(), IsNil)
}

func (s *SlackSuite) createAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *SlackSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateExpiredAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *SlackSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "slack", reqID)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *SlackSuite) pressBlockButton(c *C, msg slack.Msg, blockID, actionID string) {
	actionBlock := findActionBlock(msg, blockID)
	c.Assert(actionBlock, NotNil, Commentf("block action %q not found", blockID))
	button := findButton(*actionBlock, actionID)
	c.Assert(button, NotNil, Commentf("cannot find block element with action %q", actionID))
	s.postCallbackAndCheck(c, msg, button.ActionID, button.Value, http.StatusOK)
}

func (s *SlackSuite) postCallbackAndCheck(c *C, msg slack.Msg, actionID, value string, expectedStatus int) {
	resp, err := s.postCallback(s.ctx, msg, actionID, value)
	c.Assert(err, IsNil)
	c.Assert(resp.Body.Close(), IsNil)
	c.Assert(resp.StatusCode, Equals, expectedStatus)
}

func (s *SlackSuite) postCallback(ctx context.Context, msg slack.Msg, actionID, value string) (*http.Response, error) {
	cb := &slack.InteractionCallback{
		User: slack.User{
			ID:   s.slackUser.ID,
			Name: s.slackUser.Name,
		},
		ActionCallback: slack.ActionCallbacks{
			BlockActions: []*slack.BlockAction{
				{
					ActionID: actionID,
					Value:    value,
				},
			},
		},
		ResponseURL: fmt.Sprintf("%s/_response/%s", s.fakeSlack.URL(), msg.Timestamp),
	}

	payload, err := json.Marshal(cb)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	data := url.Values{
		"payload": {string(payload)},
	}
	body := data.Encode()

	stimestamp := fmt.Sprintf("%d", time.Now().Unix())
	hash := hmac.New(sha256.New, []byte(SlackSecret))
	_, err = hash.Write([]byte(fmt.Sprintf("v0:%s:%s", stimestamp, body)))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	signature := hash.Sum(nil)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.publicURL, strings.NewReader(body))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("X-Slack-Request-Timestamp", stimestamp)
	req.Header.Add("X-Slack-Signature", "v0="+hex.EncodeToString(signature))

	response, err := http.DefaultClient.Do(req)
	return response, trace.Wrap(err)
}

// Tests if Interactive Mode posts Slack message with buttons correctly
func (s *SlackSuite) TestSlackMessagePosting(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	pluginData := s.checkPluginData(c, request.GetName())
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(pluginData.Timestamp, Equals, msg.Timestamp)
	c.Assert(pluginData.ChannelID, Equals, msg.Channel)

	actionBlock := findActionBlock(msg, "approve_or_deny")
	c.Assert(actionBlock, NotNil)
	c.Assert(actionBlock.Elements.ElementSet, HasLen, 2)

	approveButton := findButton(*actionBlock, "approve_request")
	c.Assert(approveButton, NotNil)
	c.Assert(approveButton.Value, Equals, request.GetName())

	denyButton := findButton(*actionBlock, "deny_request")
	c.Assert(denyButton, NotNil)
	c.Assert(denyButton.Value, Equals, request.GetName())
}

// Tests if Interactive Mode posts Slack message with buttons correctly
func (s *SlackSuite) TestSlackMessagePostingReadonly(c *C) {
	s.appConfig.Slack.NotifyOnly = true
	s.startApp(c)
	request := s.createAccessRequest(c)
	pluginData := s.checkPluginData(c, request.GetName())
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(pluginData.Timestamp, Equals, msg.Timestamp)
	c.Assert(pluginData.ChannelID, Equals, msg.Channel)

	actionBlock := findActionBlock(msg, "approve_or_deny")
	c.Assert(actionBlock, IsNil, Commentf("there should be no buttons block in readonly mode"))
}

func (s *SlackSuite) TestApproval(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	s.pressBlockButton(c, msg, "approve_or_deny", "approve_request")

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByResponding(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msgUpdate.Timestamp, Equals, msg.Timestamp)
	actionBlock := findActionBlock(msgUpdate, "approve_or_deny")
	c.Assert(actionBlock, IsNil, Commentf("there should be no buttons block after request approval"))

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)

	auditLog, err := s.teleport.FilterAuditEvents("", events.EventFields{"event": events.AccessRequestUpdateEvent, "id": request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(auditLog, HasLen, 1)
	c.Assert(auditLog[0].GetString("state"), Equals, "APPROVED")
	c.Assert(auditLog[0].GetString("delegator"), Equals, "slack:"+s.slackUser.Profile.Email)
}

func (s *SlackSuite) TestDenial(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	s.pressBlockButton(c, msg, "approve_or_deny", "deny_request")

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByResponding(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msgUpdate.Timestamp, Equals, msg.Timestamp)
	actionBlock := findActionBlock(msgUpdate, "approve_or_deny")
	c.Assert(actionBlock, IsNil, Commentf("there should be no buttons block after request denial"))

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)

	auditLog, err := s.teleport.FilterAuditEvents("", events.EventFields{"event": events.AccessRequestUpdateEvent, "id": request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(auditLog, HasLen, 1)
	c.Assert(auditLog[0].GetString("state"), Equals, "DENIED")
	c.Assert(auditLog[0].GetString("delegator"), Equals, "slack:"+s.slackUser.Profile.Email)
}

func (s *SlackSuite) TestApproveReadonly(c *C) {
	s.appConfig.Slack.NotifyOnly = true
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	s.postCallbackAndCheck(c, msg, "approve_request", request.GetName(), http.StatusUnauthorized)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_PENDING)
}

func (s *SlackSuite) TestDenyReadonly(c *C) {
	s.appConfig.Slack.NotifyOnly = true
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	s.postCallbackAndCheck(c, msg, "deny_request", request.GetName(), http.StatusUnauthorized)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_PENDING)
}

func (s *SlackSuite) TestExpiration(c *C) {
	s.startApp(c)
	s.createExpiredAccessRequest(c)
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByAPI(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg.Timestamp, Equals, msgUpdate.Timestamp)
	c.Assert(findActionBlock(msgUpdate, "approve_or_deny"), IsNil, Commentf("there should be no buttons block after request expiration"))
}

func (s *SlackSuite) TestApproveExpired(c *C) {
	s.startApp(c)
	s.createExpiredAccessRequest(c)
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)

	msg1, err := s.fakeSlack.CheckMessageUpdateByAPI(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg1.Timestamp, Equals, msg.Timestamp)

	msg = s.fakeSlack.StoreMessage(msg) // Restore the message to have an action block.
	c.Assert(findActionBlock(msg, "approve_or_deny"), NotNil, Commentf("there should be an action block"))

	s.pressBlockButton(c, msg, "approve_or_deny", "approve_request")

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByResponding(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg.Timestamp, Equals, msgUpdate.Timestamp)
	c.Assert(findActionBlock(msgUpdate, "approve_or_deny"), IsNil, Commentf("there should be no buttons block after request expiration"))
}

func (s *SlackSuite) TestDenyExpired(c *C) {
	s.startApp(c)
	s.createExpiredAccessRequest(c)
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)

	msg1, err := s.fakeSlack.CheckMessageUpdateByAPI(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg1.Timestamp, Equals, msg.Timestamp)

	msg = s.fakeSlack.StoreMessage(msg) // Restore the message to have an action block.
	c.Assert(findActionBlock(msg, "approve_or_deny"), NotNil, Commentf("there should be an action block"))

	s.pressBlockButton(c, msg, "approve_or_deny", "deny_request")

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByResponding(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg.Timestamp, Equals, msgUpdate.Timestamp)
	c.Assert(findActionBlock(msgUpdate, "approve_or_deny"), IsNil, Commentf("there should be no buttons block after request expiration"))
}

func (s *SlackSuite) TestRace(c *C) {
	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	s.cancel() // Cancel the default timeout
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Second)
	s.startApp(c)

	var (
		raceErr     error
		raceErrOnce sync.Once
		requests    sync.Map
	)
	setRaceErr := func(err error) error {
		raceErrOnce.Do(func() {
			raceErr = err
		})
		return err
	}

	watcher, err := s.teleport.Process.GetAuthServer().NewWatcher(s.ctx, services.Watch{
		Kinds: []services.WatchKind{
			{
				Kind: services.KindAccessRequest,
			},
		},
	})
	c.Assert(err, IsNil)
	defer watcher.Close()
	c.Assert((<-watcher.Events()).Type, Equals, backend.OpInit)

	process := utils.NewProcess(s.ctx)
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			_, err := s.teleport.CreateAccessRequest(ctx, s.me.Username, "admin")
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			msg, err := s.fakeSlack.CheckNewMessage(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			actionBlock := findActionBlock(msg, "approve_or_deny")
			if actionBlock == nil {
				return setRaceErr(trace.Errorf("action block not found"))
			}
			if obtained, expected := len(actionBlock.Elements.ElementSet), 2; obtained != expected {
				return setRaceErr(trace.Errorf("wrong block elements size. expected %v, obtained %v", expected, obtained))
			}
			button := findButton(*actionBlock, "approve_request")
			if button == nil {
				return setRaceErr(trace.Errorf("approve button is not found"))
			}

			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			var lastErr error
			for {
				log.Infof("Trying to press \"Approve\" on msg %q", msg.Timestamp)
				resp, err := s.postCallback(ctx, msg, button.ActionID, button.Value)
				if err != nil {
					if utils.IsDeadline(err) {
						return setRaceErr(lastErr)
					}
					return setRaceErr(trace.Wrap(err))
				}
				if err := resp.Body.Close(); err != nil {
					return setRaceErr(trace.Wrap(err))
				}
				if status := resp.StatusCode; status != http.StatusOK {
					lastErr = trace.Errorf("got %v http code from webhook server", status)
				} else {
					return nil
				}
			}
		})
		process.SpawnCritical(func(ctx context.Context) error {
			msg, err := s.fakeSlack.CheckMessageUpdateByResponding(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			if obtained, expected := findActionBlock(msg, "approve_or_deny"), (*slack.ActionBlock)(nil); obtained != expected {
				return setRaceErr(trace.Errorf("there should be no buttons block after request approval"))
			}
			return nil
		})
	}
	for i := 0; i < 2*s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			var event services.Event
			select {
			case event = <-watcher.Events():
			case <-ctx.Done():
				return setRaceErr(trace.Wrap(ctx.Err()))
			}
			if obtained, expected := event.Type, backend.OpPut; obtained != expected {
				return setRaceErr(trace.Errorf("wrong event type. expected %v, obtained %v", expected, obtained))
			}
			req := event.Resource.(services.AccessRequest)
			var newCounter int64
			val, _ := requests.LoadOrStore(req.GetName(), &newCounter)
			switch state := req.GetState(); state {
			case services.RequestState_PENDING:
				atomic.AddInt64(val.(*int64), 1)
			case services.RequestState_APPROVED:
				atomic.AddInt64(val.(*int64), -1)
			default:
				return setRaceErr(trace.Errorf("wrong request state %v", state))
			}
			return nil
		})
	}
	process.Terminate()
	<-process.Done()
	c.Assert(raceErr, IsNil)

	var count int
	requests.Range(func(key, val interface{}) bool {
		count++
		c.Assert(*val.(*int64), Equals, int64(0))
		return true
	})
	c.Assert(count, Equals, s.raceNumber)
}

func findActionBlock(msg slack.Msg, blockID string) *slack.ActionBlock {
	for _, block := range msg.Blocks.BlockSet {
		if actionBlock, ok := block.(*slack.ActionBlock); ok && actionBlock.BlockID == blockID {
			return actionBlock
		}
	}
	return nil
}

func findButton(block slack.ActionBlock, actionID string) *slack.ButtonBlockElement {
	for _, element := range block.Elements.ElementSet {
		buttonElement, ok := element.(*slack.ButtonBlockElement)
		if ok && buttonElement.ActionID == actionID {
			return buttonElement
		}
	}
	return nil
}
