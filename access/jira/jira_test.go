package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/testing/integration"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	. "gopkg.in/check.v1"
)

type JiraSuite struct {
	ctx        context.Context
	cancel     context.CancelFunc
	app        *App
	publicURL  string
	raceNumber int
	me         *user.User
	authorUser UserDetails
	otherUser  UserDetails
	fakeJira   *FakeJIRA
	tmpFiles   []*os.File

	teleportI      *integration.Integration
	teleport       *integration.API
	teleportConfig lib.TeleportConfig
}

var _ = Suite(&JiraSuite{})

func TestJirabot(t *testing.T) { TestingT(t) }

func (s *JiraSuite) SetUpSuite(c *C) {
	var err error

	ctx := context.Background()

	log.SetLevel(log.DebugLevel)
	s.raceNumber = runtime.GOMAXPROCS(0)
	s.me, err = user.Current()
	c.Assert(err, IsNil)

	s.authorUser = UserDetails{AccountID: "USER-1", DisplayName: s.me.Username, EmailAddress: s.me.Username + "@example.com"}
	s.otherUser = UserDetails{AccountID: "USER-2", DisplayName: s.me.Username + " evil twin", EmailAddress: s.me.Username + ".evil@example.com"}

	teleport, err := integration.NewFromEnv(ctx)
	c.Assert(err, IsNil)

	auth, err := teleport.NewAuthServer()
	c.Assert(err, IsNil)
	go func() {
		if err := auth.Run(ctx); err != nil {
			panic(err)
		}
	}()
	ok, err := auth.WaitReady(ctx)
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)

	api, err := teleport.NewAPI(ctx, auth)
	c.Assert(err, IsNil)

	var bootstrap integration.Bootstrap

	userRole, err := bootstrap.AddRole("foo", types.RoleSpecV4{
		Allow: types.RoleConditions{
			Logins:  []string{s.me.Username}, // cannot be empty
			Request: &types.AccessRequestConditions{Roles: []string{"admin"}},
		},
	})
	c.Assert(err, IsNil)
	_, err = bootstrap.AddUserWithRoles(s.me.Username, userRole.GetName())
	c.Assert(err, IsNil)

	accessPluginRole, err := bootstrap.AddRole("access-plugin", types.RoleSpecV4{
		Allow: types.RoleConditions{
			Logins: []string{"access-plugin"}, // cannot be empty
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read", "update"}),
			},
		},
	})
	c.Assert(err, IsNil)
	accessPluginUser, err := bootstrap.AddUserWithRoles("plugin", accessPluginRole.GetName())
	c.Assert(err, IsNil)

	err = teleport.Bootstrap(ctx, auth, bootstrap.Resources())
	c.Assert(err, IsNil)

	identityPath, err := teleport.Sign(ctx, auth, accessPluginUser.GetName())
	c.Assert(err, IsNil)

	s.teleportI = teleport
	s.teleport = api
	s.teleportConfig.Addr = auth.PublicAddr()
	s.teleportConfig.Identity = identityPath
}

func (s *JiraSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Second)
	s.publicURL = ""
	s.fakeJira = NewFakeJIRA(s.authorUser, s.raceNumber)
}

func (s *JiraSuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakeJira.Close()
	s.cancel()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *JiraSuite) TearDownSuite(c *C) {
	if s.teleportI != nil {
		s.teleportI.Close()
	}
}

func (s *JiraSuite) startApp(c *C) {
	var conf Config
	conf.Teleport = s.teleportConfig
	conf.JIRA.URL = s.fakeJira.URL()
	conf.JIRA.Username = "bot@example.com"
	conf.JIRA.APIToken = "xyz"
	conf.JIRA.Project = "PROJ"
	conf.HTTP.ListenAddr = ":0"
	if s.publicURL != "" {
		conf.HTTP.PublicAddr = s.publicURL
	}
	conf.HTTP.Insecure = true

	app, err := NewApp(conf)
	c.Assert(err, IsNil)
	go func() {
		if err := app.Run(s.ctx); err != nil {
			panic(err)
		}
	}()
	ok, err := app.WaitReady(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)
	if s.publicURL == "" {
		s.publicURL = app.PublicURL().String()
	}
	s.app = app
}

