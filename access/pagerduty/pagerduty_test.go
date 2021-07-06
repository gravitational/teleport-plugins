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
	"context"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gravitational/teleport-plugins/access/integration"
	"github.com/gravitational/teleport-plugins/lib"
	. "github.com/gravitational/teleport-plugins/lib/testing"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/wrappers"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"

	EscalationPolicyID1 = "escalation_policy-1"
	EscalationPolicyID2 = "escalation_policy-2"
	NotifyServiceName   = "Teleport Notifications"
	ServiceName1        = "Service 1"
	ServiceName2        = "Service 2"
	ServiceName3        = "Service 3"
)

type PagerdutySuite struct {
	Suite
	appConfig Config
	userName  string
	userNames struct {
		plugin    string
		reviewer1 string
		reviewer2 string
		requestor string
		approver  string
		racer1    string
		racer2    string
	}
	raceNumber      int
	me              *user.User
	fakePagerduty   *FakePagerduty
	pdNotifyService Service
	pdService1      Service
	pdService2      Service
	pdService3      Service
	teleport        *integration.TeleInstance
}

func TestPagerdutySuite(t *testing.T) { suite.Run(t, &PagerdutySuite{}) }

func (s *PagerdutySuite) SetupSuite() {
	var err error
	t := s.T()
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	require.NoError(t, err)
	teleport := integration.NewInstance(integration.InstanceConfig{ClusterName: Site, HostID: HostID, NodeName: Host, Priv: priv, Pub: pub})

	s.raceNumber = 2 * runtime.GOMAXPROCS(0)
	s.me, err = user.Current()
	require.NoError(t, err)
	role, err := types.NewRole("foo", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{
				Roles: []string{"admin"},
				Annotations: wrappers.Traits{
					NotifyServiceDefaultAnnotation: []string{NotifyServiceName},
				},
				Thresholds: []types.AccessReviewThreshold{
					types.AccessReviewThreshold{Approve: 2, Deny: 2},
				},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.requestor = teleport.AddUserWithRole(s.me.Username+"@example.com", role).Username

	role, err = types.NewRole("bar", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{
				Roles: []string{"admin"},
				Annotations: wrappers.Traits{
					ServicesDefaultAnnotation: []string{ServiceName1, ServiceName2, ServiceName3},
				},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.approver = teleport.AddUserWithRole(s.me.Username+"-approver@example.com", role).Username // For testing auto-approve

	role, err = types.NewRole("foo-reviewer", types.RoleSpecV3{
		Allow: types.RoleConditions{
			ReviewRequests: &types.AccessReviewConditions{
				Roles: []string{"admin"},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.reviewer1 = teleport.AddUserWithRole(s.me.Username+"-reviewer1@example.com", role).Username
	s.userNames.reviewer2 = teleport.AddUserWithRole(s.me.Username+"-reviewer2@example.com", role).Username

	role, err = types.NewRole("foo-bar", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Request: &types.AccessRequestConditions{
				Roles: []string{"admin"},
				Annotations: wrappers.Traits{
					NotifyServiceDefaultAnnotation: []string{NotifyServiceName},
					ServicesDefaultAnnotation:      []string{ServiceName1, ServiceName2, ServiceName3},
				},
				Thresholds: []types.AccessReviewThreshold{
					types.AccessReviewThreshold{Approve: 2, Deny: 2},
				},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.racer1 = teleport.AddUserWithRole(s.me.Username+"-racer1@example.com", role).Username
	s.userNames.racer2 = teleport.AddUserWithRole(s.me.Username+"-racer2@example.com", role).Username

	role, err = types.NewRole("access-plugin", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read"}),
				types.NewRule("access_plugin_data", []string{"update"}),
			},
			ReviewRequests: &types.AccessReviewConditions{
				Roles: []string{"admin"},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.plugin = teleport.AddUserWithRole("plugin", role).Username

	err = teleport.Create(nil, nil)
	require.NoError(t, err)
	if err := teleport.Start(); err != nil {
		t.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = teleport
}

func (s *PagerdutySuite) SetupTest() {
	t := s.T()

	fakePagerduty := NewFakePagerduty(s.raceNumber)
	t.Cleanup(fakePagerduty.Close)
	s.fakePagerduty = fakePagerduty

	s.pdNotifyService = s.fakePagerduty.StoreService(Service{
		Name:             NotifyServiceName,
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID1},
	})
	s.pdService1 = s.fakePagerduty.StoreService(Service{
		Name:             ServiceName1,
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID1},
	})
	s.pdService2 = s.fakePagerduty.StoreService(Service{
		Name:             ServiceName2,
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID1},
	})
	s.pdService3 = s.fakePagerduty.StoreService(Service{
		Name:             ServiceName3,
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID2},
	})

	auth := s.teleport.Process.GetAuthServer()
	certAuthorities, err := auth.GetCertAuthorities(services.HostCA, false)
	require.NoError(t, err)
	pluginKey := s.teleport.Secrets.Users["plugin"].Key

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
	conf.Pagerduty.APIEndpoint = s.fakePagerduty.URL()
	conf.Pagerduty.UserEmail = "bot@example.com"
	conf.Pagerduty.RequestAnnotations.NotifyService = NotifyServiceDefaultAnnotation
	conf.Pagerduty.RequestAnnotations.Services = ServicesDefaultAnnotation

	s.appConfig = conf
	s.userName = s.userNames.requestor
}

func (s *PagerdutySuite) startApp() {
	t := s.T()
	t.Helper()

	app, err := NewApp(s.appConfig)
	require.NoError(t, err)

	s.StartApp(app)
}

func (s *PagerdutySuite) newAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req, err := services.NewAccessRequest(s.userName, "admin")
	require.NoError(s.T(), err)
	return req
}

func (s *PagerdutySuite) createAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest()
	err := s.teleport.CreateAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *PagerdutySuite) createExpiredAccessRequest() services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest()
	err := s.teleport.CreateExpiredAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *PagerdutySuite) checkPluginData(reqID string, cond func(PluginData) bool) PluginData {
	t := s.T()
	t.Helper()

	for {
		rawData, err := s.teleport.PollAccessRequestPluginData(s.Ctx(), "pagerduty", reqID)
		require.NoError(t, err)
		if data := DecodePluginData(rawData); cond(data) {
			return data
		}
	}
}

func (s *PagerdutySuite) TestIncidentCreation() {
	t := s.T()

	s.startApp()

	req := s.createAccessRequest()
	pluginData := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.IncidentID != ""
	})

	incident, err := s.fakePagerduty.CheckNewIncident(s.Ctx())
	require.NoError(t, err, "no new incidents stored")

	assert.Equal(t, incident.ID, pluginData.IncidentID)
	assert.Equal(t, s.pdNotifyService.ID, pluginData.ServiceID)

	assert.Equal(t, pdIncidentKeyPrefix+"/"+req.GetName(), incident.IncidentKey)
	assert.Equal(t, "triggered", incident.Status)
}

func (s *PagerdutySuite) TestReviewNotes() {
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
		return data.IncidentID != "" && data.ReviewsCount == 2
	})

	note, err := s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, pluginData.IncidentID, note.IncidentID)
	assert.Contains(t, note.Content, s.userNames.reviewer1+" reviewed the request", "note must contain a review author")
	assert.Contains(t, note.Content, "Resolution: APPROVED", "note must contain an approval resolution")
	assert.Contains(t, note.Content, "Reason: okay", "note must contain an approval reason")

	note, err = s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, pluginData.IncidentID, note.IncidentID)
	assert.Contains(t, note.Content, s.userNames.reviewer2+" reviewed the request", "note must contain a review author")
	assert.Contains(t, note.Content, "Resolution: DENIED", "note must contain a denial resolution")
	assert.Contains(t, note.Content, "Reason: not okay", "note must contain a denial reason")
}

