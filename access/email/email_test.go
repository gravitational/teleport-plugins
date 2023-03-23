/*
Copyright 2022 Gravitational, Inc.

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
	"fmt"
	"os/user"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/integrations/lib"
	"github.com/gravitational/teleport/integrations/lib/logger"
	"github.com/gravitational/teleport/integrations/lib/testing/integration"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// sender default message sender
	sender = "noreply@example.com"
	// allRecipient is a recipient for all messages sent
	allRecipient = "all@example.com"
	// mailgunMockPrivateKey private key for mock mailgun
	mailgunMockPrivateKey = "000000"
	// mailgunMockDomain domain for mock mailgun
	mailgunMockDomain = "test.example.com"
	// subjectIDSubstring indicates start of request id
	subjectIDSubstring = "Role Request "
	// newMessageCount number of original emails
	newMessageCount = 3
	// reviewMessageCount nubmer of review emails per thread
	reviewMessageCount = 6
	// resolveMessageCount number of resolve emails per thread
	resolveMessageCount = 3
	// messageCountPerThread number of total messages per thread
	messageCountPerThread = newMessageCount + reviewMessageCount + resolveMessageCount
)

type EmailSuite struct {
	integration.AuthSetup
	appConfig Config
	userNames struct {
		ruler     string
		requestor string
		reviewer1 string
		reviewer2 string
		plugin    string
	}
	raceNumber  int
	mockMailgun *MockMailgunServer

	clients          map[string]*integration.Client
	teleportFeatures *proto.Features
	teleportConfig   lib.TeleportConfig
}

func TestEmailClient(t *testing.T) { suite.Run(t, &EmailSuite{}) }

func (s *EmailSuite) SetupSuite() {
	var err error
	t := s.T()

	s.AuthSetup.SetupSuite(t)
	s.AuthSetup.SetupService()

	ctx := s.Context()

	s.raceNumber = runtime.GOMAXPROCS(0)
	me, err := user.Current()
	require.NoError(t, err)

	s.clients = make(map[string]*integration.Client)

	// Set up the user who has an access to all kinds of resources.

	s.userNames.ruler = me.Username + "-ruler@example.com"
	client, err := s.Integration.MakeAdmin(s.Context(), s.Auth, s.userNames.ruler)
	require.NoError(t, err)
	s.clients[s.userNames.ruler] = client

	// Get the server features.
	pong, err := client.Ping(s.Context())
	require.NoError(t, err)
	teleportFeatures := pong.GetServerFeatures()

	var bootstrap integration.Bootstrap

	// Set up user who can request the access to role "editor".

	conditions := types.RoleConditions{Request: &types.AccessRequestConditions{Roles: []string{"editor"}}}
	if teleportFeatures.AdvancedAccessWorkflows {
		conditions.Request.Thresholds = []types.AccessReviewThreshold{{Approve: 2, Deny: 2}}
	}
	role, err := bootstrap.AddRole("foo", types.RoleSpecV6{Allow: conditions})
	require.NoError(t, err)

	user, err := bootstrap.AddUserWithRoles(me.Username+"@example.com", role.GetName())
	require.NoError(t, err)
	s.userNames.requestor = user.GetName()

	// Set up TWO users who can review access requests to role "editor".

	conditions = types.RoleConditions{}
	if teleportFeatures.AdvancedAccessWorkflows {
		conditions.ReviewRequests = &types.AccessReviewConditions{Roles: []string{"editor"}}
	}
	role, err = bootstrap.AddRole("foo-reviewer", types.RoleSpecV6{Allow: conditions})
	require.NoError(t, err)

	user, err = bootstrap.AddUserWithRoles(me.Username+"-reviewer1@example.com", role.GetName())
	require.NoError(t, err)
	s.userNames.reviewer1 = user.GetName()

	user, err = bootstrap.AddUserWithRoles(me.Username+"-reviewer2@example.com", role.GetName())
	require.NoError(t, err)
	s.userNames.reviewer2 = user.GetName()

	// Set up plugin user.

	role, err = bootstrap.AddRole("access-email", types.RoleSpecV6{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read"}),
				types.NewRule("access_plugin_data", []string{"update"}),
			},
		},
	})
	require.NoError(t, err)

	user, err = bootstrap.AddUserWithRoles("access-email", role.GetName())
	require.NoError(t, err)
	s.userNames.plugin = user.GetName()

	// Bake all the resources.

	err = s.Integration.Bootstrap(ctx, s.Auth, bootstrap.Resources())
	require.NoError(t, err)

	// Initialize the clients.

	client, err = s.Integration.NewClient(ctx, s.Auth, s.userNames.requestor)
	require.NoError(t, err)
	s.clients[s.userNames.requestor] = client

	if teleportFeatures.AdvancedAccessWorkflows {
		client, err = s.Integration.NewClient(ctx, s.Auth, s.userNames.reviewer1)
		require.NoError(t, err)
		s.clients[s.userNames.reviewer1] = client

		client, err = s.Integration.NewClient(ctx, s.Auth, s.userNames.reviewer2)
		require.NoError(t, err)
		s.clients[s.userNames.reviewer2] = client
	}

	identityPath, err := s.Integration.Sign(ctx, s.Auth, s.userNames.plugin)
	require.NoError(t, err)

	s.teleportConfig.Addr = s.Auth.AuthAddr().String()
	s.teleportConfig.Identity = identityPath
	s.teleportFeatures = teleportFeatures
}

func (s *EmailSuite) SetupTest() {
	t := s.T()

	err := logger.Setup(logger.Config{Severity: "debug"})
	require.NoError(t, err)

	s.mockMailgun = NewMockMailgunServer(s.raceNumber)
	s.mockMailgun.Start()
	t.Cleanup(s.mockMailgun.Stop)

	var conf Config
	conf.Teleport = s.teleportConfig
	conf.Mailgun = &MailgunConfig{
		PrivateKey: mailgunMockPrivateKey,
		Domain:     mailgunMockDomain,
		APIBase:    s.mockMailgun.GetURL(),
	}
	conf.Delivery.Sender = sender
	conf.RoleToRecipients = map[string][]string{
		types.Wildcard: {allRecipient},
	}

	s.appConfig = conf
	s.SetContextTimeout(5 * time.Minute)

	s.startApp()
}

func (s *EmailSuite) startApp() {
	t := s.T()
	t.Helper()

	app, err := NewApp(s.appConfig)
	require.NoError(t, err)

	s.StartApp(app)
}

func (s *EmailSuite) ruler() *integration.Client {
	return s.clients[s.userNames.ruler]
}

func (s *EmailSuite) requestor() *integration.Client {
	return s.clients[s.userNames.requestor]
}

func (s *EmailSuite) reviewer1() *integration.Client {
	return s.clients[s.userNames.reviewer1]
}

func (s *EmailSuite) reviewer2() *integration.Client {
	return s.clients[s.userNames.reviewer2]
}

func (s *EmailSuite) newAccessRequest(suggestedReviewers []string) types.AccessRequest {
	t := s.T()
	t.Helper()

	req, err := types.NewAccessRequest(uuid.New().String(), s.userNames.requestor, "editor")
	require.NoError(t, err)
	req.SetRequestReason("because of")
	req.SetSuggestedReviewers(suggestedReviewers)

	return req
}

func (s *EmailSuite) createAccessRequest(suggestedReviewers []string) types.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest(suggestedReviewers)
	err := s.requestor().CreateAccessRequest(s.Context(), req)
	require.NoError(t, err)
	return req
}

func (s *EmailSuite) checkPluginData(reqID string, cond func(PluginData) bool) PluginData {
	t := s.T()
	t.Helper()

	for {
		rawData, err := s.ruler().PollAccessRequestPluginData(s.Context(), "email", reqID)
		require.NoError(t, err)
		if data := DecodePluginData(rawData); cond(data) {
			return data
		}
	}
}

func (s *EmailSuite) TestNewThreads() {
	t := s.T()

	request := s.createAccessRequest([]string{s.userNames.reviewer1, s.userNames.reviewer2})

	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return len(data.EmailThreads) > 0
	})
	require.Len(t, pluginData.EmailThreads, 3) // 2 recipients + all@example.com

	var messages = s.getMessages(s.Context(), t, 3)

	require.Len(t, messages, 3)

	// Senders
	require.Equal(t, sender, messages[0].Sender)
	require.Equal(t, sender, messages[1].Sender)
	require.Equal(t, sender, messages[2].Sender)

	// Recipients
	expectedRecipients := []string{allRecipient, s.userNames.reviewer1, s.userNames.reviewer2}
	actualRecipients := []string{messages[0].Recipient, messages[1].Recipient, messages[2].Recipient}
	sort.Strings(expectedRecipients)
	sort.Strings(actualRecipients)

	require.Equal(t, expectedRecipients, actualRecipients)

	// Subjects
	require.Contains(t, messages[0].Subject, request.GetName())
	require.Contains(t, messages[1].Subject, request.GetName())
	require.Contains(t, messages[2].Subject, request.GetName())

	// Body
	require.Contains(t, messages[0].Body, fmt.Sprintf("User: %v", s.userNames.requestor))
	require.Contains(t, messages[1].Body, "Reason: because of")
	require.Contains(t, messages[2].Body, "Status: ⏳ PENDING")
}

func (s *EmailSuite) TestApproval() {
	t := s.T()

	req := s.createAccessRequest([]string{s.userNames.reviewer1})

	s.skipMessages(s.Context(), t, 2)

	err := s.ruler().ApproveAccessRequest(s.Context(), req.GetName(), "okay")
	require.NoError(t, err)

	messages := s.getMessages(s.Context(), t, 2)

	recipients := []string{messages[0].Recipient, messages[1].Recipient}

	require.Contains(t, recipients, allRecipient)
	require.Contains(t, recipients, s.userNames.reviewer1)

	require.Contains(t, messages[0].Body, "Status: ✅ APPROVED (okay)")
}

func (s *EmailSuite) TestDenial() {
	t := s.T()

	req := s.createAccessRequest([]string{s.userNames.reviewer1})

	s.skipMessages(s.Context(), t, 2)

	err := s.ruler().DenyAccessRequest(s.Context(), req.GetName(), "not okay")
	require.NoError(t, err)

	messages := s.getMessages(s.Context(), t, 2)

	recipients := []string{messages[0].Recipient, messages[1].Recipient}

	require.Contains(t, recipients, allRecipient)
	require.Contains(t, recipients, s.userNames.reviewer1)

	require.Contains(t, messages[0].Body, "Status: ❌ DENIED (not okay)")
}

func (s *EmailSuite) TestReviewReplies() {
	t := s.T()

	if !s.teleportFeatures.AdvancedAccessWorkflows {
		t.Skip("Doesn't work in OSS version")
	}

	req := s.createAccessRequest([]string{s.userNames.reviewer1})
	s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return len(data.EmailThreads) > 0
	})

	s.skipMessages(s.Context(), t, 2)

	err := s.reviewer1().SubmitAccessRequestReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	messages := s.getMessages(s.Context(), t, 2)

	reply := messages[0].Body

	require.Contains(t, reply, s.userNames.reviewer1+" reviewed the request", "reply must contain a review author")
	require.Contains(t, reply, "Resolution: ✅ APPROVED", "reply must contain a proposed state")
	require.Contains(t, reply, "Reason: okay", "reply must contain a reason")

	err = s.reviewer2().SubmitAccessRequestReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	messages = s.getMessages(s.Context(), t, 2)

	reply = messages[0].Body

	require.Contains(t, reply, s.userNames.reviewer2+" reviewed the request", "reply must contain a review author")
	require.Contains(t, reply, "Resolution: ❌ DENIED", "reply must contain a proposed state")
	require.Contains(t, reply, "Reason: not okay", "reply must contain a reason")
}

func (s *EmailSuite) TestApprovalByReview() {
	t := s.T()

	if !s.teleportFeatures.AdvancedAccessWorkflows {
		t.Skip("Doesn't work in OSS version")
	}

	req := s.createAccessRequest([]string{s.userNames.reviewer2})

	s.skipMessages(s.Context(), t, 2)

	err := s.reviewer1().SubmitAccessRequestReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	messages := s.getMessages(s.Context(), t, 2)

	require.Contains(t, messages[0].Body, s.userNames.reviewer1+" reviewed the request", "reply must contain a review author")

	err = s.reviewer2().SubmitAccessRequestReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "finally okay",
	})
	require.NoError(t, err)

	messages = s.getMessages(s.Context(), t, 2)
	require.Contains(t, messages[0].Body, s.userNames.reviewer2+" reviewed the request", "reply must contain a review author")

	messages = s.getMessages(s.Context(), t, 2)
	require.Contains(t, messages[0].Body, "Status: ✅ APPROVED (finally okay)")
}

func (s *EmailSuite) TestDenialByReview() {
	t := s.T()

	if !s.teleportFeatures.AdvancedAccessWorkflows {
		t.Skip("Doesn't work in OSS version")
	}

	req := s.createAccessRequest([]string{s.userNames.requestor})

	s.skipMessages(s.Context(), t, 2)

	err := s.reviewer1().SubmitAccessRequestReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	messages := s.getMessages(s.Context(), t, 2)
	require.Contains(t, messages[0].Body, s.userNames.reviewer1+" reviewed the request", "reply must contain a review author")

	err = s.reviewer2().SubmitAccessRequestReview(s.Context(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "finally not okay",
	})
	require.NoError(t, err)

	messages = s.getMessages(s.Context(), t, 2)
	require.Contains(t, messages[0].Body, s.userNames.reviewer2+" reviewed the request", "reply must contain a review author")

	messages = s.getMessages(s.Context(), t, 2)
	require.Contains(t, messages[0].Body, "Status: ❌ DENIED (finally not okay)")
}

func (s *EmailSuite) TestExpiration() {
	t := s.T()

	request := s.createAccessRequest([]string{s.userNames.requestor})
	s.skipMessages(s.Context(), t, 2)

	s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return len(data.EmailThreads) > 0
	})

	err := s.ruler().DeleteAccessRequest(s.Context(), request.GetName()) // simulate expiration
	require.NoError(t, err)

	messages := s.getMessages(s.Context(), t, 2)
	require.Contains(t, messages[0].Body, "Status: ⌛ EXPIRED")
}

func (s *EmailSuite) TestRace() {
	t := s.T()

	if !s.teleportFeatures.AdvancedAccessWorkflows {
		t.Skip("Doesn't work in OSS version")
	}

	err := logger.Setup(logger.Config{Severity: "info"}) // Turn off noisy debug logging
	require.NoError(t, err)

	s.SetContextTimeout(20 * time.Second)

	var (
		raceErr     error
		raceErrOnce sync.Once
		msgIDs      sync.Map
		msgCount    int32
		threadIDs   sync.Map
		replyIDs    sync.Map
		resolveIDs  sync.Map
	)
	setRaceErr := func(err error) error {
		raceErrOnce.Do(func() {
			raceErr = err
		})
		return err
	}
	incCounter := func(m *sync.Map, id string) {
		var newCounter int32
		val, _ := m.LoadOrStore(id, &newCounter)
		counterPtr := val.(*int32)
		atomic.AddInt32(counterPtr, 1)
	}

	process := lib.NewProcess(s.Context())
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			req, err := types.NewAccessRequest(uuid.New().String(), s.userNames.requestor, "editor")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			req.SetSuggestedReviewers([]string{s.userNames.reviewer1, s.userNames.reviewer2})
			if err := s.requestor().CreateAccessRequest(ctx, req); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
	}

	// 3 original messages + 2*3 reviews + 3 resolve
	for i := 0; i < messageCountPerThread*s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			msg, err := s.mockMailgun.GetMessage(ctx)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			if _, loaded := msgIDs.LoadOrStore(msg.ID, struct{}{}); loaded {
				return setRaceErr(trace.Errorf("message %v already stored", msg.ID))
			}
			atomic.AddInt32(&msgCount, 1)

			reqID := s.extractRequestID(msg.Subject)

			// Handle thread creation notifications
			if strings.Contains(msg.Body, "You have a new Role Request") {
				incCounter(&threadIDs, reqID)

				// We must approve message if it's not an all recipient
				if msg.Recipient != allRecipient {
					if err = s.clients[msg.Recipient].SubmitAccessRequestReview(ctx, reqID, types.AccessReview{
						Author:        msg.Recipient,
						ProposedState: types.RequestState_APPROVED,
						Created:       time.Now(),
						Reason:        "okay",
					}); err != nil {
						return setRaceErr(trace.Wrap(err))
					}
				}
			} else if strings.Contains(msg.Body, "reviewed the request") { // Review
				incCounter(&replyIDs, reqID)
			} else if strings.Contains(msg.Body, "has been resolved") { // Resolution
				incCounter(&resolveIDs, reqID)
			}

			return nil
		})
	}

	process.Terminate()
	<-process.Done()
	require.NoError(t, raceErr)

	threadIDs.Range(func(key, value interface{}) bool {
		next := true

		val, loaded := threadIDs.LoadAndDelete(key)
		next = next && assert.True(t, loaded)

		c, ok := val.(*int32)
		require.True(t, ok)
		require.Equal(t, int32(newMessageCount), *c)

		return next
	})

	replyIDs.Range(func(key, value interface{}) bool {
		next := true

		val, loaded := replyIDs.LoadAndDelete(key)
		next = next && assert.True(t, loaded)

		c, ok := val.(*int32)
		require.True(t, ok)
		require.Equal(t, int32(reviewMessageCount), *c)

		return next
	})

	resolveIDs.Range(func(key, value interface{}) bool {
		next := true

		val, loaded := resolveIDs.LoadAndDelete(key)
		next = next && assert.True(t, loaded)

		c, ok := val.(*int32)
		require.True(t, ok)
		require.Equal(t, int32(resolveMessageCount), *c)

		return next
	})

	// Total message count:
	// (3 original threads + 6 review replies + 3 * resolve) * number of processes
	require.Equal(t, int32(messageCountPerThread*s.raceNumber), msgCount)
}

// skipEmails ensures that emails were received, but dumps the contents
func (s *EmailSuite) skipMessages(ctx context.Context, t *testing.T, n int) {
	for i := 0; i < n; i++ {
		_, err := s.mockMailgun.GetMessage(ctx)
		require.NoError(t, err)
	}
}

// getMessages returns next n email messages
func (s *EmailSuite) getMessages(ctx context.Context, t *testing.T, n int) []MockMailgunMessage {
	messages := make([]MockMailgunMessage, n)
	for i := 0; i < n; i++ {
		m, err := s.mockMailgun.GetMessage(ctx)
		require.NoError(t, err)
		messages[i] = m
	}

	return messages
}

// extractRequestID extracts request id from a subject
func (s *EmailSuite) extractRequestID(subject string) string {
	idx := strings.Index(subject, subjectIDSubstring)
	return subject[idx+len(subjectIDSubstring):]
}
