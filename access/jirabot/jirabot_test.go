package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"sync"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	jira "gopkg.in/andygrunwald/go-jira.v1"

	"github.com/gravitational/teleport/integration"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/utils"

	. "gopkg.in/check.v1"
)

const (
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"
)

type JirabotSuite struct {
	app         *App
	appPort     string
	webhookUrl  string
	me          *user.User
	fakeJira    *httprouter.Router
	fakeJiraSrv *httptest.Server
	issues      sync.Map
	newIssues   chan *jira.Issue
	teleport    *integration.TeleInstance
	tmpFiles    []*os.File
}

var _ = Suite(&JirabotSuite{})

func TestJirabot(t *testing.T) { TestingT(t) }

func (s *JirabotSuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	portList, err := utils.GetFreeTCPPorts(6)
	c.Assert(err, IsNil)
	ports := portList.PopIntSlice(5)
	t := integration.NewInstance(integration.InstanceConfig{ClusterName: Site, HostID: HostID, NodeName: Host, Ports: ports, Priv: priv, Pub: pub})

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

	err = t.Create(nil, true, nil)
	c.Assert(err, IsNil)
	if err := t.Start(); err != nil {
		c.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = t
	s.appPort = portList.Pop()
	s.webhookUrl = "http://" + Host + ":" + s.appPort + "/"
}

func (s *JirabotSuite) SetUpTest(c *C) {
	s.startFakeJira(c)
	s.startApp(c)
	time.Sleep(time.Millisecond * 250) // Wait some time for services to start up
}

func (s *JirabotSuite) TearDownTest(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*250)
	defer cancel()
	err := s.app.Shutdown(ctx)
	c.Assert(err, IsNil)
	s.fakeJiraSrv.Close()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *JirabotSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *JirabotSuite) startFakeJira(c *C) {
	s.fakeJira = httprouter.New()
	s.newIssues = make(chan *jira.Issue, 1)

	s.fakeJira.GET("/rest/api/2/field", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		field := jira.Field{
			Custom: true,
			Name:   RequestIdFieldName,
			Key:    "custom_123",
		}
		respBody, err := json.Marshal([]jira.Field{field})
		c.Assert(err, IsNil)

		rw.WriteHeader(http.StatusOK)
		_, err = rw.Write(respBody)
		c.Assert(err, IsNil)
	})
	s.fakeJira.POST("/rest/api/2/issue", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		body, err := ioutil.ReadAll(r.Body)
		c.Assert(err, IsNil)

		issue := &jira.Issue{}
		err = json.Unmarshal(body, issue)
		c.Assert(err, IsNil)
		issue.ID = fmt.Sprintf("%v", time.Now().UnixNano())
		issue.Key = "ISSUE-" + issue.ID
		if issue.Fields == nil {
			issue.Fields = &jira.IssueFields{}
		}
		issue.Fields.Status = &jira.Status{Name: "Pending"}
		s.putIssue(*issue)
		s.newIssues <- issue

		respBody, err := json.Marshal(issue)
		c.Assert(err, IsNil)

		rw.WriteHeader(http.StatusCreated)
		_, err = rw.Write(respBody)
		c.Assert(err, IsNil)
	})
	s.fakeJira.GET("/rest/api/2/issue/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		issue := s.getIssue(ps.ByName("id"))
		if issue == nil {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		respBody, err := json.Marshal(issue)
		c.Assert(err, IsNil)

		rw.WriteHeader(http.StatusOK)
		_, err = rw.Write(respBody)
		c.Assert(err, IsNil)
	})

	s.fakeJiraSrv = httptest.NewServer(s.fakeJira)
}

func (s *JirabotSuite) startApp(c *C) {
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

	var conf Config
	conf.Teleport.AuthServer = s.teleport.Config.Auth.SSHAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.JIRA.URL = s.fakeJiraSrv.URL
	conf.JIRA.Username = "bot@example.com"
	conf.JIRA.APIToken = "xyz"
	conf.JIRA.Project = "PROJ"
	conf.HTTP.Listen = ":" + s.appPort
	conf.HTTP.Insecure = true

	s.app, err = NewApp(conf)
	c.Assert(err, IsNil)

	go func() {
		err = s.app.Run(context.TODO())
		c.Assert(err, IsNil)
	}()
}

