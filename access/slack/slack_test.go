package main

import (
	"context"
	"os/user"
	"regexp"
	"runtime"
	"sort"
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

var msgFieldRegexp = regexp.MustCompile(`(?im)^\*([a-zA-Z ]+)\*: (.+)$`)

type SlackSuite struct {
	Suite
	appConfig Config
	userNames struct {
		requestor string
		reviewer1 string
		reviewer2 string
	}
	raceNumber int
	fakeSlack  *FakeSlack
	teleport   *integration.TeleInstance
}

func TestSlackbot(t *testing.T) { suite.Run(t, &SlackSuite{}) }

func (s *SlackSuite) SetupSuite() {
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
			Logins: []string{"guest"}, // cannot be empty
			Request: &services.AccessRequestConditions{
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
			Logins: []string{"guest"}, // cannot be empty
			ReviewRequests: &types.AccessReviewConditions{
				Roles: []string{"admin"},
			},
		},
	})
	require.NoError(t, err)
	s.userNames.reviewer1 = teleport.AddUserWithRole(me.Username+"-reviewer1@example.com", role).Username
	s.userNames.reviewer2 = teleport.AddUserWithRole(me.Username+"-reviewer2@example.com", role).Username

	role, err = types.NewRole("access-plugin", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Logins: []string{"access-plugin"}, // cannot be empty
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read"}),
				types.NewRule("access_plugin_data", []string{"update"}),
			},
		},
	})
	require.NoError(t, err)
	teleport.AddUserWithRole("plugin", role)

	err = teleport.Create(nil, nil)
	require.NoError(t, err)
	if err := teleport.Start(); err != nil {
		t.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = teleport
}

func (s *SlackSuite) SetupTest() {
	t := s.T()

	s.fakeSlack = NewFakeSlack(User{Name: "slackbot"}, s.raceNumber)
	t.Cleanup(s.fakeSlack.Close)
	s.fakeSlack.StoreUser(User{Name: "Vladimir", Profile: UserProfile{Email: s.userNames.requestor}})

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
	conf.Teleport.AuthServer = authAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.Slack.Token = "000000"
	conf.Slack.APIURL = s.fakeSlack.URL() + "/"

	s.appConfig = conf
}

func (s *SlackSuite) startApp() {
	t := s.T()
	t.Helper()

	app, err := NewApp(s.appConfig)
	require.NoError(t, err)

	s.StartApp(app)
}

func (s *SlackSuite) newAccessRequest(reviewers []User) services.AccessRequest {
	t := s.T()
	t.Helper()

	req, err := services.NewAccessRequest(s.userNames.requestor, "admin")
	require.NoError(t, err)
	req.SetRequestReason("because of")
	var suggestedReviewers []string
	for _, user := range reviewers {
		suggestedReviewers = append(suggestedReviewers, user.Profile.Email)
	}
	req.SetSuggestedReviewers(suggestedReviewers)
	return req
}

func (s *SlackSuite) createAccessRequest(reviewers []User) services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest(reviewers)
	err := s.teleport.CreateAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *SlackSuite) createExpiredAccessRequest(reviewers []User) services.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest(reviewers)
	err := s.teleport.CreateExpiredAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *SlackSuite) checkPluginData(reqID string, cond func(PluginData) bool) PluginData {
	t := s.T()
	t.Helper()

	for {
		rawData, err := s.teleport.PollAccessRequestPluginData(s.Ctx(), "slack", reqID)
		require.NoError(t, err)
		if data := DecodePluginData(rawData); cond(data) {
			return data
		}
	}
}

