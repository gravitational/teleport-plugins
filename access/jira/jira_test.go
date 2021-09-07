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
	"net/http"
	"os/user"
	"runtime"
	"strings"
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
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"
)

type JiraSuite struct {
	Suite
	appConfig Config
	userNames struct {
		plugin    string
		requestor string
		reviewer1 string
		reviewer2 string
	}
	publicURL  string
	raceNumber int
	authorUser UserDetails
	otherUser  UserDetails
	fakeJira   *FakeJira
	teleport   *integration.TeleInstance
}

func TestJira(t *testing.T) { suite.Run(t, &JiraSuite{}) }

func (s *JiraSuite) SetupSuite() {
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

	s.authorUser = UserDetails{AccountID: "USER-1", DisplayName: me.Username, EmailAddress: s.userNames.requestor}
	s.otherUser = UserDetails{AccountID: "USER-2", DisplayName: me.Username + " evil twin", EmailAddress: me.Username + "-evil@example.com"}

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

	role, err = types.NewRole("access-jira", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read", "update"}),
			},
		},
	})
	require.NoError(t, err)
	s.userNames.plugin = teleport.AddUserWithRole("access-jira", role).Username

	err = teleport.Create(nil, nil)
	require.NoError(t, err)
	if err := teleport.Start(); err != nil {
		t.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = teleport
}

func (s *JiraSuite) SetupTest() {
	t := s.T()

	s.fakeJira = NewFakeJira(s.authorUser, s.raceNumber)
	t.Cleanup(s.fakeJira.Close)

	auth := s.teleport.Process.GetAuthServer()
	certAuthorities, err := auth.GetCertAuthorities(services.HostCA, false)
	require.NoError(t, err)
	pluginKey := s.teleport.Secrets.Users["access-jira"].Key

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
	conf.Jira.URL = s.fakeJira.URL()
	conf.Jira.Username = "jira-bot@example.com"
	conf.Jira.APIToken = "xyz"
	conf.Jira.Project = "PROJ"
	conf.HTTP.ListenAddr = ":0"
	conf.HTTP.Insecure = true

	s.appConfig = conf
}

func (s *JiraSuite) startApp() {
	t := s.T()
	t.Helper()

	app, err := NewApp(s.appConfig)
	require.NoError(t, err)

	s.StartApp(app)
	s.publicURL = app.PublicURL().String()
}

func (s *JiraSuite) newAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req, err := services.NewAccessRequest(s.userNames.requestor, "admin")
	require.NoError(t, err)
	return req
}

func (s *JiraSuite) createAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest()
	err := s.teleport.CreateAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *JiraSuite) createExpiredAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest()
	err := s.teleport.CreateExpiredAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *JiraSuite) checkPluginData(reqID string, cond func(PluginData) bool) PluginData {
	t := s.T()
	t.Helper()

	for {
		rawData, err := s.teleport.PollAccessRequestPluginData(s.Ctx(), "jira", reqID)
		require.NoError(t, err)
		if data := DecodePluginData(rawData); cond(data) {
			return data
		}
	}
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

func (s *JiraSuite) postWebhookAndCheck(issueID string) {
	t := s.T()
	t.Helper()

	resp, err := s.postWebhook(s.Ctx(), issueID)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func (s *JiraSuite) TestIssueCreation() {
	t := s.T()

	s.startApp()
	request := s.createAccessRequest()

	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	}) // when issue id is written, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issue stored")
	assert.Equal(t, "PROJ", issue.Fields.Project.Key)
	assert.Equal(t, request.GetName(), issue.Properties[RequestIDPropertyKey])
	assert.Equal(t, pluginData.IssueID, issue.ID)
}

func (s *JiraSuite) TestIssueCreationWithRequestReason() {
	t := s.T()

	s.startApp()

	req := s.newAccessRequest()
	req.SetRequestReason("because of")
	err := s.teleport.CreateAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	}) // when issue id is written, we are sure that request is completely served.

	issue, err := s.fakeJira.CheckNewIssue(s.Ctx())
	require.NoError(t, err)

	if !strings.Contains(issue.Fields.Description, `Reason: *because of*`) {
		t.Error("Issue description should contain request reason")
	}
}