func (s *PagerdutySuite) TestIncidentApprovalResolution() {
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

	note, err := s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Contains(t, note.Content, s.userNames.reviewer1+" reviewed the request", "note must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "finally okay",
	})
	require.NoError(t, err)

	note, err = s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Contains(t, note.Content, s.userNames.reviewer2+" reviewed the request", "note must contain a review author")

	data := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, Resolution{Tag: ResolvedApproved, Reason: "finally okay"}, data.Resolution)

	note, err = s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Contains(t, note.Content, "Access request has been approved")
	assert.Contains(t, note.Content, "Reason: finally okay")

	incidentUpdate, err := s.fakePagerduty.CheckIncidentUpdate(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "resolved", incidentUpdate.Status)
}

func (s *PagerdutySuite) TestIncidentDenialResolution() {
	t := s.T()

	s.startApp()
	req := s.createAccessRequest()

	req, err := s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	note, err := s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Contains(t, note.Content, s.userNames.reviewer1+" reviewed the request", "note must contain a review author")

	req, err = s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "finally not okay",
	})
	require.NoError(t, err)

	note, err = s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Contains(t, note.Content, s.userNames.reviewer2+" reviewed the request", "note must contain a review author")

	data := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.ReviewsCount == 2 && data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, Resolution{Tag: ResolvedDenied, Reason: "finally not okay"}, data.Resolution)

	note, err = s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err)
	assert.Contains(t, note.Content, "Access request has been denied")
	assert.Contains(t, note.Content, "Reason: finally not okay")

	incidentUpdate, err := s.fakePagerduty.CheckIncidentUpdate(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, "resolved", incidentUpdate.Status)
}