func (s *JiraSuite) shutdownApp(c *C) {
	if s.app == nil {
		return
	}
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(s.app.Err(), IsNil)
}

func (s *JiraSuite) newAccessRequest(c *C) types.AccessRequest {
	req, err := types.NewAccessRequest(uuid.New().String(), s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *JiraSuite) createAccessRequest(c *C) types.AccessRequest {
	req := s.newAccessRequest(c)
	err := s.teleport.CreateAccessRequest(s.ctx, req)
	c.Assert(err, IsNil)
	return req
}

func (s *JiraSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "jira", reqID)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *JiraSuite) postWebhook(ctx context.Context, issueID string) (*http.Response, error) {
	var buf bytes.Buffer
	wh := Webhook{
		WebhookEvent:       "jira:issue_updated",
		IssueEventTypeName: "issue_generic",
		Issue:              &WebhookIssue{ID: issueID},
	}
	err := json.NewEncoder(&buf).Encode(&wh)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.publicURL, &buf)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	request.Header.Add("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	return response, trace.Wrap(err)
}

func (s *JiraSuite) postWebhookAndCheck(c *C, issueID string) {
	resp, err := s.postWebhook(s.ctx, issueID)
	c.Assert(err, IsNil)
	c.Assert(resp.Body.Close(), IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
}

func (s *JiraSuite) TestIssueCreation(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	pluginData := s.checkPluginData(c, request.GetName())

	issue, err := s.fakeJira.CheckNewIssue(s.ctx)
	c.Assert(issue.Fields.Project.Key, Equals, "PROJ")
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	c.Assert(issue.Properties[RequestIDPropertyKey], Equals, request.GetName())
	c.Assert(pluginData.ID, Equals, issue.ID)
}

func (s *JiraSuite) TestIssueCreationWithRequestReason(c *C) {
	s.startApp(c)
	req := s.newAccessRequest(c)
	req.SetRequestReason("because of")
	err := s.teleport.CreateAccessRequest(s.ctx, req)
	c.Assert(err, IsNil)
	s.checkPluginData(c, req.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.ctx)
	c.Assert(err, IsNil)

	if !strings.Contains(issue.Fields.Description, `Reason: *because of*`) {
		c.Error("Issue description should contain request reason")
	}
}

func (s *JiraSuite) TestApproval(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.ctx)
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	s.fakeJira.TransitionIssue(issue, "Approved")
	s.postWebhookAndCheck(c, issue.ID)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, types.RequestState_APPROVED)

	events, err := s.teleport.SearchAccessRequestEvents(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(events, HasLen, 1)
	c.Assert(events[0].RequestState, Equals, "APPROVED")
	c.Assert(events[0].Delegator, Equals, "jira:"+s.authorUser.EmailAddress)
}

func (s *JiraSuite) TestDenial(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.ctx)
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	s.fakeJira.TransitionIssue(issue, "Denied")
	s.postWebhookAndCheck(c, issue.ID)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, types.RequestState_DENIED)

	events, err := s.teleport.SearchAccessRequestEvents(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(events, HasLen, 1)
	c.Assert(events[0].RequestState, Equals, "DENIED")
	c.Assert(events[0].Delegator, Equals, "jira:"+s.authorUser.EmailAddress)
}

func (s *JiraSuite) TestApprovalWithReason(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.ctx)
	c.Assert(err, IsNil, Commentf("no new issue stored"))

	issue = s.fakeJira.StoreIssueComment(issue, Comment{
		Author: s.authorUser,
		Body:   "hi! i'm going to approve this request.\nReason:\n\nfoo\nbar\nbaz",
	})

	s.fakeJira.TransitionIssue(issue, "Approved")
	s.postWebhookAndCheck(c, issue.ID)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, types.RequestState_APPROVED)
	c.Assert(request.GetResolveReason(), Equals, "foo\nbar\nbaz")

	events, err := s.teleport.SearchAccessRequestEvents(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(events, HasLen, 1)
	c.Assert(events[0].RequestState, Equals, "APPROVED")
	c.Assert(events[0].Delegator, Equals, "jira:"+s.authorUser.EmailAddress)
}

