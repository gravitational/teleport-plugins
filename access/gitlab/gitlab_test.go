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

	"github.com/google/uuid"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	. "github.com/gravitational/teleport-plugins/lib/testing"
	"github.com/gravitational/teleport-plugins/lib/testing/integration"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	WebhookSecret = "0000"
	projectID     = IntID(1111)
)

type GitlabSuite struct {
	Suite
	appConfig Config
	userNames struct {
		requestor string
		reviewer1 string
		reviewer2 string
		plugin    string
	}
	approverEmail string
	raceNumber    int
	dbPath        string
	fakeGitlab    *FakeGitlab

	teleport         *integration.API
	teleportFeatures *proto.Features
	teleportConfig   lib.TeleportConfig
}

func TestGitlab(t *testing.T) { suite.Run(t, &GitlabSuite{}) }

func (s *GitlabSuite) SetupSuite() {
	var err error
	t := s.T()
	ctx := s.Context()

	logger.Init()
	logger.Setup(logger.Config{Severity: "debug"})
	s.raceNumber = runtime.GOMAXPROCS(0)
	me, err := user.Current()
	require.NoError(t, err)

	s.approverEmail = me.Username + "-approver@example.com"

	teleport, err := integration.NewFromEnv(ctx)
	require.NoError(t, err)
	t.Cleanup(teleport.Close)

	auth, err := teleport.NewAuthServer()
	require.NoError(t, err)
	s.StartApp(auth)

	api, err := teleport.NewAPI(ctx, auth)
	require.NoError(t, err)

	pong, err := api.Ping(ctx)
	require.NoError(t, err)
	teleportFeatures := pong.GetServerFeatures()

	var bootstrap integration.Bootstrap

	// Set up user who can request the access to role "admin".

	conditions := types.RoleConditions{Request: &types.AccessRequestConditions{Roles: []string{"admin"}}}
	if teleportFeatures.AdvancedAccessWorkflows {
		conditions.Request.Thresholds = []types.AccessReviewThreshold{types.AccessReviewThreshold{Approve: 2, Deny: 2}}
	}
	role, err := bootstrap.AddRole("foo", types.RoleSpecV4{Allow: conditions})
	require.NoError(t, err)

	user, err := bootstrap.AddUserWithRoles(me.Username+"@example.com", role.GetName())
	require.NoError(t, err)
	s.userNames.requestor = user.GetName()

	// Set up TWO users who can review access requests to role "admin".

	conditions = types.RoleConditions{}
	if teleportFeatures.AdvancedAccessWorkflows {
		conditions.ReviewRequests = &types.AccessReviewConditions{Roles: []string{"admin"}}
	}
	role, err = bootstrap.AddRole("foo-reviewer", types.RoleSpecV4{Allow: conditions})
	require.NoError(t, err)

	user, err = bootstrap.AddUserWithRoles(me.Username+"-reviewer1@example.com", role.GetName())
	require.NoError(t, err)
	s.userNames.reviewer1 = user.GetName()

	user, err = bootstrap.AddUserWithRoles(me.Username+"-reviewer2@example.com", role.GetName())
	require.NoError(t, err)
	s.userNames.reviewer2 = user.GetName()

	// Set up plugin user.

	role, err = bootstrap.AddRole("access-gitlab", types.RoleSpecV4{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read", "update"}),
			},
		},
	})
	require.NoError(t, err)

	user, err = bootstrap.AddUserWithRoles("access-gitlab", role.GetName())
	require.NoError(t, err)
	s.userNames.plugin = user.GetName()

	// Bake all the resources.

	err = teleport.Bootstrap(ctx, auth, bootstrap.Resources())
	require.NoError(t, err)

	identityPath, err := teleport.Sign(ctx, auth, s.userNames.plugin)
	require.NoError(t, err)

	s.teleport = api
	s.teleportConfig.Addr = auth.PublicAddr()
	s.teleportConfig.Identity = identityPath
	s.teleportFeatures = teleportFeatures
}

func (s *GitlabSuite) SetupTest() {
	t := s.T()

	logger.Setup(logger.Config{Severity: "debug"})

	s.fakeGitlab = NewFakeGitLab(projectID, s.raceNumber)
	t.Cleanup(s.fakeGitlab.Close)

	dbFile := s.NewTmpFile("db.*")
	s.dbPath = dbFile.Name()
	dbFile.Close()

	var conf Config
	conf.Teleport = s.teleportConfig
	conf.Gitlab.URL = s.fakeGitlab.URL()
	conf.Gitlab.WebhookSecret = WebhookSecret
	conf.Gitlab.ProjectID = fmt.Sprintf("%d", projectID)
	conf.DB.Path = s.dbPath
	conf.HTTP.ListenAddr = ":0"
	conf.HTTP.Insecure = true

	s.appConfig = conf

	s.SetContextTimeout(5 * time.Second)
}