func (s *PagerdutySuite) assertNewEvent(watcher types.Watcher, opType types.OpType, resourceKind, resourceName string) types.Event {
	t := s.T()
	t.Helper()

	var ev types.Event
	select {
	case ev = <-watcher.Events():
		assert.Equal(t, opType, ev.Type)
		if resourceKind != "" {
			assert.Equal(t, resourceKind, ev.Resource.GetKind())
			assert.Equal(t, resourceName, ev.Resource.GetName())
		} else {
			assert.Nil(t, ev.Resource)
		}
	case <-s.Ctx().Done():
		t.Error(t, "No events received", s.Ctx().Err())
	}
	return ev
}

func (s *PagerdutySuite) assertNoNewEvents(watcher types.Watcher) {
	t := s.T()
	t.Helper()

	select {
	case ev := <-watcher.Events():
		t.Errorf("Unexpected event %#v", ev)
	case <-time.After(250 * time.Millisecond):
	case <-s.Ctx().Done():
		t.Error(t, s.Ctx().Err())
	}
}

func (s *PagerdutySuite) assertReviewSubmitted() {
	t := s.T()
	t.Helper()

	watcher, err := s.teleport.Process.GetAuthServer().NewWatcher(s.Ctx(), types.Watch{
		Kinds: []types.WatchKind{{Kind: types.KindAccessRequest}},
	})
	require.NoError(t, err)
	defer watcher.Close()

	_ = s.assertNewEvent(watcher, types.OpInit, "", "")

	request := s.createAccessRequest()
	reqID := request.GetName()

	ev := s.assertNewEvent(watcher, types.OpPut, types.KindAccessRequest, reqID)
	request, ok := ev.Resource.(types.AccessRequest)
	require.True(t, ok)
	assert.Len(t, request.GetReviews(), 0)
	assert.Equal(t, types.RequestState_PENDING, request.GetState())

	ev = s.assertNewEvent(watcher, types.OpPut, types.KindAccessRequest, reqID)
	request, ok = ev.Resource.(services.AccessRequest)
	require.True(t, ok)
	assert.Equal(t, types.RequestState_APPROVED, request.GetState())
	reqReviews := request.GetReviews()
	assert.Len(t, reqReviews, 1)
	assert.Equal(t, "plugin", reqReviews[0].Author)
}

