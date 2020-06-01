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
	"reflect"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/utils/nettest"
	"github.com/gravitational/teleport/integration"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/services"

	. "gopkg.in/check.v1"
)

const (
	Host          = "localhost"
	HostID        = "00000000-0000-0000-0000-000000000000"
	Site          = "local-site"
	WebhookSecret = "0000"
	projectID     = IntID(1111)
)

type GitlabSuite struct {
	app        *App
	publicURL  string
	me         *user.User
	tmpFiles   []*os.File
	dbPath     string
	fakeGitLab *FakeGitlab

	teleport *integration.TeleInstance
}

var _ = Suite(&GitlabSuite{})

func TestGitlab(t *testing.T) { TestingT(t) }

func (s *GitlabSuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	portList, err := nettest.GetFreeTCPPortsForTests(6)
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
}

func (s *GitlabSuite) SetUpTest(c *C) {
	s.publicURL = ""
	dbFile := s.newTmpFile(c, "db.*")
	s.dbPath = dbFile.Name()
	dbFile.Close()

	s.fakeGitLab = NewFakeGitLab(c, projectID)
}

func (s *GitlabSuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakeGitLab.Close()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *GitlabSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *GitlabSuite) startApp(c *C) {
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
	conf.Gitlab.URL = s.fakeGitLab.URL()
	conf.Gitlab.WebhookSecret = WebhookSecret
	conf.Gitlab.ProjectID = fmt.Sprintf("%d", projectID)
	conf.DB.Path = s.dbPath
	if s.publicURL != "" {
		conf.HTTP.PublicAddr = s.publicURL
	}
	conf.HTTP.ListenAddr = ":0"
	conf.HTTP.Insecure = true

	s.app, err = NewApp(conf)
	c.Assert(err, IsNil)

	go func() {
		err = s.app.Run(context.TODO())
		c.Assert(err, IsNil)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*250)
	defer cancel()
	ok, err := s.app.WaitReady(ctx)
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)
	if s.publicURL == "" {
		s.publicURL = s.app.PublicURL().String()
	}
}

func (s *GitlabSuite) shutdownApp(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*2000)
	defer cancel()
	err := s.app.Shutdown(ctx)
	c.Assert(err, IsNil)
}

func (s *GitlabSuite) createAccessRequest(c *C) services.AccessRequest {
	auth := s.teleport.Process.GetAuthServer()
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	err = auth.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)
	return req
}

func (s *GitlabSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	auth := s.teleport.Process.GetAuthServer()
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	ttl := time.Millisecond * 250
	req.SetAccessExpiry(time.Now().Add(ttl))
	err = auth.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)

	time.Sleep(ttl)
	ctx, cancel := context.WithTimeout(context.Background(), ttl)
	defer cancel()
	for {
		requests, err := auth.GetAccessRequests(ctx, services.AccessRequestFilter{ID: req.GetName()})
		c.Assert(err, IsNil)
		if len(requests) == 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	return req
}

func (s *GitlabSuite) checkPluginData(c *C, reqID string) PluginData {
	timeout := time.After(time.Millisecond * 250)
	ticker := time.NewTicker(time.Millisecond * 25)
	defer ticker.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			data, err := s.app.GetPluginData(ctx, reqID)
			c.Assert(err, IsNil)
			if data.ID != 0 {
				return data
			}
		case <-timeout:
			c.Fatal("no plugin data saved")
			return PluginData{}
		}
	}
}

func (s *GitlabSuite) postIssueUpdateHook(c *C, oldIssue, newIssue Issue) {
	var labelsChange *LabelsChange
	if !reflect.DeepEqual(oldIssue.Labels, newIssue.Labels) {
		labelsChange = &LabelsChange{Previous: oldIssue.Labels, Current: newIssue.Labels}
	}
	payload := IssueEvent{
		Project: Project{ID: projectID},
		ObjectAttributes: IssueObjectAttributes{
			Action: "update",
			Issue:  oldIssue,
		},
		Changes: IssueChanges{
			Labels: labelsChange,
		},
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&payload)
	c.Assert(err, IsNil)
	req, err := http.NewRequest("POST", s.publicURL+gitlabWebhookPath, &buf)
	c.Assert(err, IsNil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Gitlab-Token", WebhookSecret)
	req.Header.Add("X-Gitlab-Event", "Issue Hook")
	response, err := http.DefaultClient.Do(req)
	response.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)
}

func (s *GitlabSuite) openDB(c *C, fn func(db DB) error) {
	db, err := OpenDB(s.dbPath, projectID)
	c.Assert(err, IsNil)
	defer func() {
		if err := db.Close(); err != nil {
			panic(err)
		}
	}()
	c.Assert(fn(db), IsNil)
}

func (s *GitlabSuite) TestProjectHookSetup(c *C) {
	s.startApp(c)
	s.shutdownApp(c)

	hook := s.fakeGitLab.checkNewProjectHook(c)
	c.Assert(hook.URL, Equals, s.publicURL+gitlabWebhookPath)

	var dbHookID IntID
	s.openDB(c, func(db DB) error {
		return db.ViewSettings(func(settings Settings) error {
			dbHookID = settings.HookID()
			return nil
		})
	})
	c.Assert(dbHookID, Equals, hook.ID)
}

func (s *GitlabSuite) TestProjectHookSetupWhenItExists(c *C) {
	s.publicURL = "http://teleport-gitlab.local"
	hook := s.fakeGitLab.storeProjectHook(ProjectHook{URL: s.publicURL + gitlabWebhookPath})
	s.startApp(c)
	s.shutdownApp(c)

	s.fakeGitLab.checkNoNewProjectHooks(c)

	var dbHookID IntID
	s.openDB(c, func(db DB) error {
		return db.ViewSettings(func(settings Settings) error {
			dbHookID = settings.HookID()
			return nil
		})
	})
	c.Assert(dbHookID, Equals, hook.ID)
}