func (s *SlackSuite) TestMessagePosting() {
	t := s.T()

	reviewer1 := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer1}})
	reviewer2 := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer2}})

	s.startApp()
	request := s.createAccessRequest([]User{reviewer2, reviewer1})

	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return len(data.SlackData) > 0
	})
	assert.Len(t, pluginData.SlackData, 2)

	var messages []Msg
	messageSet := make(SlackDataMessageSet)
	for i := 0; i < 2; i++ {
		msg, err := s.fakeSlack.CheckNewMessage(s.Ctx())
		require.NoError(t, err)
		messageSet.Add(SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp})
		messages = append(messages, msg)
	}

	assert.Len(t, messageSet, 2)
	assert.Contains(t, messageSet, pluginData.SlackData[0])
	assert.Contains(t, messageSet, pluginData.SlackData[1])

	sort.Sort(SlackMessageSlice(messages))

	assert.Equal(t, reviewer1.ID, messages[0].Channel)
	assert.Equal(t, reviewer2.ID, messages[1].Channel)

	msgUser, err := parseMessageField(messages[0], "User")
	require.NoError(t, err)
	assert.Equal(t, s.userNames.requestor, msgUser)

	msgReason, err := parseMessageField(messages[0], "Reason")
	require.NoError(t, err)
	assert.Equal(t, "because of", msgReason)

	statusLine, err := getStatusLine(messages[0])
	require.NoError(t, err)
	assert.Equal(t, "*Status:* ⏳ PENDING", statusLine)
}

func (s *SlackSuite) TestReviewReplies() {
	t := s.T()

	reviewer := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer1}})

	s.startApp()

	req := s.createAccessRequest([]User{reviewer})
	s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return len(data.SlackData) > 0
	})

	msg, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, reviewer.ID, msg.Channel)

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	reply, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, msg.Channel, reply.Channel)
	assert.Equal(t, msg.Timestamp, reply.ThreadTs)
	assert.Contains(t, reply.Text, s.userNames.reviewer1+" reviewed the request", "reply must contain a review author")
	assert.Contains(t, reply.Text, "Resolution: ✅ APPROVED", "reply must contain a proposed state")
	assert.Contains(t, reply.Text, "Reason: okay", "reply must contain a reason")

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	reply, err = s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, msg.Channel, reply.Channel)
	assert.Equal(t, msg.Timestamp, reply.ThreadTs)
	assert.Contains(t, reply.Text, s.userNames.reviewer2+" reviewed the request", "reply must contain a review author")
	assert.Contains(t, reply.Text, "Resolution: ❌ DENIED", "reply must contain a proposed state")
	assert.Contains(t, reply.Text, "Reason: not okay", "reply must contain a reason")
}

func (s *SlackSuite) TestRecipientsConfig() {
	t := s.T()

	reviewer1 := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer1}})
	reviewer2 := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer2}})
	s.appConfig.Slack.Recipients = []string{reviewer2.Profile.Email, reviewer1.ID}

	s.startApp()

	request := s.createAccessRequest(nil)
	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return len(data.SlackData) > 0
	})
	assert.Len(t, pluginData.SlackData, 2)

	var (
		msg      Msg
		messages []Msg
	)

	messageSet := make(SlackDataMessageSet)

	msg, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	messageSet.Add(SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp})
	messages = append(messages, msg)

	msg, err = s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	messageSet.Add(SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp})
	messages = append(messages, msg)

	assert.Len(t, messageSet, 2)
	assert.Contains(t, messageSet, pluginData.SlackData[0])
	assert.Contains(t, messageSet, pluginData.SlackData[1])

	sort.Sort(SlackMessageSlice(messages))

	assert.Equal(t, reviewer1.ID, messages[0].Channel)
	assert.Equal(t, reviewer2.ID, messages[1].Channel)
}