func (s *PagerdutySuite) assertNoReviewSubmitted() {
	t := s.T()
	t.Helper()

	watcher, err := s.teleport.Process.GetAuthServer().NewWatcher(s.Ctx(), types.Watch{
		Kinds: []types.WatchKind{{Kind: types.KindAccessRequest}},
	})
	require.NoError(t, err)
	defer watcher.Close()

	_ = s.assertNewEvent(watcher, types.OpInit, "", "")

	request := s.createAccessRequest()
	reqID := request.GetName()

	ev := s.assertNewEvent(watcher, types.OpPut, types.KindAccessRequest, reqID)

	request, ok := ev.Resource.(types.AccessRequest)
	require.True(t, ok)
	assert.Equal(t, types.RequestState_PENDING, request.GetState())
	assert.Len(t, request.GetReviews(), 0)

	s.assertNoNewEvents(watcher)

	request, err = s.teleport.GetAccessRequest(s.Ctx(), request.GetName())
	require.NoError(t, err)
	assert.Equal(t, types.RequestState_PENDING, request.GetState())
	assert.Len(t, request.GetReviews(), 0)
}

func (s *PagerdutySuite) TestAutoApprovalWhenNoActiveIncidents() {
	s.userName = s.userNames.approver
	pdUser := s.fakePagerduty.StoreUser(User{
		Name:  "Test User",
		Email: s.userName,
	})
	s.fakePagerduty.StoreOnCall(OnCall{
		User:             Reference{Type: "user_reference", ID: pdUser.ID},
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID1},
	})
	s.startApp()
	s.assertNoReviewSubmitted()
}

func (s *PagerdutySuite) TestAutoApprovalWhenActiveIncident() {
	s.userName = s.userNames.approver
	pdUser := s.fakePagerduty.StoreUser(User{
		Name:  "Test User",
		Email: s.userName,
	})
	s.fakePagerduty.StoreOnCall(OnCall{
		User:             Reference{Type: "user_reference", ID: pdUser.ID},
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID1},
	})
	s.fakePagerduty.StoreIncident(Incident{
		Title:   "Maintenance - linux kernel upgrade",
		Status:  "triggered",
		Service: Reference{Type: "service_reference", ID: s.pdService1.ID},
		Assignments: []IncidentAssignment{
			{Assignee: Reference{Type: "user_reference", ID: pdUser.ID}},
		},
	})
	s.startApp()
	s.assertReviewSubmitted()
}

func (s *PagerdutySuite) TestAutoApprovalWhenActiveIncidentInAnotherService() {
	s.userName = s.userNames.approver
	pdUser := s.fakePagerduty.StoreUser(User{
		Name:  "Test User",
		Email: s.userName,
	})
	s.fakePagerduty.StoreOnCall(OnCall{
		User:             Reference{Type: "user_reference", ID: pdUser.ID},
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID1},
	})
	s.fakePagerduty.StoreIncident(Incident{
		Title:   "Maintenance - linux kernel upgrade",
		Status:  "triggered",
		Service: Reference{Type: "service_reference", ID: s.pdService2.ID},
		Assignments: []IncidentAssignment{
			{Assignee: Reference{Type: "user_reference", ID: pdUser.ID}},
		},
	})
	s.startApp()
	s.assertReviewSubmitted()
}

func (s *PagerdutySuite) TestAutoApprovalWhenActiveIncidentOnAnotherPolicy() {
	s.userName = s.userNames.approver
	pdUser := s.fakePagerduty.StoreUser(User{
		Name:  "Test User",
		Email: s.userName,
	})
	s.fakePagerduty.StoreOnCall(OnCall{
		User:             Reference{Type: "user_reference", ID: pdUser.ID},
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID2},
	})
	s.fakePagerduty.StoreIncident(Incident{
		Title:   "Maintenance - linux kernel upgrade",
		Status:  "triggered",
		Service: Reference{Type: "service_reference", ID: s.pdService3.ID},
		Assignments: []IncidentAssignment{
			{Assignee: Reference{Type: "user_reference", ID: pdUser.ID}},
		},
	})
	s.startApp()
	s.assertReviewSubmitted()
}