func (s *JirabotSuite) createAccessRequest(c *C) services.AccessRequest {
	client, err := s.teleport.NewClient(integration.ClientConfig{Login: s.me.Username})
	c.Assert(err, IsNil)
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	err = client.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)
	time.Sleep(time.Millisecond * 250) // Wait some time for watcher to receive a request
	return req
}

func (s *JirabotSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	client, err := s.teleport.NewClient(integration.ClientConfig{Login: s.me.Username})
	c.Assert(err, IsNil)
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	ttl := time.Millisecond * 250
	req.SetAccessExpiry(time.Now().Add(ttl))
	err = client.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)
	time.Sleep(ttl + time.Millisecond*50)
	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: req.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 0)
	return req
}

func (s *JirabotSuite) putIssue(issue jira.Issue) {
	s.issues.Store(issue.ID, issue)
	s.issues.Store(issue.Key, issue)
}

func (s *JirabotSuite) getIssue(idOrKey string) *jira.Issue {
	if obj, ok := s.issues.Load(idOrKey); ok {
		issue := obj.(jira.Issue)
		return &issue
	} else {
		return nil
	}
}

func (s *JirabotSuite) transitionIssue(c *C, id string, status string) {
	issue := s.getIssue(id)
	c.Assert(issue, NotNil)

	if issue.Fields == nil {
		issue.Fields = &jira.IssueFields{}
	} else {
		copy := *issue.Fields
		issue.Fields = &copy
	}
	issue.Fields.Status = &jira.Status{Name: status}
	if issue.Changelog == nil {
		issue.Changelog = &jira.Changelog{}
	} else {
		copy := *issue.Changelog
		issue.Changelog = &copy
	}

	history := jira.ChangelogHistory{
		Author: jira.User{
			Name:         "Robert Smith",
			EmailAddress: "robert@example.com",
		},
		Items: []jira.ChangelogItems{
			jira.ChangelogItems{
				FieldType: "jira",
				Field:     "status",
				ToString:  status,
			},
		},
	}
	issue.Changelog.Histories = append([]jira.ChangelogHistory{history}, issue.Changelog.Histories...)
	s.putIssue(*issue)

	response, err := s.postWebhook(c, &Webhook{
		WebhookEvent:       "jira:issue_updated",
		IssueEventTypeName: "issue_generic",
		Issue:              &WebhookIssue{ID: issue.ID},
	})
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)
}

func (s *JirabotSuite) postWebhook(c *C, wh *Webhook) (*http.Response, error) {
	body, err := json.Marshal(wh)
	c.Assert(err, IsNil)

	req, err := http.NewRequest("POST", s.webhookUrl, bytes.NewReader(body))
	c.Assert(err, IsNil)

	req.Header.Add("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func (s *JirabotSuite) TestSlackMessagePosting(c *C) {
	request := s.createAccessRequest(c)

	var issue *jira.Issue
	select {
	case issue = <-s.newIssues:
	case <-time.After(time.Millisecond * 250):
		c.Fatal("issue wasn't created")
	}

	c.Assert(issue.Fields.Unknowns["custom_123"], Equals, request.GetName())
}

func (s *JirabotSuite) TestApproval(c *C) {
	request := s.createAccessRequest(c)

	var issue *jira.Issue
	select {
	case issue = <-s.newIssues:
	case <-time.After(time.Millisecond * 250):
		c.Fatal("issue wasn't created")
	}

	s.transitionIssue(c, issue.ID, "Approved")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *JirabotSuite) TestDenial(c *C) {
	request := s.createAccessRequest(c)

	var issue *jira.Issue
	select {
	case issue = <-s.newIssues:
	case <-time.After(time.Millisecond * 250):
		c.Fatal("issue wasn't created")
	}

	s.transitionIssue(c, issue.ID, "Denied")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}