func (s *GitlabSuite) startApp() *App {
	t := s.T()
	t.Helper()

	app, err := NewApp(s.appConfig)
	require.NoError(t, err)

	s.StartApp(app)

	return app
}

func (s *GitlabSuite) newAccessRequest() types.AccessRequest {
	t := s.T()
	t.Helper()

	req, err := types.NewAccessRequest(uuid.New().String(), s.userNames.requestor, "admin")
	require.NoError(t, err)
	return req
}

func (s *GitlabSuite) createAccessRequest() types.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest()
	err := s.teleport.CreateAccessRequest(s.Context(), req)
	require.NoError(t, err)
	return req
}

func (s *GitlabSuite) checkPluginData(reqID string, cond func(PluginData) bool) PluginData {
	t := s.T()
	t.Helper()

	for {
		rawData, err := s.teleport.PollAccessRequestPluginData(s.Context(), "gitlab", reqID)
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

func (s *GitlabSuite) postIssueUpdateHook(ctx context.Context, url string, oldIssue, newIssue Issue) (*http.Response, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Gitlab-Token", WebhookSecret)
	req.Header.Add("X-Gitlab-Event", "Issue Hook")

	response, err := http.DefaultClient.Do(req)
	return response, trace.Wrap(err)
}

func (s *GitlabSuite) postIssueUpdateHookAndCheck(url string, oldIssue, newIssue Issue) {
	t := s.T()
	t.Helper()

	resp, err := s.postIssueUpdateHook(s.Context(), url, oldIssue, newIssue)
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

	app := s.startApp()

	hook, err := s.fakeGitlab.CheckNewProjectHook(s.Context())
	require.NoError(t, err, "no new project hooks stored")
	assert.Equal(t, app.PublicURL().String()+gitlabWebhookPath, hook.URL)

	err = app.Shutdown(s.Context())
	require.NoError(t, err)

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

	app := s.startApp()
	err := app.Shutdown(s.Context())
	require.NoError(t, err)

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

	app := s.startApp()

	hook, err := s.fakeGitlab.CheckProjectHookUpdate(s.Context())
	require.NoError(t, err, "no project hooks updated")
	assert.Equal(t, existingHook.ProjectID, hook.ProjectID)
	assert.Equal(t, existingHook.ID, hook.ID)
	assert.Equal(t, app.PublicURL().String()+gitlabWebhookPath, hook.URL)

	err = app.Shutdown(s.Context())
	require.NoError(t, err)

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

	app := s.startApp()

	newLabels := s.assertNewLabels(4)
	assert.Equal(t, "Teleport: Pending", newLabels["pending"].Name)
	assert.Equal(t, "Teleport: Approved", newLabels["approved"].Name)
	assert.Equal(t, "Teleport: Denied", newLabels["denied"].Name)
	assert.Equal(t, "Teleport: Expired", newLabels["expired"].Name)

	err := app.Shutdown(s.Context())
	require.NoError(t, err)

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

	app := s.startApp()

	newLabels := s.assertNewLabels(2)
	assert.Equal(t, "Teleport: Approved", newLabels["approved"].Name)
	assert.Equal(t, "Teleport: Denied", newLabels["denied"].Name)

	err := app.Shutdown(s.Context())
	require.NoError(t, err)

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

	app := s.startApp()

	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.GitlabData.IssueID != 0
	}) // when issue id is written, we are sure that request is completely served.

	issue, err := s.fakeGitlab.CheckNewIssue(s.Context())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, projectID, issue.ProjectID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	assert.Equal(t, issue.ProjectID, pluginData.ProjectID)
	assert.Equal(t, issue.IID, pluginData.IssueIID)
	assert.Equal(t, issue.ID, pluginData.IssueID)

	err = app.Shutdown(s.Context())
	require.NoError(t, err)

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

	req, err := s.teleport.SubmitAccessReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)
	req, err = s.teleport.SubmitAccessReview(s.Context(), req.GetName(), types.AccessReview{
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

	note, err := s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer1+"** reviewed the request", "comment must contain a review author")
	assert.Contains(t, note.Body, "Resolution: **APPROVED**", "comment must contain an approval resolution")
	assert.Contains(t, note.Body, "Reason: okay", "comment must contain an approval reason")

	note, err = s.fakeGitlab.CheckNewNote(s.Context())
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

	req, err := s.teleport.SubmitAccessReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	note, err := s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer1+"** reviewed the request", "comment must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "finally okay",
	})
	require.NoError(t, err)

	note, err = s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer2+"** reviewed the request", "comment must contain a review author")

	pluginData = s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != 0 && data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, issueID, pluginData.IssueID)
	assert.Equal(t, Resolution{Tag: ResolvedApproved, Reason: "finally okay"}, pluginData.Resolution)

	issue, err := s.fakeGitlab.CheckIssueUpdate(s.Context())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "approved", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	note, err = s.fakeGitlab.CheckNewNote(s.Context())
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

	req, err := s.teleport.SubmitAccessReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	note, err := s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer1+"** reviewed the request", "comment must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "finally not okay",
	})
	require.NoError(t, err)

	note, err = s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "**"+s.userNames.reviewer2+"** reviewed the request", "comment must contain a review author")

	pluginData = s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != 0 && data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, issueID, pluginData.IssueID)
	assert.Equal(t, Resolution{Tag: ResolvedDenied, Reason: "finally not okay"}, pluginData.Resolution)

	issue, err := s.fakeGitlab.CheckIssueUpdate(s.Context())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "denied", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	note, err = s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been denied")
	assert.Contains(t, note.Body, "Reason: finally not okay")
}