func (s *SlackSuite) TestApproval() {
	t := s.T()

	reviewer := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer1}})

	s.startApp()

	req := s.createAccessRequest([]User{reviewer})
	msg, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, reviewer.ID, msg.Channel)

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	reply, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, msg.Channel, reply.Channel)
	assert.Equal(t, msg.Timestamp, reply.ThreadTs)
	assert.Contains(t, reply.Text, s.userNames.reviewer1+" reviewed the request", "reply must contain a review author")

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "finally okay",
	})
	require.NoError(t, err)

	reply, err = s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, msg.Channel, reply.Channel)
	assert.Equal(t, msg.Timestamp, reply.ThreadTs)
	assert.Contains(t, reply.Text, s.userNames.reviewer2+" reviewed the request", "reply must contain a review author")

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByAPI(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, reviewer.ID, msgUpdate.Channel)
	assert.Equal(t, msg.Timestamp, msgUpdate.Timestamp)

	statusLine, err := getStatusLine(msgUpdate)
	require.NoError(t, err)
	assert.Equal(t, "*Status:* ✅ APPROVED (finally okay)", statusLine)
}

func (s *SlackSuite) TestDenial() {
	t := s.T()

	reviewer := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer1}})

	s.startApp()

	req := s.createAccessRequest([]User{reviewer})
	msg, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, reviewer.ID, msg.Channel)

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	reply, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, msg.Channel, reply.Channel)
	assert.Equal(t, msg.Timestamp, reply.ThreadTs)
	assert.Contains(t, reply.Text, s.userNames.reviewer1+" reviewed the request", "reply must contain a review author")

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "finally not okay",
	})
	require.NoError(t, err)

	reply, err = s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, msg.Channel, reply.Channel)
	assert.Equal(t, msg.Timestamp, reply.ThreadTs)
	assert.Contains(t, reply.Text, s.userNames.reviewer2+" reviewed the request", "reply must contain a review author")

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByAPI(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, reviewer.ID, msgUpdate.Channel)
	assert.Equal(t, msg.Timestamp, msgUpdate.Timestamp)

	statusLine, err := getStatusLine(msgUpdate)
	require.NoError(t, err)
	assert.Equal(t, "*Status:* ❌ DENIED (finally not okay)", statusLine)
}

func (s *SlackSuite) TestExpiration() {
	t := s.T()

	reviewer := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer1}})

	s.startApp()

	s.createExpiredAccessRequest([]User{reviewer})
	msg, err := s.fakeSlack.CheckNewMessage(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, reviewer.ID, msg.Channel)

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByAPI(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, reviewer.ID, msgUpdate.Channel)
	assert.Equal(t, msg.Timestamp, msgUpdate.Timestamp)

	statusLine, err := getStatusLine(msgUpdate)
	require.NoError(t, err)
	assert.Equal(t, "*Status:* ⌛ EXPIRED", statusLine)
}