func (s *PagerdutySuite) TestExpiration() {
	t := s.T()

	s.startApp()

	req := s.createExpiredAccessRequest()
	data := s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return data.Resolution.Tag != Unresolved
	})
	assert.Equal(t, Resolution{Tag: ResolvedExpired}, data.Resolution)

	incident, err := s.fakePagerduty.CheckNewIncident(s.Ctx())
	require.NoError(t, err, "no new incidents stored")
	assert.Equal(t, "triggered", incident.Status)
	incidentID := incident.ID

	incident, err = s.fakePagerduty.CheckIncidentUpdate(s.Ctx())
	require.NoError(t, err, "no new incidents updated")
	assert.Equal(t, incidentID, incident.ID)
	assert.Equal(t, "resolved", incident.Status)

	note, err := s.fakePagerduty.CheckNewIncidentNote(s.Ctx())
	require.NoError(t, err, "no new notes stored")
	assert.Contains(t, note.Content, "Access request has been expired")
}

func (s *PagerdutySuite) TestRace() {
	t := s.T()

	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	s.SetContext(20 * time.Second)
	s.startApp()

	var (
		raceErr               error
		raceErrOnce           sync.Once
		pendingRequests       sync.Map
		resolvedRequests      sync.Map
		incidentIDs           sync.Map
		incidentsCount        int32
		incidentNoteCounters  sync.Map
		resolvedRequestsCount int32
	)
	setRaceErr := func(err error) error {
		raceErrOnce.Do(func() {
			raceErr = err
		})
		return err
	}

	// Set one of the users on-call and assign an incident to her.
	racer2 := s.fakePagerduty.StoreUser(User{
		Name:  "Mr Racer",
		Email: s.userNames.racer2,
	})
	s.fakePagerduty.StoreOnCall(OnCall{
		User:             Reference{Type: "user_reference", ID: racer2.ID},
		EscalationPolicy: Reference{Type: "escalation_policy_reference", ID: EscalationPolicyID1},
	})
	s.fakePagerduty.StoreIncident(Incident{
		Title:   "Maintenance - linux kernel upgrade",
		Status:  "triggered",
		Service: Reference{Type: "service_reference", ID: s.pdService1.ID},
		Assignments: []IncidentAssignment{
			{Assignee: Reference{Type: "user_reference", ID: racer2.ID}},
		},
	})

	watcher, err := s.teleport.Process.GetAuthServer().NewWatcher(s.Ctx(), services.Watch{
		Kinds: []services.WatchKind{{Kind: types.KindAccessRequest}},
	})
	require.NoError(t, err)
	defer watcher.Close()
	assert.Equal(t, types.OpInit, (<-watcher.Events()).Type)

	process := lib.NewProcess(s.Ctx())
	for i := 0; i < s.raceNumber; i++ {
		userName := s.userNames.racer1
		var proposedState types.RequestState
		switch i % 2 {
		case 0:
			proposedState = types.RequestState_APPROVED
			userName = s.userNames.racer2
		case 1:
			proposedState = types.RequestState_DENIED
		}
		process.SpawnCritical(func(ctx context.Context) error {
			req, err := services.NewAccessRequest(userName, "admin")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			if err := s.teleport.CreateAccessRequest(ctx, req); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			pendingRequests.Store(req.GetName(), struct{}{})
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			incident, err := s.fakePagerduty.CheckNewIncident(ctx)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			if obtained, expected := incident.Status, "triggered"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong incident status. expected %q, obtained %q", expected, obtained))
			}
			reqID, err := getIncidentRequestID(incident)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			if _, loaded := incidentIDs.LoadOrStore(incident.ID, struct{}{}); loaded {
				return setRaceErr(trace.Errorf("incident %q has already been stored", incident.ID))
			}
			atomic.AddInt32(&incidentsCount, 1)
			req, err := s.teleport.GetAccessRequest(ctx, reqID)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			// All other requests must be resolved with either two approval reviews or two denial reviews.
			reviewsNumber := 2

			// Requests by racer2 must are auto-reviewed by plugin so only one approval is required.
			if req.GetUser() == s.userNames.racer2 {
				reviewsNumber = 1
				proposedState = types.RequestState_APPROVED
			}

			review := types.AccessReview{ProposedState: proposedState, Reason: "reviewed"}
			for j := 0; j < reviewsNumber; j++ {
				if j == 0 {
					review.Author = s.userNames.reviewer1
				} else {
					review.Author = s.userNames.reviewer2
				}
				review.Created = time.Now()
				if _, err = s.teleport.SubmitAccessReview(ctx, reqID, review); err != nil {
					return setRaceErr(trace.Wrap(err))
				}
			}
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			incident, err := s.fakePagerduty.CheckIncidentUpdate(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}
			if obtained, expected := incident.Status, "resolved"; obtained != expected {
				return setRaceErr(trace.Errorf("wrong incident status. expected %q, obtained %q", expected, obtained))
			}
			return nil
		})
	}
	for i := 0; i < 3*s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			note, err := s.fakePagerduty.CheckNewIncidentNote(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}

			var newCounter int32
			val, _ := incidentNoteCounters.LoadOrStore(note.IncidentID, &newCounter)
			counterPtr := val.(*int32)
			atomic.AddInt32(counterPtr, 1)

			return nil
		})
	}
	process.SpawnCritical(func(ctx context.Context) error {
		for {
			var event services.Event
			select {
			case event = <-watcher.Events():
			case <-ctx.Done():
				return setRaceErr(trace.Wrap(ctx.Err()))
			}
			if obtained, expected := event.Type, types.OpPut; obtained != expected {
				return setRaceErr(trace.Errorf("wrong event type. expected %v, obtained %v", expected, obtained))
			}
			if obtained, expected := event.Resource.GetKind(), types.KindAccessRequest; obtained != expected {
				return setRaceErr(trace.Errorf("wrong resource kind. expected %v, obtained %v", expected, obtained))
			}
			req := event.Resource.(services.AccessRequest)
			if req.GetState() != types.RequestState_APPROVED && req.GetState() != types.RequestState_DENIED {
				continue
			}
			resolvedRequests.Store(req.GetName(), struct{}{})
			if atomic.AddInt32(&resolvedRequestsCount, 1) == int32(s.raceNumber) {
				return nil
			}
		}
	})
	process.Terminate()
	<-process.Done()
	require.NoError(t, raceErr)

	pendingRequests.Range(func(key, _ interface{}) bool {
		_, ok := resolvedRequests.LoadAndDelete(key)
		return assert.True(t, ok)
	})

	assert.Equal(t, int32(s.raceNumber), resolvedRequestsCount)

	incidentIDs.Range(func(key, _ interface{}) bool {
		next := true

		val, ok := incidentNoteCounters.LoadAndDelete(key)
		next = next && assert.True(t, ok)
		counterPtr := val.(*int32)
		next = next && assert.Equal(t, int32(3), *counterPtr)

		return next
	})

	assert.Equal(t, int32(s.raceNumber), incidentsCount)
}

func getIncidentRequestID(incident Incident) (string, error) {
	prefix := pdIncidentKeyPrefix + "/"
	if !strings.HasPrefix(incident.IncidentKey, prefix) {
		return "", trace.Errorf("failed to parse incident_key %q", incident.IncidentKey)
	}
	return incident.IncidentKey[len(prefix):], nil
}
