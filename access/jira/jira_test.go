package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"testing"
	"time"

	"github.com/andygrunwald/go-jira"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/access/integration"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/services"

	. "gopkg.in/check.v1"
)

const (
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"
)

type JiraSuite struct {
	ctx       context.Context
	cancel    context.CancelFunc
	app       *App
	publicURL string
	me        *user.User
	fakeJira  *FakeJIRA
	teleport  *integration.TeleInstance
	tmpFiles  []*os.File
}

var _ = Suite(&JiraSuite{})

func TestJirabot(t *testing.T) { TestingT(t) }

func (s *JiraSuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	t := integration.NewInstance(integration.InstanceConfig{ClusterName: Site, HostID: HostID, NodeName: Host, Priv: priv, Pub: pub})

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

func (s *JiraSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Second)
	s.publicURL = ""
	s.fakeJira = NewFakeJIRA(jira.User{
		Name:         "Test User",
		EmailAddress: s.me.Username + "@example.com",
	})
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

func (s *JiraSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *JiraSuite) startApp(c *C) {
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
	conf.JIRA.URL = s.fakeJira.URL()
	conf.JIRA.Username = "bot@example.com"
	conf.JIRA.APIToken = "xyz"
	conf.JIRA.Project = "PROJ"
	conf.HTTP.ListenAddr = ":0"
	if s.publicURL != "" {
		conf.HTTP.PublicAddr = s.publicURL
	}
	conf.HTTP.Insecure = true

	s.app, err = NewApp(conf)
	c.Assert(err, IsNil)

	go func() {
		err = s.app.Run(s.ctx)
		c.Assert(err, IsNil)
	}()
	ctx, cancel := context.WithTimeout(s.ctx, time.Millisecond*250)
	defer cancel()
	ok, err := s.app.WaitReady(ctx)
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)
	if s.publicURL == "" {
		s.publicURL = s.app.PublicURL().String()
	}
}

func (s *JiraSuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
}

func (s *JiraSuite) createAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *JiraSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateExpiredAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *JiraSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "jira", reqID)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *JiraSuite) postWebhook(c *C, issueID string) {
	var buf bytes.Buffer
	wh := Webhook{
		WebhookEvent:       "jira:issue_updated",
		IssueEventTypeName: "issue_generic",
		Issue:              &WebhookIssue{ID: issueID},
	}
	err := json.NewEncoder(&buf).Encode(&wh)
	c.Assert(err, IsNil)
	resp, err := http.Post(s.publicURL, "application/json", &buf)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	err = resp.Body.Close()
	c.Assert(err, IsNil)
}

func (s *JiraSuite) TestIssueCreation(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	pluginData := s.checkPluginData(c, request.GetName())

	issue, err := s.fakeJira.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	c.Assert(issue.Properties[RequestIDPropertyKey], Equals, request.GetName())
	c.Assert(pluginData.ID, Equals, issue.ID)
}

func (s *JiraSuite) TestApproval(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	s.fakeJira.TransitionIssue(issue, "Approved")
	s.postWebhook(c, issue.ID)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)

	auditLog, err := s.teleport.FilterAuditEvents("", events.EventFields{"event": events.AccessRequestUpdateEvent, "id": request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(auditLog, HasLen, 1)
	c.Assert(auditLog[0].GetString("state"), Equals, "APPROVED")
	c.Assert(auditLog[0].GetString("delegator"), Equals, "jira:"+s.fakeJira.GetAuthor().EmailAddress)
}

func (s *JiraSuite) TestDenial(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	s.fakeJira.TransitionIssue(issue, "Denied")
	s.postWebhook(c, issue.ID)

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)

	auditLog, err := s.teleport.FilterAuditEvents("", events.EventFields{"event": events.AccessRequestUpdateEvent, "id": request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(auditLog, HasLen, 1)
	c.Assert(auditLog[0].GetString("state"), Equals, "DENIED")
	c.Assert(auditLog[0].GetString("delegator"), Equals, "jira:"+s.fakeJira.GetAuthor().EmailAddress)
}

func (s *JiraSuite) TestExpiration(c *C) {
	s.startApp(c)
	s.createExpiredAccessRequest(c)

	issue, err := s.fakeJira.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issue stored"))
	issueID := issue.ID
	issue, err = s.fakeJira.CheckIssueTransition(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no issue transition detected"))
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.Fields.Status.Name, Equals, "Expired")
}