func (s *GitlabSuite) TestProjectHookSetupWhenItExistsInDB(c *C) {
	existingID := s.fakeGitLab.storeProjectHook(ProjectHook{URL: "http://fooo"}).ID

	s.openDB(c, func(db DB) error {
		return db.UpdateSettings(func(settings Settings) error {
			return settings.SetHookID(existingID)
		})
	})

	s.startApp(c)
	s.shutdownApp(c)

	hook := s.fakeGitLab.checkProjectHookUpdate(c)
	c.Assert(hook.ID, Equals, existingID)
	c.Assert(hook.URL, Equals, s.publicURL+gitlabWebhookPath)

	var dbHookID IntID
	s.openDB(c, func(db DB) error {
		return db.ViewSettings(func(settings Settings) error {
			dbHookID = settings.HookID()
			return nil
		})
	})
	c.Assert(dbHookID, Equals, existingID)
}

func (s *GitlabSuite) TestLabelsSetup(c *C) {
	s.startApp(c)
	s.shutdownApp(c)

	newLabels := s.fakeGitLab.checkNewLabels(c, 4)
	c.Assert(newLabels["pending"].Name, Equals, "Teleport: Pending")
	c.Assert(newLabels["approved"].Name, Equals, "Teleport: Approved")
	c.Assert(newLabels["denied"].Name, Equals, "Teleport: Denied")
	c.Assert(newLabels["expired"].Name, Equals, "Teleport: Expired")

	var dbLabels map[string]string
	s.openDB(c, func(db DB) error {
		return db.ViewSettings(func(settings Settings) error {
			dbLabels = settings.GetLabels("pending", "approved", "denied", "expired")
			return nil
		})
	})
	c.Assert(dbLabels["pending"], Equals, newLabels["pending"].Name)
	c.Assert(dbLabels["approved"], Equals, newLabels["approved"].Name)
	c.Assert(dbLabels["denied"], Equals, newLabels["denied"].Name)
	c.Assert(dbLabels["expired"], Equals, newLabels["expired"].Name)
}

func (s *GitlabSuite) TestLabelsSetupWhenSomeExist(c *C) {
	labels := map[string]Label{
		"pending": s.fakeGitLab.storeLabel(Label{Name: "teleport:pending"}),
		"expired": s.fakeGitLab.storeLabel(Label{Name: "teleport:expired"}),
	}

	s.startApp(c)
	s.shutdownApp(c)

	newLabels := s.fakeGitLab.checkNewLabels(c, 2)
	c.Assert(newLabels["approved"].Name, Equals, "Teleport: Approved")
	c.Assert(newLabels["denied"].Name, Equals, "Teleport: Denied")

	var dbLabels map[string]string
	s.openDB(c, func(db DB) error {
		return db.ViewSettings(func(settings Settings) error {
			dbLabels = settings.GetLabels("pending", "approved", "denied", "expired")
			return nil
		})
	})

	c.Assert(dbLabels["pending"], Equals, labels["pending"].Name)
	c.Assert(dbLabels["approved"], Equals, newLabels["approved"].Name)
	c.Assert(dbLabels["denied"], Equals, newLabels["denied"].Name)
	c.Assert(dbLabels["expired"], Equals, labels["expired"].Name)
}

func (s *GitlabSuite) TestIssueCreation(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	pluginData := s.checkPluginData(c, request.GetName())
	s.shutdownApp(c)

	issue := s.fakeGitLab.checkNewIssue(c)
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")

	c.Assert(pluginData.ID, Equals, issue.ID)
	c.Assert(pluginData.IID, Equals, issue.IID)

	var reqID string
	s.openDB(c, func(db DB) error {
		return db.ViewIssues(func(issues Issues) error {
			reqID = issues.GetRequestID(issue.ID)
			return nil
		})
	})

	c.Assert(reqID, Equals, request.GetName())
}

func (s *GitlabSuite) TestApproval(c *C) {
	s.startApp(c)
	labels := s.fakeGitLab.checkNewLabels(c, 4)
	request := s.createAccessRequest(c)
	_ = s.checkPluginData(c, request.GetName()) // when plugin data created, the request must be already served.

	issue := s.fakeGitLab.checkNewIssue(c)
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")
	issueID := issue.ID

	oldIssue := issue
	issue.Labels = []Label{labels["approved"]}
	s.fakeGitLab.storeIssue(issue)
	s.postIssueUpdateHook(c, oldIssue, issue)

	issue = s.fakeGitLab.checkIssueUpdate(c)
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.State, Equals, "closed")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *GitlabSuite) TestDenial(c *C) {
	s.startApp(c)
	labels := s.fakeGitLab.checkNewLabels(c, 4)
	request := s.createAccessRequest(c)
	_ = s.checkPluginData(c, request.GetName()) // when plugin data created, the request must be already served.

	issue := s.fakeGitLab.checkNewIssue(c)
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")
	issueID := issue.ID

	oldIssue := issue
	issue.Labels = []Label{labels["denied"]}
	s.fakeGitLab.storeIssue(issue)
	s.postIssueUpdateHook(c, oldIssue, issue)

	issue = s.fakeGitLab.checkIssueUpdate(c)
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.State, Equals, "closed")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *GitlabSuite) TestExpiration(c *C) {
	s.startApp(c)
	_ = s.createExpiredAccessRequest(c)

	issue := s.fakeGitLab.checkNewIssue(c)
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")
	issueID := issue.ID

	issue = s.fakeGitLab.checkIssueUpdate(c)
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "expired")
	c.Assert(issue.State, Equals, "closed")
}