func (s *JiraSuite) TestReviewComments() {
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
		return data.IssueID != "" && data.ReviewsCount == 2
	})

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, pluginData.IssueID, comment.IssueID)
	assert.Contains(t, comment.Body, "*"+s.userNames.reviewer1+"* reviewed the request", "comment must contain a review author")
	assert.Contains(t, comment.Body, "Resolution: *APPROVED*", "comment must contain an approval resolution")
	assert.Contains(t, comment.Body, "Reason: okay", "comment must contain an approval reason")

	comment, err = s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, pluginData.IssueID, comment.IssueID)
	assert.Contains(t, comment.Body, "*"+s.userNames.reviewer2+"* reviewed the request", "comment must contain a review author")
	assert.Contains(t, comment.Body, "Resolution: *DENIED*", "comment must contain a denial resolution")
	assert.Contains(t, comment.Body, "Reason: not okay", "comment must contain a denial reason")
}

func (s *JiraSuite) TestReviewerApproval() {
	t := s.T()

	s.startApp()
	req := s.createAccessRequest()

	pluginData := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	})
	issueID := pluginData.IssueID

	req, err := s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "*"+s.userNames.reviewer1+"* reviewed the request", "comment must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "finally okay",
	})
	require.NoError(t, err)

	comment, err = s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "*"+s.userNames.reviewer2+"* reviewed the request", "comment must contain a review author")

	pluginData = s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != "" && data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, issueID, pluginData.IssueID)
	assert.Equal(t, Resolution{Tag: ResolvedApproved, Reason: "finally okay"}, pluginData.Resolution)

	issue, err := s.fakeJira.CheckIssueTransition(s.Ctx())
	require.NoError(t, err, "no issue transition detected")
	assert.Equal(t, issueID, issue.ID)
	assert.Equal(t, "Approved", issue.Fields.Status.Name)

	comment, err = s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "Access request has been approved")
	assert.Contains(t, comment.Body, "Reason: finally okay")
}

func (s *JiraSuite) TestReviewerDenial() {
	t := s.T()

	s.startApp()
	req := s.createAccessRequest()

	pluginData := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	})
	issueID := pluginData.IssueID

	req, err := s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "*"+s.userNames.reviewer1+"* reviewed the request", "comment must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "finally not okay",
	})
	require.NoError(t, err)

	comment, err = s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "*"+s.userNames.reviewer2+"* reviewed the request", "comment must contain a review author")

	pluginData = s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IssueID != "" && data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, issueID, pluginData.IssueID)
	assert.Equal(t, Resolution{Tag: ResolvedDenied, Reason: "finally not okay"}, pluginData.Resolution)

	issue, err := s.fakeJira.CheckIssueTransition(s.Ctx())
	require.NoError(t, err, "no issue transition detected")
	assert.Equal(t, issueID, issue.ID)
	assert.Equal(t, "Denied", issue.Fields.Status.Name)

	comment, err = s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "Access request has been denied")
	assert.Contains(t, comment.Body, "Reason: finally not okay")
}

func (s *JiraSuite) TestWebhookApproval() {
	t := s.T()

	s.startApp()
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	}) // when issue id is written, we are sure that request is completely served.
	issueID := pluginData.IssueID

	issue, err := s.fakeJira.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issue stored")
	assert.Equal(t, issueID, issue.ID)

	s.fakeJira.TransitionIssue(issue, "Approved")
	s.postWebhookAndCheck(issue.ID)

	request, err = s.teleport.GetAccessRequest(s.Ctx(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, types.RequestState_APPROVED, request.GetState())

	events, err := s.teleport.SearchAccessRequestEvents(request.GetName())
	require.NoError(t, err)
	if assert.Len(t, events, 1) {
		assert.Equal(t, "APPROVED", events[0].RequestState)
		assert.Equal(t, "jira:"+s.authorUser.EmailAddress, events[0].Delegator)
	}

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "Access request has been approved")
}

