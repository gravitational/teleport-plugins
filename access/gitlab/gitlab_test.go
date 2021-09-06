/*
Copyright 2020-2021 Gravitational, Inc.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/user"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/access/integration"
	"github.com/gravitational/teleport-plugins/lib"
	. "github.com/gravitational/teleport-plugins/lib/testing"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	Host          = "localhost"
	HostID        = "00000000-0000-0000-0000-000000000000"
	Site          = "local-site"
	WebhookSecret = "0000"
	projectID     = IntID(1111)
)

type GitlabSuite struct {
	Suite
	appConfig Config
	userNames struct {
		plugin    string
		requestor string
		reviewer1 string
		reviewer2 string
	}
	approverEmail string
	publicURL     string
	raceNumber    int
	dbPath        string
	app           *App
	fakeGitlab    *FakeGitlab
	teleport      *integration.TeleInstance
}

func TestGitlab(t *testing.T) { suite.Run(t, &GitlabSuite{}) }

func (s *GitlabSuite) SetupSuite() {
	var err error
	t := s.T()
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	require.NoError(t, err)
	teleport := integration.NewInstance(integration.InstanceConfig{ClusterName: Site, HostID: HostID, NodeName: Host, Priv: priv, Pub: pub})

	s.raceNumber = runtime.GOMAXPROCS(0)
	me, err := user.Current()
	require.NoError(t, err)

	role, err := types.NewRole("foo", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{
				Roles: []string{"admin"},
				Thresholds: []types.AccessReviewThreshold{
					types.AccessReviewThreshold{Approve: 2, Deny: 2},
				},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.requestor = teleport.AddUserWithRole(me.Username+"@example.com", role).Username

	role, err = types.NewRole("foo-reviewer", types.RoleSpecV3{
		Allow: types.RoleConditions{
			ReviewRequests: &types.AccessReviewConditions{
				Roles: []string{"admin"},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.reviewer1 = teleport.AddUserWithRole(me.Username+"-reviewer1@example.com", role).Username
	s.userNames.reviewer2 = teleport.AddUserWithRole(me.Username+"-reviewer2@example.com", role).Username

	s.approverEmail = me.Username + "-approver@example.com"

	role, err = types.NewRole("access-gitlab", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read", "update"}),
			},
		},
	})
	require.NoError(t, err)
	s.userNames.plugin = teleport.AddUserWithRole("access-gitlab", role).Username

	err = teleport.Create(nil, nil)
	require.NoError(t, err)
	if err := teleport.Start(); err != nil {
		t.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = teleport
}

func (s *GitlabSuite) SetupTest() {
	t := s.T()

	s.fakeGitlab = NewFakeGitLab(projectID, s.raceNumber)
	t.Cleanup(s.fakeGitlab.Close)

	dbFile := s.NewTmpFile("db.*")
	s.dbPath = dbFile.Name()
	dbFile.Close()

	auth := s.teleport.Process.GetAuthServer()
	certAuthorities, err := auth.GetCertAuthorities(services.HostCA, false)
	require.NoError(t, err)
	pluginKey := s.teleport.Secrets.Users["access-gitlab"].Key

	keyFile := s.NewTmpFile("auth.*.key")
	_, err = keyFile.Write(pluginKey.Priv)
	require.NoError(t, err)
	keyFile.Close()

	certFile := s.NewTmpFile("auth.*.crt")
	_, err = certFile.Write(pluginKey.TLSCert)
	require.NoError(t, err)
	certFile.Close()

	casFile := s.NewTmpFile("auth.*.cas")
	for _, ca := range certAuthorities {
		for _, keyPair := range ca.GetTLSKeyPairs() {
			_, err = casFile.Write(keyPair.Cert)
			require.NoError(t, err)
		}
	}
	casFile.Close()

	authAddr, err := s.teleport.Process.AuthSSHAddr()
	require.NoError(t, err)

	var conf Config
	conf.Teleport.Addr = authAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.Gitlab.URL = s.fakeGitlab.URL()
	conf.Gitlab.WebhookSecret = WebhookSecret
	conf.Gitlab.ProjectID = fmt.Sprintf("%d", projectID)
	conf.DB.Path = s.dbPath
	conf.HTTP.ListenAddr = ":0"
	conf.HTTP.Insecure = true

	s.appConfig = conf
}

func (s *GitlabSuite) startApp() {
	t := s.T()
	t.Helper()

	app, err := NewApp(s.appConfig)
	require.NoError(t, err)

	s.StartApp(app)
	s.publicURL = app.PublicURL().String()
	s.app = app
	t.Cleanup(func() {
		s.app = nil
	})
}

func (s *GitlabSuite) shutdownApp() {
	t := s.T()
	t.Helper()

	err := s.app.Shutdown(s.Ctx())
	require.NoError(t, err)
}

func (s *GitlabSuite) newAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req, err := services.NewAccessRequest(s.userNames.requestor, "admin")
	require.NoError(t, err)
	return req
}
func (s *GitlabSuite) createAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest()
	err := s.teleport.CreateAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *GitlabSuite) createExpiredAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest()
	err := s.teleport.CreateExpiredAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *GitlabSuite) checkPluginData(reqID string, cond func(PluginData) bool) PluginData {
	t := s.T()
	t.Helper()

	for {
		rawData, err := s.teleport.PollAccessRequestPluginData(s.Ctx(), "gitlab", reqID)
		require.NoError(t, err)
		if data := DecodePluginData(rawData); cond(data) {
			return data
		}
	}
}

func (s *GitlabSuite) assertNewLabels(expected int) map[string]Label {
	t := s.T()
	t.Helper()

	newLabels := s.fakeGitlab.GetAllNewLabels()
	actual := len(newLabels)
	assert.GreaterOrEqual(t, expected, actual, "expected %d labels but extra %d labels was stored", expected, actual-expected)
	assert.LessOrEqual(t, expected, actual, "expected %d labels but %d labels are missing", expected, expected-actual)
	return newLabels
}

func (s *GitlabSuite) postIssueUpdateHook(ctx context.Context, oldIssue, newIssue Issue) (*http.Response, error) {
	var labelsChange *LabelsChange
	if !reflect.DeepEqual(oldIssue.Labels, newIssue.Labels) {
		labelsChange = &LabelsChange{Previous: s.fakeGitlab.GetLabels(oldIssue.Labels...), Current: s.fakeGitlab.GetLabels(newIssue.Labels...)}
	}
	payload := IssueEvent{
		Project: Project{ID: projectID},
		User: User{
			Name:  "Test User",
			Email: s.approverEmail,
		},
		ObjectAttributes: IssueObjectAttributes{
			Action:      "update",
			ID:          oldIssue.ID,
			IID:         oldIssue.IID,
			ProjectID:   oldIssue.ProjectID,
			Description: oldIssue.Description,
		},
		Changes: IssueChanges{
			Labels: labelsChange,
		},
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&payload)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.publicURL+gitlabWebhookPath, &buf)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Gitlab-Token", WebhookSecret)
	req.Header.Add("X-Gitlab-Event", "Issue Hook")

	response, err := http.DefaultClient.Do(req)
	return response, trace.Wrap(err)
}

func (s *GitlabSuite) postIssueUpdateHookAndCheck(oldIssue, newIssue Issue) {
	t := s.T()
	t.Helper()

	resp, err := s.postIssueUpdateHook(s.Ctx(), oldIssue, newIssue)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	err = resp.Body.Close()
	require.NoError(t, err)
}

func (s *GitlabSuite) openDB(fn func(db DB) error) {
	t := s.T()
	t.Helper()

	db, err := OpenDB(s.dbPath)
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			panic(err)
		}
	}()
	require.NoError(t, fn(db))
}

func (s *GitlabSuite) TestProjectHookSetup() {
	t := s.T()

	s.startApp()

	hook, err := s.fakeGitlab.CheckNewProjectHook(s.Ctx())
	require.NoError(t, err, "no new project hooks stored")
	assert.Equal(t, s.publicURL+gitlabWebhookPath, hook.URL)

	s.shutdownApp()

	var dbHookID IntID
	s.openDB(func(db DB) error {
		return db.ViewSettings(projectID, func(settings SettingsBucket) error {
			dbHookID = settings.HookID()
			return nil
		})
	})
	assert.Equal(t, hook.ID, dbHookID)
}

func (s *GitlabSuite) TestProjectHookSetupWhenItExists() {
	t := s.T()

	s.appConfig.HTTP.PublicAddr = "http://teleport-gitlab.local"
	hook := s.fakeGitlab.StoreProjectHook(ProjectHook{
		ProjectID: projectID,
		URL:       s.appConfig.HTTP.PublicAddr + gitlabWebhookPath,
	})

	s.startApp()
	s.shutdownApp()

	require.True(t, s.fakeGitlab.CheckNoNewProjectHooks())

	var dbHookID IntID
	s.openDB(func(db DB) error {
		return db.ViewSettings(projectID, func(settings SettingsBucket) error {
			dbHookID = settings.HookID()
			return nil
		})
	})
	assert.Equal(t, hook.ID, dbHookID)
}

func (s *GitlabSuite) TestProjectHookSetupWhenItExistsInDB() {
	t := s.T()

	existingHook := s.fakeGitlab.StoreProjectHook(ProjectHook{
		ProjectID: projectID,
		URL:       "http://fooo",
	})

	s.openDB(func(db DB) error {
		return db.UpdateSettings(projectID, func(settings SettingsBucket) error {
			return settings.SetHookID(existingHook.ID)
		})
	})

	s.startApp()

	hook, err := s.fakeGitlab.CheckProjectHookUpdate(s.Ctx())
	require.NoError(t, err, "no project hooks updated")
	assert.Equal(t, existingHook.ProjectID, hook.ProjectID)
	assert.Equal(t, existingHook.ID, hook.ID)
	assert.Equal(t, s.publicURL+gitlabWebhookPath, hook.URL)

	s.shutdownApp()

	var dbHookID IntID
	s.openDB(func(db DB) error {
		return db.ViewSettings(projectID, func(settings SettingsBucket) error {
			dbHookID = settings.HookID()
			return nil
		})
	})
	assert.Equal(t, existingHook.ID, dbHookID)
}

func (s *GitlabSuite) TestLabelsSetup() {
	t := s.T()

	s.startApp()

	newLabels := s.assertNewLabels(4)
	assert.Equal(t, "Teleport: Pending", newLabels["pending"].Name)
	assert.Equal(t, "Teleport: Approved", newLabels["approved"].Name)
	assert.Equal(t, "Teleport: Denied", newLabels["denied"].Name)
	assert.Equal(t, "Teleport: Expired", newLabels["expired"].Name)

	s.shutdownApp()

	var dbLabels map[string]string
	s.openDB(func(db DB) error {
		return db.ViewSettings(projectID, func(settings SettingsBucket) error {
			dbLabels = settings.GetLabels("pending", "approved", "denied", "expired")
			return nil
		})
	})
	assert.Equal(t, newLabels["pending"].Name, dbLabels["pending"])
	assert.Equal(t, newLabels["approved"].Name, dbLabels["approved"])
	assert.Equal(t, newLabels["denied"].Name, dbLabels["denied"])
	assert.Equal(t, newLabels["expired"].Name, dbLabels["expired"])
}

func (s *GitlabSuite) TestLabelsSetupWhenSomeExist() {
	t := s.T()

	labels := map[string]Label{
		"pending": s.fakeGitlab.StoreLabel(Label{Name: "teleport:pending"}),
		"expired": s.fakeGitlab.StoreLabel(Label{Name: "teleport:expired"}),
	}

	s.startApp()

	newLabels := s.assertNewLabels(2)
	assert.Equal(t, "Teleport: Approved", newLabels["approved"].Name)
	assert.Equal(t, "Teleport: Denied", newLabels["denied"].Name)

	s.shutdownApp()

	var dbLabels map[string]string
	s.openDB(func(db DB) error {
		return db.ViewSettings(projectID, func(settings SettingsBucket) error {
			dbLabels = settings.GetLabels("pending", "approved", "denied", "expired")
			return nil
		})
	})

	assert.Equal(t, labels["pending"].Name, dbLabels["pending"])
	assert.Equal(t, newLabels["approved"].Name, dbLabels["approved"])
	assert.Equal(t, newLabels["denied"].Name, dbLabels["denied"])
	assert.Equal(t, labels["expired"].Name, dbLabels["expired"])
}

func (s *GitlabSuite) TestIssueCreation() {
	t := s.T()

	s.startApp()

	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.GitlabData.IssueID != 0
	}) // when issue id is written, we are sure that request is completely served.

	issue, err := s.fakeGitlab.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, projectID, issue.ProjectID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	assert.Equal(t, issue.ProjectID, pluginData.ProjectID)
	assert.Equal(t, issue.IID, pluginData.IssueIID)
	assert.Equal(t, issue.ID, pluginData.IssueID)

	s.shutdownApp()

	var reqID string
	s.openDB(func(db DB) error {
		return db.ViewIssues(projectID, func(issues IssuesBucket) error {
			reqID = issues.GetRequestID(issue.IID)
			return nil
		})
	})

	assert.Equal(t, request.GetName(), reqID)
}

func (s *GitlabSuite) TestReviewComments() {
	t := s.T()

	s.startApp()
	req := s.createAccessRequest()

	req, err := s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)
	req, err = s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	pluginData := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.GitlabData.IssueID != 0 && data.ReviewsCount == 2
	})
	issueID := pluginData.IssueID

	note, err := s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer1+"** reviewed the request", "comment must contain a review author")
	assert.Contains(t, note.Body, "Resolution: **APPROVED**", "comment must contain an approval resolution")
	assert.Contains(t, note.Body, "Reason: okay", "comment must contain an approval reason")

	note, err = s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer2+"** reviewed the request", "comment must contain a review author")
	assert.Contains(t, note.Body, "Resolution: **DENIED**", "comment must contain an denial resolution")
	assert.Contains(t, note.Body, "Reason: not okay", "comment must contain an denial reason")
}

func (s *GitlabSuite) TestReviewerApproval() {
	t := s.T()

	s.startApp()
	req := s.createAccessRequest()

	pluginData := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != 0
	})
	issueID := pluginData.IssueID

	req, err := s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	note, err := s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer1+"** reviewed the request", "comment must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "finally okay",
	})
	require.NoError(t, err)

	note, err = s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer2+"** reviewed the request", "comment must contain a review author")

	pluginData = s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != 0 && data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, issueID, pluginData.IssueID)
	assert.Equal(t, Resolution{Tag: ResolvedApproved, Reason: "finally okay"}, pluginData.Resolution)

	issue, err := s.fakeGitlab.CheckIssueUpdate(s.Ctx())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "approved", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	note, err = s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been approved")
	assert.Contains(t, note.Body, "Reason: finally okay")
}

func (s *GitlabSuite) TestReviewerDenial() {
	t := s.T()

	s.startApp()
	req := s.createAccessRequest()

	pluginData := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != 0
	})
	issueID := pluginData.IssueID

	req, err := s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	note, err := s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer1+"** reviewed the request", "comment must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "finally not okay",
	})
	require.NoError(t, err)

	note, err = s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer2+"** reviewed the request", "comment must contain a review author")

	pluginData = s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != 0 && data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, issueID, pluginData.IssueID)
	assert.Equal(t, Resolution{Tag: ResolvedDenied, Reason: "finally not okay"}, pluginData.Resolution)

	issue, err := s.fakeGitlab.CheckIssueUpdate(s.Ctx())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "denied", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	note, err = s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been denied")
	assert.Contains(t, note.Body, "Reason: finally not okay")
}

func (s *GitlabSuite) TestWebhookApproval() {
	t := s.T()

	s.startApp()

	labels := s.assertNewLabels(4)
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.GitlabData.IssueID != 0
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeGitlab.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	oldIssue := issue
	issue.Labels = []string{labels["approved"].Title}
	s.fakeGitlab.StoreIssue(issue)
	s.postIssueUpdateHookAndCheck(oldIssue, issue)

	issue, err = s.fakeGitlab.CheckIssueUpdate(s.Ctx())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "approved", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	request, err = s.teleport.GetAccessRequest(s.Ctx(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, types.RequestState_APPROVED, request.GetState())

	events, err := s.teleport.SearchAccessRequestEvents(request.GetName())
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "APPROVED", events[0].RequestState)
	assert.Equal(t, "gitlab:"+s.approverEmail, events[0].Delegator)

	note, err := s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been approved")
}

func (s *GitlabSuite) TestWebhookDenial() {
	t := s.T()

	s.startApp()

	labels := s.assertNewLabels(4)
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.GitlabData.IssueID != 0
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeGitlab.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	oldIssue := issue
	issue.Labels = []string{labels["denied"].Title}
	s.fakeGitlab.StoreIssue(issue)
	s.postIssueUpdateHookAndCheck(oldIssue, issue)

	issue, err = s.fakeGitlab.CheckIssueUpdate(s.Ctx())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "denied", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	request, err = s.teleport.GetAccessRequest(s.Ctx(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, types.RequestState_DENIED, request.GetState())

	events, err := s.teleport.SearchAccessRequestEvents(request.GetName())
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "DENIED", events[0].RequestState)
	assert.Equal(t, "gitlab:"+s.approverEmail, events[0].Delegator)

	note, err := s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been denied")
}

func (s *GitlabSuite) TestExpiration() {
	t := s.T()

	s.startApp()
	request := s.createExpiredAccessRequest()

	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != 0
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeGitlab.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	issue, err = s.fakeGitlab.CheckIssueUpdate(s.Ctx())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "expired", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	note, err := s.fakeGitlab.CheckNewNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been expired")
}

func (s *GitlabSuite) TestRace() {
	t := s.T()

	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	s.SetContext(20 * time.Second)
	s.startApp()

	labels := s.assertNewLabels(4)

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

	watcher, err := s.teleport.Process.GetAuthServer().NewWatcher(s.Ctx(), services.Watch{
		Kinds: []services.WatchKind{
			{
				Kind: types.KindAccessRequest,
			},
		},
	})
	require.NoError(t, err)
	defer watcher.Close()
	assert.Equal(t, backend.OpInit, (<-watcher.Events()).Type)

	process := lib.NewProcess(s.Ctx())
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			req, err := services.NewAccessRequest(s.userNames.requestor, "admin")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			if err = s.teleport.CreateAccessRequest(ctx, req); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			issue, err := s.fakeGitlab.CheckNewIssue(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			if obtained, expected := len(issue.Labels), 1; obtained != expected {
				return setRaceErr(trace.Errorf("wrong labels size. expected %v, obtained %v", expected, obtained))
			}
			if obtained, expected := LabelName(issue.Labels[0]).Reduced(), "pending"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong label. expected %q, obtained %q", expected, obtained))
			}

			oldIssue := issue
			issue.Labels = []string{labels["approved"].Name}
			s.fakeGitlab.StoreIssue(issue)

			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			var lastErr error
			for {
				log.Infof("Trying to approve issue %v", issue.ID)
				resp, err := s.postIssueUpdateHook(ctx, oldIssue, issue)
				if err != nil {
					if lib.IsDeadline(err) {
						return setRaceErr(lastErr)
					}
					return setRaceErr(trace.Wrap(err))
				}
				if err := resp.Body.Close(); err != nil {
					return setRaceErr(trace.Wrap(err))
				}
				if status := resp.StatusCode; status != http.StatusNoContent {
					lastErr = trace.Errorf("got %v http code from webhook server", status)
				} else {
					return nil
				}
			}
		})
		process.SpawnCritical(func(ctx context.Context) error {
			issue, err := s.fakeGitlab.CheckIssueUpdate(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			if obtained, expected := issue.State, "closed"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong issue state. expected %q, obtained %q", expected, obtained))
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
	require.NoError(t, raceErr)

	var count int
	requests.Range(func(key, val interface{}) bool {
		count++
		assert.Equal(t, int64(0), *val.(*int64))
		return true
	})
	assert.Equal(t, s.raceNumber, count)
}
