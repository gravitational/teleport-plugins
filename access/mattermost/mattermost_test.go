package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mm "github.com/mattermost/mattermost-server/v5/model"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/access/integration"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"

	. "gopkg.in/check.v1"
)

const (
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"
)

type MattermostSuite struct {
	ctx            context.Context
	cancel         context.CancelFunc
	app            *App
	publicURL      string
	raceNumber     int
	me             *user.User
	fakeMattermost *FakeMattermost
	mmUser         mm.User
	teleport       *integration.TeleInstance
	tmpFiles       []*os.File
}

var _ = Suite(&MattermostSuite{})

func TestMattermost(t *testing.T) { TestingT(t) }

func (s *MattermostSuite) SetUpSuite(c *C) {
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

func (s *MattermostSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Second)
	s.publicURL = ""
	s.fakeMattermost = NewFakeMattermost(s.raceNumber)
	s.mmUser = s.fakeMattermost.StoreUser(mm.User{
		FirstName: "User",
		LastName:  "Test",
		Username:  s.me.Username,
		Email:     s.me.Username + "@example.com",
	})
}

func (s *MattermostSuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakeMattermost.Close()
	s.cancel()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *MattermostSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *MattermostSuite) startApp(c *C) {
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
	conf.Mattermost.URL = s.fakeMattermost.URL()
	conf.Mattermost.Team = "test-team"
	conf.Mattermost.Channel = "test-channel"
	conf.Mattermost.Secret = "1234567812345678123456781234567812345678123456781234567812345678"
	conf.HTTP.ListenAddr = ":0"
	if s.publicURL != "" {
		conf.HTTP.PublicAddr = s.publicURL
	}
	conf.HTTP.Insecure = true

	s.app, err = NewApp(conf)
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

func (s *MattermostSuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(s.app.Err(), IsNil)
}

func (s *MattermostSuite) createAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *MattermostSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateExpiredAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *MattermostSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "mattermost", reqID)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *MattermostSuite) postWebhook(ctx context.Context, post Post, actionName string) (*http.Response, error) {
	attachments := post.Attachments()
	if size := len(attachments); size != 1 {
		return nil, trace.Errorf("ambigous attachments array: expected exactly 1 element, got %v", size)
	}
	var action *PostAction
	for _, a := range attachments[0].Actions {
		if a.Name == actionName {
			action = &a
			break
		}
	}
	if action == nil {
		return nil, trace.Errorf("cannot find action %q in the attachments", actionName)
	}

	payload := mm.PostActionIntegrationRequest{
		PostId:    post.ID,
		TeamId:    "1111",
		ChannelId: "2222",
		UserId:    s.mmUser.Id,
		Context:   action.Integration.Context,
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&payload)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, action.Integration.URL, &buf)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	request.Header.Add("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	return response, trace.Wrap(err)

}

func (s *MattermostSuite) postWebhookAndCheck(c *C, post Post, actionName string) ActionResponse {
	response, err := s.postWebhook(s.ctx, post, actionName)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var actionResponse ActionResponse
	c.Assert(json.NewDecoder(response.Body).Decode(&actionResponse), IsNil)
	c.Assert(response.Body.Close(), IsNil)

	return actionResponse
}

func (s *MattermostSuite) TestMattermostMessagePosting(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))

	pluginData := s.checkPluginData(c, request.GetName())
	c.Assert(pluginData.PostID, Equals, post.ID)

	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	attachment := attachments[0]
	c.Assert(attachment.Actions, HasLen, 2)
	c.Assert(attachment.Actions[0].Name, Equals, "Approve")
	c.Assert(attachment.Actions[1].Name, Equals, "Deny")
}