func (s *GitlabSuite) TestWebhookApproval() {
	t := s.T()

	app := s.startApp()

	labels := s.assertNewLabels(4)
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.GitlabData.IssueID != 0
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeGitlab.CheckNewIssue(s.Context())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	oldIssue := issue
	issue.Labels = []string{labels["approved"].Title}
	s.fakeGitlab.StoreIssue(issue)
	s.postIssueUpdateHookAndCheck(app.PublicURL().String()+gitlabWebhookPath, oldIssue, issue)

	issue, err = s.fakeGitlab.CheckIssueUpdate(s.Context())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "approved", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	request, err = s.teleport.GetAccessRequest(s.Context(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, types.RequestState_APPROVED, request.GetState())

	events, err := s.teleport.SearchAccessRequestEvents(s.Context(), request.GetName())
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "APPROVED", events[0].RequestState)
	assert.Equal(t, "gitlab:"+s.approverEmail, events[0].Delegator)

	note, err := s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been approved")
}

func (s *GitlabSuite) TestWebhookDenial() {
	t := s.T()

	app := s.startApp()

	labels := s.assertNewLabels(4)
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.GitlabData.IssueID != 0
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeGitlab.CheckNewIssue(s.Context())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	oldIssue := issue
	issue.Labels = []string{labels["denied"].Title}
	s.fakeGitlab.StoreIssue(issue)
	s.postIssueUpdateHookAndCheck(app.PublicURL().String()+gitlabWebhookPath, oldIssue, issue)

	issue, err = s.fakeGitlab.CheckIssueUpdate(s.Context())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "denied", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	request, err = s.teleport.GetAccessRequest(s.Context(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, types.RequestState_DENIED, request.GetState())

	events, err := s.teleport.SearchAccessRequestEvents(s.Context(), request.GetName())
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "DENIED", events[0].RequestState)
	assert.Equal(t, "gitlab:"+s.approverEmail, events[0].Delegator)

	note, err := s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been denied")
}

func (s *GitlabSuite) TestExpiration() {
	t := s.T()

	s.startApp()

	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != 0
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeGitlab.CheckNewIssue(s.Context())
	require.NoError(t, err, "no new issues stored")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "pending", LabelName(issue.Labels[0]).Reduced())

	err = s.teleport.DeleteAccessRequest(s.Context(), request.GetName()) // simulate expiration
	require.NoError(t, err)

	issue, err = s.fakeGitlab.CheckIssueUpdate(s.Context())
	require.NoError(t, err, "no issues updated")
	assert.Equal(t, issueID, issue.ID)
	assert.Len(t, issue.Labels, 1)
	assert.Equal(t, "expired", LabelName(issue.Labels[0]).Reduced())
	assert.Equal(t, "closed", issue.State)

	note, err := s.fakeGitlab.CheckNewNote(s.Context())
	require.NoError(t, err)
	assert.Equal(t, "Issue", note.NoteableType)
	assert.Equal(t, issueID, note.NoteableID)
	assert.Contains(t, note.Body, "Access request has been expired")
}

func (s *GitlabSuite) TestRace() {
	t := s.T()

	logger.Setup(logger.Config{Severity: "info"}) // Turn off noisy debug logging

	s.SetContextTimeout(20 * time.Second)
	app := s.startApp()

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

	watcher, err := s.teleport.NewWatcher(s.Context(), types.Watch{
		Kinds: []types.WatchKind{
			{
				Kind: types.KindAccessRequest,
			},
		},
	})
	require.NoError(t, err)
	defer watcher.Close()
	assert.Equal(t, types.OpInit, (<-watcher.Events()).Type)

	process := lib.NewProcess(s.Context())
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			req, err := types.NewAccessRequest(uuid.New().String(), s.userNames.requestor, "admin")
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
				logger.Get(ctx).Infof("Trying to approve issue %v", issue.ID)
				resp, err := s.postIssueUpdateHook(ctx, app.PublicURL().String()+gitlabWebhookPath, oldIssue, issue)
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
	require.NoError(t, raceErr)

	var count int
	requests.Range(func(key, val interface{}) bool {
		count++
		assert.Equal(t, int64(0), *val.(*int64))
		return true
	})
	assert.Equal(t, s.raceNumber, count)
}
