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

	"github.com/gravitational/teleport-plugins/access/integration"
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
	ctx        context.Context
	cancel     context.CancelFunc
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

func (s *GitlabSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), time.Second)
	s.publicURL = ""
	dbFile := s.newTmpFile(c, "db.*")
	s.dbPath = dbFile.Name()
	dbFile.Close()

	s.fakeGitLab = NewFakeGitLab(projectID)
}

func (s *GitlabSuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakeGitLab.Close()
	s.cancel()
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

	authAddr, err := s.teleport.Process.AuthSSHAddr()
	c.Assert(err, IsNil)

	var conf Config
	conf.Teleport.AuthServer = authAddr.Addr
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

func (s *GitlabSuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
}

func (s *GitlabSuite) createAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *GitlabSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateExpiredAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *GitlabSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "gitlab", reqID, time.Millisecond*250)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *GitlabSuite) assertNewLabels(c *C, expected int) map[string]Label {
	newLabels := s.fakeGitLab.GetAllNewLabels()
	actual := len(newLabels)
	if actual > expected {
		c.Fatalf("expected %d labels but extra %d labels was stored", expected, actual-expected)
	} else if actual < expected {
		c.Fatalf("expected %d labels but %d labels are missing", expected, expected-actual)
	}
	return newLabels
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

	hook, err := s.fakeGitLab.CheckNewProjectHook(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new project hooks stored"))
	c.Assert(hook.URL, Equals, s.publicURL+gitlabWebhookPath)

	s.shutdownApp(c)

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
	hook := s.fakeGitLab.StoreProjectHook(ProjectHook{URL: s.publicURL + gitlabWebhookPath})

	s.startApp(c)
	s.shutdownApp(c)

	c.Assert(s.fakeGitLab.CheckNoNewProjectHooks(), Equals, true)

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
	existingID := s.fakeGitLab.StoreProjectHook(ProjectHook{URL: "http://fooo"}).ID

	s.openDB(c, func(db DB) error {
		return db.UpdateSettings(func(settings Settings) error {
			return settings.SetHookID(existingID)
		})
	})

	s.startApp(c)

	hook, err := s.fakeGitLab.CheckProjectHookUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no project hooks updated"))
	c.Assert(hook.ID, Equals, existingID)
	c.Assert(hook.URL, Equals, s.publicURL+gitlabWebhookPath)

	s.shutdownApp(c)

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

	newLabels := s.assertNewLabels(c, 4)
	c.Assert(newLabels["pending"].Name, Equals, "Teleport: Pending")
	c.Assert(newLabels["approved"].Name, Equals, "Teleport: Approved")
	c.Assert(newLabels["denied"].Name, Equals, "Teleport: Denied")
	c.Assert(newLabels["expired"].Name, Equals, "Teleport: Expired")

	s.shutdownApp(c)

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
		"pending": s.fakeGitLab.StoreLabel(Label{Name: "teleport:pending"}),
		"expired": s.fakeGitLab.StoreLabel(Label{Name: "teleport:expired"}),
	}

	s.startApp(c)

	newLabels := s.assertNewLabels(c, 2)
	c.Assert(newLabels["approved"].Name, Equals, "Teleport: Approved")
	c.Assert(newLabels["denied"].Name, Equals, "Teleport: Denied")

	s.shutdownApp(c)

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

	issue, err := s.fakeGitLab.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issues stored"))
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")

	c.Assert(pluginData.ID, Equals, issue.ID)
	c.Assert(pluginData.IID, Equals, issue.IID)

	s.shutdownApp(c)

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

	labels := s.assertNewLabels(c, 4)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeGitLab.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issues stored"))
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")
	issueID := issue.ID

	oldIssue := issue
	issue.Labels = []Label{labels["approved"]}
	s.fakeGitLab.StoreIssue(issue)
	s.postIssueUpdateHook(c, oldIssue, issue)

	issue, err = s.fakeGitLab.CheckIssueUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no issues updated"))
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.State, Equals, "closed")

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *GitlabSuite) TestDenial(c *C) {
	s.startApp(c)

	labels := s.assertNewLabels(c, 4)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	issue, err := s.fakeGitLab.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issues stored"))
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")
	issueID := issue.ID

	oldIssue := issue
	issue.Labels = []Label{labels["denied"]}
	s.fakeGitLab.StoreIssue(issue)
	s.postIssueUpdateHook(c, oldIssue, issue)

	issue, err = s.fakeGitLab.CheckIssueUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no issues updated"))
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.State, Equals, "closed")

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *GitlabSuite) TestExpiration(c *C) {
	s.startApp(c)

	s.createExpiredAccessRequest(c)

	issue, err := s.fakeGitLab.CheckNewIssue(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new issues stored"))
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "pending")
	issueID := issue.ID

	issue, err = s.fakeGitLab.CheckIssueUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no issues updated"))
	c.Assert(issue.ID, Equals, issueID)
	c.Assert(issue.Labels, HasLen, 1)
	c.Assert(LabelName(issue.Labels[0].Name).Reduced(), Equals, "expired")
	c.Assert(issue.State, Equals, "closed")
}