func (s *SlackSuite) TestRace() {
	t := s.T()

	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	reviewer1 := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer1}})
	reviewer2 := s.fakeSlack.StoreUser(User{Profile: UserProfile{Email: s.userNames.reviewer2}})

	s.SetContext(20 * time.Second)
	s.startApp()

	var (
		raceErr             error
		raceErrOnce         sync.Once
		threadMsgIDs        sync.Map
		threadMsgsCount     int32
		msgUpdateCounters   sync.Map
		reviewReplyCounters sync.Map
	)
	setRaceErr := func(err error) error {
		raceErrOnce.Do(func() {
			raceErr = err
		})
		return err
	}

	process := lib.NewProcess(s.Ctx())
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			req, err := services.NewAccessRequest(s.userNames.requestor, "admin")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			req.SetSuggestedReviewers([]string{reviewer1.Profile.Email, reviewer2.Profile.Email})
			if err := s.teleport.CreateAccessRequest(ctx, req); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
	}

	// Having TWO suggested reviewers will post TWO messages for each request.
	// We also have approval threshold of TWO set in the role properties
	// so lets simply submit the approval from each of the suggested reviewers.
	//
	// Multiplier SIX means that we handle TWO messages for each request and also
	// TWO comments for each message: 2 * (1 message + 2 comments).
	for i := 0; i < 6*s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			msg, err := s.fakeSlack.CheckNewMessage(ctx)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			if msg.ThreadTs == "" {
				// Handle "root" notifications.

				threadMsgKey := SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp}
				if _, loaded := threadMsgIDs.LoadOrStore(threadMsgKey, struct{}{}); loaded {
					return setRaceErr(trace.Errorf("thread %v already stored", threadMsgKey))
				}
				atomic.AddInt32(&threadMsgsCount, 1)

				user, ok := s.fakeSlack.GetUser(msg.Channel)
				if !ok {
					return setRaceErr(trace.Errorf("user %q is not found", msg.Channel))
				}

				reqID, err := parseMessageField(msg, "ID")
				if err != nil {
					return setRaceErr(trace.Wrap(err))
				}

				if _, err = s.teleport.SubmitAccessReview(ctx, reqID, types.AccessReview{
					Author:        user.Profile.Email,
					ProposedState: types.RequestState_APPROVED,
					Created:       time.Now(),
					Reason:        "okay",
				}); err != nil {
					return setRaceErr(trace.Wrap(err))
				}
			} else {
				// Handle review comments.

				threadMsgKey := SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.ThreadTs}
				var newCounter int32
				val, _ := reviewReplyCounters.LoadOrStore(threadMsgKey, &newCounter)
				counterPtr := val.(*int32)
				atomic.AddInt32(counterPtr, 1)
			}

			return nil
		})
	}

	// Multiplier TWO means that we handle updates for each of the two messages posted to reviewers.
	for i := 0; i < 2*s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			msg, err := s.fakeSlack.CheckMessageUpdateByAPI(ctx)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			threadMsgKey := SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp}
			var newCounter int32
			val, _ := msgUpdateCounters.LoadOrStore(threadMsgKey, &newCounter)
			counterPtr := val.(*int32)
			atomic.AddInt32(counterPtr, 1)

			return nil
		})
	}

	process.Terminate()
	<-process.Done()
	require.NoError(t, raceErr)

	assert.Equal(t, int32(2*s.raceNumber), threadMsgsCount)
	threadMsgIDs.Range(func(key, value interface{}) bool {
		next := true

		val, loaded := reviewReplyCounters.LoadAndDelete(key)
		next = next && assert.True(t, loaded)
		counterPtr := val.(*int32)
		next = next && assert.Equal(t, int32(2), *counterPtr)

		val, loaded = msgUpdateCounters.LoadAndDelete(key)
		next = next && assert.True(t, loaded)
		counterPtr = val.(*int32)
		next = next && assert.Equal(t, int32(1), *counterPtr)

		return next
	})
}

func parseMessageField(msg Msg, field string) (string, error) {
	block := msg.BlockItems[1].Block
	sectionBlock, ok := block.(SectionBlock)
	if !ok {
		return "", trace.Errorf("invalid block type %T", block)
	}

	if sectionBlock.Text.TextObject == nil {
		return "", trace.Errorf("section block does not contain text")
	}

	text := sectionBlock.Text.GetText()
	matches := msgFieldRegexp.FindAllStringSubmatch(text, -1)
	if matches == nil {
		return "", trace.Errorf("cannot parse fields from text %q", text)
	}
	var fields []string
	for _, match := range matches {
		if match[1] == field {
			return match[2], nil
		}
		fields = append(fields, match[1])
	}
	return "", trace.Errorf("cannot find field %q in %v", field, fields)
}

func getStatusLine(msg Msg) (string, error) {
	block := msg.BlockItems[2].Block
	contextBlock, ok := block.(ContextBlock)
	if !ok {
		return "", trace.Errorf("invalid block type %T", block)
	}

	elementItems := contextBlock.ElementItems
	if n := len(elementItems); n != 1 {
		return "", trace.Errorf("expected only one context element, got %v", n)
	}

	element := elementItems[0].ContextElement
	textBlock, ok := element.(TextObject)
	if !ok {
		return "", trace.Errorf("invalid element type %T", element)
	}

	return textBlock.GetText(), nil
}