func (s *JiraSuite) TestWebhookDenial() {
	t := s.T()

	s.startApp()
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	}) // when issue id is written, we are sure that request is completely served.
	issueID := pluginData.IssueID

	issue, err := s.fakeJira.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issue stored")
	assert.Equal(t, issueID, issue.ID)

	s.fakeJira.TransitionIssue(issue, "Denied")
	s.postWebhookAndCheck(issue.ID)

	request, err = s.teleport.GetAccessRequest(s.Ctx(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, types.RequestState_DENIED, request.GetState())

	events, err := s.teleport.SearchAccessRequestEvents(request.GetName())
	require.NoError(t, err)
	if assert.Len(t, events, 1) {
		assert.Equal(t, "DENIED", events[0].RequestState)
		assert.Equal(t, "jira:"+s.authorUser.EmailAddress, events[0].Delegator)
	}

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "Access request has been denied")
}

func (s *JiraSuite) TestWebhookApprovalWithReason() {
	t := s.T()

	s.startApp()
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeJira.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issue stored")
	assert.Equal(t, issueID, issue.ID)

	issue = s.fakeJira.StoreIssueComment(issue, Comment{
		Author: s.authorUser,
		Body:   "hi! i'm going to approve this request.\nReason:\n\nfoo\nbar\nbaz",
	})

	s.fakeJira.TransitionIssue(issue, "Approved")
	s.postWebhookAndCheck(issue.ID)

	request, err = s.teleport.GetAccessRequest(s.Ctx(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, services.RequestState_APPROVED, request.GetState())
	assert.Equal(t, "foo\nbar\nbaz", request.GetResolveReason())

	events, err := s.teleport.SearchAccessRequestEvents(request.GetName())
	require.NoError(t, err)
	if assert.Len(t, events, 1) {
		assert.Equal(t, "APPROVED", events[0].RequestState)
		assert.Equal(t, "jira:"+s.authorUser.EmailAddress, events[0].Delegator)
	}

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "Access request has been approved")
	assert.Contains(t, comment.Body, "Reason: foo\nbar\nbaz")
}

func (s *JiraSuite) TestWebhookDenialWithReason() {
	t := s.T()

	s.startApp()
	request := s.createAccessRequest()
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeJira.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issue stored")
	assert.Equal(t, issueID, issue.ID)

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
	s.postWebhookAndCheck(issue.ID)

	request, err = s.teleport.GetAccessRequest(s.Ctx(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, services.RequestState_DENIED, request.GetState())
	assert.Equal(t, "foo bar baz", request.GetResolveReason())

	events, err := s.teleport.SearchAccessRequestEvents(request.GetName())
	require.NoError(t, err)
	if assert.Len(t, events, 1) {
		assert.Equal(t, "DENIED", events[0].RequestState)
		assert.Equal(t, "jira:"+s.authorUser.EmailAddress, events[0].Delegator)
	}

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "Access request has been denied")
	assert.Contains(t, comment.Body, "Reason: foo bar baz")
}

func (s *JiraSuite) TestExpiration() {
	t := s.T()

	s.startApp()
	request := s.createExpiredAccessRequest()

	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return data.IssueID != ""
	})
	issueID := pluginData.IssueID

	issue, err := s.fakeJira.CheckNewIssue(s.Ctx())
	require.NoError(t, err, "no new issue stored")
	assert.Equal(t, issueID, issue.ID)

	issue, err = s.fakeJira.CheckIssueTransition(s.Ctx())
	require.NoError(t, err, "no issue transition detected")
	assert.Equal(t, issueID, issue.ID)
	assert.Equal(t, "Expired", issue.Fields.Status.Name)

	comment, err := s.fakeJira.CheckNewIssueComment(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, issueID, comment.IssueID)
	assert.Contains(t, comment.Body, "Access request has been expired")
}

func (s *JiraSuite) TestRace() {
	t := s.T()

	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	s.SetContext(20 * time.Second)
	s.startApp()

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
			if err = s.teleport.CreateAccessRequest(s.Ctx(), req); err != nil {
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
