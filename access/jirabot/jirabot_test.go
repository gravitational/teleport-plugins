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
	fakeJiraSrv *httptest.Server
	issues      sync.Map
	newIssues   chan *Issue
	transitions chan *Issue
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
	portList, err := utils.GetFreeTCPPorts(6, 100)
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
	close(s.newIssues)
	close(s.transitions)
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
	s.newIssues = make(chan *Issue, 1)
	s.transitions = make(chan *Issue, 1)

	fakeJira := httprouter.New()
	fakeJira.GET("/rest/api/2/myself", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
	})
	fakeJira.GET("/rest/api/2/project/PROJ", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		project := jira.Project{
			Key:  "PROJ",
			Name: "The Project",
		}
		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(&project)
		c.Assert(err, IsNil)
	})
	fakeJira.GET("/rest/api/2/mypermissions", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		permissions := Permissions{
			Permissions: map[string]Permission{
				"BROWSE_PROJECTS": Permission{
					HavePermission: true,
				},
				"CREATE_ISSUES": Permission{
					HavePermission: true,
				},
			},
		}
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		err := json.NewEncoder(rw).Encode(&permissions)
		c.Assert(err, IsNil)
	})
	fakeJira.POST("/rest/api/2/issue", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var issueInput IssueInput

		err := json.NewDecoder(r.Body).Decode(&issueInput)
		c.Assert(err, IsNil)

		id := fmt.Sprintf("%v", time.Now().UnixNano())
		issue := &Issue{
			ID:         id,
			Key:        "ISSUE-" + id,
			Fields:     issueInput.Fields,
			Properties: make(map[string]interface{}),
		}
		for _, property := range issueInput.Properties {
			issue.Properties[property.Key] = property.Value
		}
		if issue.Fields == nil {
			issue.Fields = &jira.IssueFields{}
		}
		issue.Fields.Status = &jira.Status{Name: "Pending"}
		issue.Transitions = []jira.Transition{
			jira.Transition{
				ID: "100001", To: jira.Status{Name: "Approved"},
			},
			jira.Transition{
				ID: "100002", To: jira.Status{Name: "Denied"},
			},
			jira.Transition{
				ID: "100003", To: jira.Status{Name: "Expired"},
			},
		}
		s.putIssue(*issue)
		s.newIssues <- issue

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(issue)
		c.Assert(err, IsNil)
	})
	fakeJira.GET("/rest/api/2/issue/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		issue := s.getIssue(ps.ByName("id"))
		if issue == nil {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		err := json.NewEncoder(rw).Encode(issue)
		c.Assert(err, IsNil)
	})
	fakeJira.POST("/rest/api/2/issue/:id/transitions", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		issue := s.getIssue(ps.ByName("id"))
		if issue == nil {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		var payload jira.CreateTransitionPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		c.Assert(err, IsNil)

		switch payload.Transition.ID {
		case "100001":
			s.transitionIssue(c, issue, "Approved")
		case "100002":
			s.transitionIssue(c, issue, "Denied")
		case "100003":
			s.transitionIssue(c, issue, "Expired")
		default:
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusNoContent)
	})

	s.fakeJiraSrv = httptest.NewServer(fakeJira)
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

func (s *JirabotSuite) putIssue(issue Issue) {
	s.issues.Store(issue.ID, issue)
	s.issues.Store(issue.Key, issue)
}

func (s *JirabotSuite) getIssue(idOrKey string) *Issue {
	if obj, ok := s.issues.Load(idOrKey); ok {
		issue := obj.(Issue)
		return &issue
	} else {
		return nil
	}
}

func (s *JirabotSuite) transitionIssue(c *C, issue *Issue, status string) {
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
	s.transitions <- issue

	response, err := s.postWebhook(c, &Webhook{
		WebhookEvent:       "jira:issue_updated",
		IssueEventTypeName: "issue_generic",
		Issue:              &WebhookIssue{ID: issue.ID},
	})
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
}

func (s *JirabotSuite) postWebhook(c *C, wh *Webhook) (*http.Response, error) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(wh)
	c.Assert(err, IsNil)

	return http.Post(s.webhookUrl, "application/json", &buf)
}

func (s *JirabotSuite) TestSlackMessagePosting(c *C) {
	request := s.createAccessRequest(c)

	var issue *Issue
	select {
	case issue = <-s.newIssues:
		c.Assert(issue, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("issue wasn't created")
	}

	c.Assert(issue.Properties[RequestIdPropertyKey], Equals, request.GetName())
}

func (s *JirabotSuite) TestApproval(c *C) {
	request := s.createAccessRequest(c)

	var issue *Issue
	select {
	case issue = <-s.newIssues:
		c.Assert(issue, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("issue wasn't created")
	}

	s.transitionIssue(c, issue, "Approved")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *JirabotSuite) TestDenial(c *C) {
	request := s.createAccessRequest(c)

	var issue *Issue
	select {
	case issue = <-s.newIssues:
		c.Assert(issue, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("issue wasn't created")
	}

	s.transitionIssue(c, issue, "Denied")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *JirabotSuite) TestExpired(c *C) {
	_ = s.createExpiredAccessRequest(c)

	var issue *Issue
	select {
	case issue = <-s.transitions:
		c.Assert(issue, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("no issue transition detected")
	}
	c.Assert(issue.Fields.Status.Name, Equals, "Expired")
}