func (s *JiraSuite) TestDenialWithReason(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.ctx)
	c.Assert(err, IsNil, Commentf("no new issue stored"))

	issue = s.fakeJira.StoreIssueComment(issue, Comment{
		Author: s.otherUser,
		Body:   "comment 1", // just ignored.
	})
	issue = s.fakeJira.StoreIssueComment(issue, Comment{
		Author: s.authorUser,
		Body:   "hi! i'm rejecting the request.\nreason: bar baz", // reason is "bar baz" but the next comment will override it.
	})
	issue = s.fakeJira.StoreIssueComment(issue, Comment{
		Author: s.authorUser,
		Body:   "hi! i'm rejecting the request.\nreason: foo bar baz", // reason is "foo bar baz".
	})
	issue = s.fakeJira.StoreIssueComment(issue, Comment{
		Author: s.otherUser,
		Body:   "reason: test", // has reason too but ignored because it's not the same user that did transition.
	})

	s.fakeJira.TransitionIssue(issue, "Denied")
	s.postWebhookAndCheck(c, issue.ID)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, types.RequestState_DENIED)
	c.Assert(request.GetResolveReason(), Equals, "foo bar baz")

	events, err := s.teleport.SearchAccessRequestEvents(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(events, HasLen, 1)
	c.Assert(events[0].RequestState, Equals, "DENIED")
	c.Assert(events[0].Delegator, Equals, "jira:"+s.authorUser.EmailAddress)
}

func (s *JiraSuite) TestExpiration(c *C) {
	s.startApp(c)
	req := s.createAccessRequest(c)

	issue, err := s.fakeJira.CheckNewIssue(s.ctx)
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	issueID := issue.ID
	s.checkPluginData(c, req.GetName())

	err = s.teleport.DeleteAccessRequest(s.ctx, req.GetName())
	c.Assert(err, IsNil)

	issue, err = s.fakeJira.CheckIssueTransition(s.ctx)
	c.Assert(err, IsNil, Commentf("no issue transition detected"))
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.Fields.Status.Name, Equals, "Expired")
}

func (s *JiraSuite) TestRace(c *C) {
	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	s.cancel() // Cancel the default timeout
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 20*time.Second)
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

	watcher, err := s.teleport.NewWatcher(s.ctx, types.Watch{
		Kinds: []types.WatchKind{
			{
				Kind: types.KindAccessRequest,
			},
		},
	})
	c.Assert(err, IsNil)
	defer watcher.Close()
	c.Assert((<-watcher.Events()).Type, Equals, types.OpInit)

	process := lib.NewProcess(s.ctx)
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			req, err := types.NewAccessRequest(uuid.New().String(), s.me.Username, "admin")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			if err = s.teleport.CreateAccessRequest(s.ctx, req); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			issue, err := s.fakeJira.CheckNewIssue(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			if obtained, expected := issue.Fields.Status.Name, "Pending"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong issue status. expected %q, obtained %q", expected, obtained))
			}
			s.fakeJira.TransitionIssue(issue, "Approved")

			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			var lastErr error
			for {
				log.Infof("Trying to approve issue %q", issue.Key)
				resp, err := s.postWebhook(ctx, issue.ID)
				if err != nil {
					if lib.IsDeadline(err) {
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
			issue, err := s.fakeJira.CheckIssueTransition(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			if obtained, expected := issue.Fields.Status.Name, "Approved"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong issue status. expected %q, obtained %q", expected, obtained))
			}
			return nil
		})
	}
	for i := 0; i < 2*s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			var event types.Event
			select {
			case event = <-watcher.Events():
			case <-ctx.Done():
				return setRaceErr(trace.Wrap(ctx.Err()))
			}
			if obtained, expected := event.Type, types.OpPut; obtained != expected {
				return setRaceErr(trace.Errorf("wrong event type. expected %v, obtained %v", expected, obtained))
			}
			req := event.Resource.(types.AccessRequest)
			var newCounter int64
			val, _ := requests.LoadOrStore(req.GetName(), &newCounter)
			switch state := req.GetState(); state {
			case types.RequestState_PENDING:
				atomic.AddInt64(val.(*int64), 1)
			case types.RequestState_APPROVED:
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