func (s *MattermostSuite) TestApproval(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))

	response := s.postWebhookAndCheck(c, post, "Approve")
	c.Assert(response.EphemeralText, Equals, fmt.Sprintf("You have **approved** the request %s", request.GetName()))
	c.Assert(response.Update, NotNil)
	attachments := response.Update.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)

	auditLog, err := s.teleport.FilterAuditEvents("", events.EventFields{"event": events.AccessRequestUpdateEvent, "id": request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(auditLog, HasLen, 1)
	c.Assert(auditLog[0].GetString("state"), Equals, "APPROVED")
	c.Assert(auditLog[0].GetString("delegator"), Equals, "mattermost:"+s.mmUser.Email)
}

func (s *MattermostSuite) TestDenial(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))

	response := s.postWebhookAndCheck(c, post, "Deny")
	c.Assert(response.EphemeralText, Equals, fmt.Sprintf("You have **denied** the request %s", request.GetName()))
	c.Assert(response.Update, NotNil)
	attachments := response.Update.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)

	auditLog, err := s.teleport.FilterAuditEvents("", events.EventFields{"event": events.AccessRequestUpdateEvent, "id": request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(auditLog, HasLen, 1)
	c.Assert(auditLog[0].GetString("state"), Equals, "DENIED")
	c.Assert(auditLog[0].GetString("delegator"), Equals, "mattermost:"+s.mmUser.Email)
}

func (s *MattermostSuite) TestExpiration(c *C) {
	s.startApp(c)
	s.createExpiredAccessRequest(c)

	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))
	postID := post.ID

	post, err = s.fakeMattermost.CheckPostUpdate(s.ctx)
	c.Assert(err, IsNil, Commentf("no messages updated"))
	c.Assert(post.ID, Equals, postID)
	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)
}

func (s *MattermostSuite) TestApproveExpired(c *C) {
	s.startApp(c)
	req := s.createExpiredAccessRequest(c)

	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))

	response := s.postWebhookAndCheck(c, post, "Approve")
	c.Assert(response.EphemeralText, Equals, fmt.Sprintf(`Request %s had been **expired**`, req.GetName()))
	c.Assert(response.Update, NotNil)
	attachments := response.Update.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)
}

func (s *MattermostSuite) TestDenyExpired(c *C) {
	s.startApp(c)
	req := s.createExpiredAccessRequest(c)

	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))

	response := s.postWebhookAndCheck(c, post, "Deny")
	c.Assert(response.EphemeralText, Equals, fmt.Sprintf(`Request %s had been **expired**`, req.GetName()))
	c.Assert(response.Update, NotNil)
	attachments := response.Update.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)
}

func (s *MattermostSuite) TestRace(c *C) {
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

	process := lib.NewProcess(s.ctx)
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			_, err := s.teleport.CreateAccessRequest(ctx, s.me.Username, "admin")
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			post, err := s.fakeMattermost.CheckNewPost(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			attachments := post.Attachments()
			if obtained, expected := len(attachments), 1; obtained != expected {
				return setRaceErr(trace.Errorf("wrong attachments size. expected %v, obtained %v", expected, obtained))
			}
			attachment := attachments[0]
			if obtained, expected := len(attachment.Actions), 2; obtained != expected {
				return setRaceErr(trace.Errorf("wrong attachment actions size. expected %v, obtained %v", expected, obtained))
			}
			if obtained, expected := attachment.Actions[0].Name, "Approve"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong attachment action. expected %q, obtained %q", expected, obtained))
			}
			if obtained, expected := attachment.Actions[1].Name, "Deny"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong attachment action. expected %q, obtained %q", expected, obtained))
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			var lastErr error
			for {
				log.Infof("Trying to approve post %q", post.ID)
				resp, err := s.postWebhook(ctx, post, "Approve")
				if err != nil {
					if lib.IsDeadline(err) {
						return setRaceErr(lastErr)
					}
					return setRaceErr(trace.Wrap(err))
				}
				if status := resp.StatusCode; status != http.StatusOK {
					if err := resp.Body.Close(); err != nil {
						return setRaceErr(trace.Wrap(err))
					}
					lastErr = trace.Errorf("got %v http code from webhook server", status)
					continue
				}
				var actionResponse ActionResponse
				if err := json.NewDecoder(resp.Body).Decode(&actionResponse); err != nil {
					return setRaceErr(trace.Wrap(err))
				}
				if err := resp.Body.Close(); err != nil {
					return setRaceErr(trace.Wrap(err))
				}
				if !strings.HasPrefix(actionResponse.EphemeralText, "You have **approved** the request") {
					return setRaceErr(trace.Errorf(`action response contains wrong "ephemeral_text" field: %q`, actionResponse.EphemeralText))
				}
				update := actionResponse.Update
				if update == nil {
					return setRaceErr(trace.Errorf(`action response does not have an "update" field`))
				}
				updateAttachments := update.Attachments()
				if obtained, expected := len(updateAttachments), 1; obtained != expected {
					return setRaceErr(trace.Errorf("wrong attachments size. expected %v, obtained %v", expected, obtained))
				}
				if obtained, expected := len(updateAttachments[0].Actions), 0; obtained != expected {
					return setRaceErr(trace.Errorf("wrong attachment actions size. expected %v, obtained %v", expected, obtained))
				}

				return nil
			}
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
