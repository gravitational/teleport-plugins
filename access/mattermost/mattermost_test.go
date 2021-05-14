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

var msgFieldRegexp = regexp.MustCompile(`(?im)^\*\*([a-zA-Z ]+)\*\*:\ +(.+)$`)

type MattermostSuite struct {
	Suite
	appConfig Config
	userNames struct {
		requestor string
		reviewer1 string
		reviewer2 string
	}
	raceNumber     int
	fakeMattermost *FakeMattermost
	mmUser         User
	teleport       *integration.TeleInstance
}

func TestMattermost(t *testing.T) { suite.Run(t, &MattermostSuite{}) }

func (s *MattermostSuite) SetupSuite() {
	t := s.T()

	var err error
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

func (s *MattermostSuite) SetupTest() {
	t := s.T()

	s.fakeMattermost = NewFakeMattermost(User{Username: "bot", Email: "bot@example.com"}, s.raceNumber)
	t.Cleanup(s.fakeMattermost.Close)

	s.mmUser = s.fakeMattermost.StoreUser(User{
		FirstName: "User",
		LastName:  "Test",
		Username:  "Vladimir",
		Email:     s.userNames.requestor,
	})

	auth := s.teleport.Process.GetAuthServer()
	certAuthorities, err := auth.GetCertAuthorities(services.HostCA, false)
	require.NoError(t, err)
	pluginKey := s.teleport.Secrets.Users["plugin"].Key

	keyFile := s.NewTmpFile("auth.*.key")
	_, err = keyFile.Write(pluginKey.Priv)
	require.NoError(t, err)
	require.NoError(t, keyFile.Close())

	certFile := s.NewTmpFile("auth.*.crt")
	_, err = certFile.Write(pluginKey.TLSCert)
	require.NoError(t, err)
	require.NoError(t, certFile.Close())

	casFile := s.NewTmpFile("auth.*.cas")
	for _, ca := range certAuthorities {
		for _, keyPair := range ca.GetTLSKeyPairs() {
			_, err = casFile.Write(keyPair.Cert)
			require.NoError(t, err)
		}
	}
	require.NoError(t, casFile.Close())

	authAddr, err := s.teleport.Process.AuthSSHAddr()
	require.NoError(t, err)

	var conf Config
	conf.Teleport.AuthServer = authAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.Mattermost.URL = s.fakeMattermost.URL()

	s.appConfig = conf
}

func (s *MattermostSuite) startApp() {
	t := s.T()
	t.Helper()

	app, err := NewApp(s.appConfig)
	require.NoError(t, err)

	s.StartApp(app)
}

func (s *MattermostSuite) newAccessRequest(reviewers []User) types.AccessRequest {
	t := s.T()
	t.Helper()

	req, err := services.NewAccessRequest(s.userNames.requestor, "admin")
	require.NoError(t, err)
	req.SetRequestReason("because of")
	var suggestedReviewers []string
	for _, user := range reviewers {
		suggestedReviewers = append(suggestedReviewers, user.Email)
	}
	req.SetSuggestedReviewers(suggestedReviewers)
	return req
}

func (s *MattermostSuite) createAccessRequest(reviewers []User) types.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest(reviewers)
	err := s.teleport.CreateAccessRequest(s.Ctx(), req)
	require.NoError(s.T(), err)
	return req
}

func (s *MattermostSuite) createExpiredAccessRequest(reviewers []User) types.AccessRequest {
	t := s.T()
	t.Helper()

	req := s.newAccessRequest(reviewers)
	err := s.teleport.CreateExpiredAccessRequest(s.Ctx(), req)
	require.NoError(t, err)
	return req
}

func (s *MattermostSuite) checkPluginData(reqID string, cond func(PluginData) bool) PluginData {
	t := s.T()
	t.Helper()

	for {
		rawData, err := s.teleport.PollAccessRequestPluginData(s.Ctx(), "mattermost", reqID)
		require.NoError(t, err)
		if data := DecodePluginData(rawData); cond(data) {
			return data
		}
	}
}

func (s *MattermostSuite) TestMattermostMessagePosting() {
	t := s.T()

	reviewer1 := s.fakeMattermost.StoreUser(User{Email: s.userNames.reviewer1})
	reviewer2 := s.fakeMattermost.StoreUser(User{Email: s.userNames.reviewer2})
	directChannel1 := s.fakeMattermost.GetDirectChannelFor(s.fakeMattermost.GetBotUser(), reviewer1)
	directChannel2 := s.fakeMattermost.GetDirectChannelFor(s.fakeMattermost.GetBotUser(), reviewer2)

	s.startApp()
	request := s.createAccessRequest([]User{reviewer2, reviewer1})

	pluginData := s.checkPluginData(request.GetName(), func(data PluginData) bool {
		return len(data.MattermostData) > 0
	})
	assert.Len(t, pluginData.MattermostData, 2)

	var posts []Post
	postSet := make(MattermostDataPostSet)
	for i := 0; i < 2; i++ {
		post, err := s.fakeMattermost.CheckNewPost(s.Ctx())
		require.NoError(t, err, "no new messages posted")
		postSet.Add(MattermostDataPost{ChannelID: post.ChannelID, PostID: post.ID})
		posts = append(posts, post)
	}

	assert.Len(t, postSet, 2)
	assert.Contains(t, postSet, pluginData.MattermostData[0])
	assert.Contains(t, postSet, pluginData.MattermostData[1])

	sort.Sort(MattermostPostSlice(posts))

	assert.Equal(t, directChannel1.ID, posts[0].ChannelID)
	assert.Equal(t, directChannel2.ID, posts[1].ChannelID)

	post := posts[0]
	reqID, err := parsePostField(post, "Request ID")
	require.NoError(t, err)
	assert.Equal(t, request.GetName(), reqID)

	username, err := parsePostField(post, "User")
	require.NoError(t, err)
	assert.Equal(t, s.userNames.requestor, username)

	reason, err := parsePostField(post, "Reason")
	require.NoError(t, err)
	assert.Equal(t, "because of", reason)

	statusLine, err := parsePostField(post, "Status")
	require.NoError(t, err)
	assert.Equal(t, "⏳ PENDING", statusLine)
}

func (s *MattermostSuite) TestReviewComments() {
	t := s.T()

	reviewer := s.fakeMattermost.StoreUser(User{Email: s.userNames.reviewer1})
	directChannelID := s.fakeMattermost.GetDirectChannelFor(s.fakeMattermost.GetBotUser(), reviewer).ID

	s.startApp()

	req := s.createAccessRequest([]User{reviewer})
	s.checkPluginData(req.GetName(), func(data PluginData) bool {
		return len(data.MattermostData) > 0
	})

	post, err := s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, directChannelID, post.ChannelID)

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	comment, err := s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, post.ChannelID, comment.ChannelID)
	assert.Equal(t, post.ID, comment.RootID)
	assert.Contains(t, comment.Message, s.userNames.reviewer1+" reviewed the request", "comment must contain a review author")
	assert.Contains(t, comment.Message, "Resolution: ✅ APPROVED", "comment must contain a proposed state")
	assert.Contains(t, comment.Message, "Reason: okay", "comment must contain a reason")

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	comment, err = s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, post.ChannelID, comment.ChannelID)
	assert.Equal(t, post.ID, comment.RootID)
	assert.Contains(t, comment.Message, s.userNames.reviewer2+" reviewed the request", "comment must contain a review author")
	assert.Contains(t, comment.Message, "Resolution: ❌ DENIED", "comment must contain a proposed state")
	assert.Contains(t, comment.Message, "Reason: not okay", "comment must contain a reason")
}

func (s *MattermostSuite) TestApproval() {
	t := s.T()

	reviewer := s.fakeMattermost.StoreUser(User{Email: s.userNames.reviewer1})

	s.startApp()

	req := s.createAccessRequest([]User{reviewer})
	post, err := s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err, "no new messages posted")
	directChannelID := s.fakeMattermost.GetDirectChannelFor(s.fakeMattermost.GetBotUser(), reviewer).ID
	assert.Equal(t, directChannelID, post.ChannelID)

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "okay",
	})
	require.NoError(t, err)

	comment, err := s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, post.ChannelID, comment.ChannelID)
	assert.Equal(t, post.ID, comment.RootID)
	assert.Contains(t, comment.Message, s.userNames.reviewer1+" reviewed the request", "comment must contain a review author")

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_APPROVED,
		Created:       time.Now(),
		Reason:        "finally okay",
	})
	require.NoError(t, err)

	comment, err = s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, post.ChannelID, comment.ChannelID)
	assert.Equal(t, post.ID, comment.RootID)
	assert.Contains(t, comment.Message, s.userNames.reviewer2+" reviewed the request", "comment must contain a review author")

	postUpdate, err := s.fakeMattermost.CheckPostUpdate(s.Ctx())
	require.NoError(t, err, "no messages updated")
	assert.Equal(t, post.ID, postUpdate.ID)
	assert.Equal(t, post.ChannelID, postUpdate.ChannelID)

	statusLine, err := parsePostField(postUpdate, "Status")
	require.NoError(t, err)
	assert.Equal(t, "✅ APPROVED (finally okay)", statusLine)
}

func (s *MattermostSuite) TestDenial() {
	t := s.T()

	reviewer := s.fakeMattermost.StoreUser(User{Email: s.userNames.reviewer1})

	s.startApp()

	req := s.createAccessRequest([]User{reviewer})
	post, err := s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err, "no new messages posted")
	directChannelID := s.fakeMattermost.GetDirectChannelFor(s.fakeMattermost.GetBotUser(), reviewer).ID
	assert.Equal(t, directChannelID, post.ChannelID)

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer1,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "not okay",
	})
	require.NoError(t, err)

	comment, err := s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, post.ChannelID, comment.ChannelID)
	assert.Equal(t, post.ID, comment.RootID)
	assert.Contains(t, comment.Message, s.userNames.reviewer1+" reviewed the request", "comment must contain a review author")

	s.teleport.SubmitAccessReview(s.Ctx(), req.GetName(), types.AccessReview{
		Author:        s.userNames.reviewer2,
		ProposedState: types.RequestState_DENIED,
		Created:       time.Now(),
		Reason:        "finally not okay",
	})
	require.NoError(t, err)

	comment, err = s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err)
	assert.Equal(t, post.ChannelID, comment.ChannelID)
	assert.Equal(t, post.ID, comment.RootID)
	assert.Contains(t, comment.Message, s.userNames.reviewer2+" reviewed the request", "comment must contain a review author")

	postUpdate, err := s.fakeMattermost.CheckPostUpdate(s.Ctx())
	require.NoError(t, err, "no messages updated")
	assert.Equal(t, post.ID, postUpdate.ID)
	assert.Equal(t, post.ChannelID, postUpdate.ChannelID)

	statusLine, err := parsePostField(postUpdate, "Status")
	require.NoError(t, err)
	assert.Equal(t, "❌ DENIED (finally not okay)", statusLine)
}

func (s *MattermostSuite) TestExpiration() {
	t := s.T()

	reviewer := s.fakeMattermost.StoreUser(User{Email: "user@example.com"})

	s.startApp()
	s.createExpiredAccessRequest([]User{reviewer})

	post, err := s.fakeMattermost.CheckNewPost(s.Ctx())
	require.NoError(t, err, "no new messages posted")
	directChannelID := s.fakeMattermost.GetDirectChannelFor(s.fakeMattermost.GetBotUser(), reviewer).ID
	assert.Equal(t, directChannelID, post.ChannelID)

	postUpdate, err := s.fakeMattermost.CheckPostUpdate(s.Ctx())
	require.NoError(t, err, "no new messages updated")
	assert.Equal(t, post.ID, postUpdate.ID)
	assert.Equal(t, post.ChannelID, postUpdate.ChannelID)

	statusLine, err := parsePostField(postUpdate, "Status")
	require.NoError(t, err)
	assert.Equal(t, "⌛ EXPIRED", statusLine)
}

func (s *MattermostSuite) TestRace() {
	t := s.T()

	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	reviewer1 := s.fakeMattermost.StoreUser(User{Email: s.userNames.reviewer1})
	reviewer2 := s.fakeMattermost.StoreUser(User{Email: s.userNames.reviewer2})
	botUser := s.fakeMattermost.GetBotUser()

	s.SetContext(20 * time.Second)
	s.startApp()

	var (
		raceErr               error
		raceErrOnce           sync.Once
		postIDs               sync.Map
		postsCount            int32
		postUpdateCounters    sync.Map
		reviewCommentCounters sync.Map
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
			req.SetSuggestedReviewers([]string{reviewer1.Email, reviewer2.Email})
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
			post, err := s.fakeMattermost.CheckNewPost(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}

			if post.RootID == "" {
				// Handle "root" notifications.

				postKey := MattermostDataPost{ChannelID: post.ChannelID, PostID: post.ID}
				if _, loaded := postIDs.LoadOrStore(postKey, struct{}{}); loaded {
					return setRaceErr(trace.Errorf("post %v already stored", postKey))
				}
				atomic.AddInt32(&postsCount, 1)

				reqID, err := parsePostField(post, "Request ID")
				if err != nil {
					return setRaceErr(trace.Wrap(err))
				}

				directChannel, ok := s.fakeMattermost.GetDirectChannel(post.ChannelID)
				if !ok {
					return setRaceErr(trace.Errorf("direct channel %q not found", post.ChannelID))
				}

				var userID string
				if directChannel.User2ID == botUser.ID {
					userID = directChannel.User1ID
				} else {
					userID = directChannel.User2ID
				}
				user, ok := s.fakeMattermost.GetUser(userID)
				if !ok {
					return setRaceErr(trace.Errorf("user %q not found", userID))
				}

				if _, err = s.teleport.SubmitAccessReview(ctx, reqID, types.AccessReview{
					Author:        user.Email,
					ProposedState: types.RequestState_APPROVED,
					Created:       time.Now(),
					Reason:        "okay",
				}); err != nil {
					return setRaceErr(trace.Wrap(err))
				}
			} else {
				// Handle review comments.

				postKey := MattermostDataPost{ChannelID: post.ChannelID, PostID: post.RootID}
				var newCounter int32
				val, _ := reviewCommentCounters.LoadOrStore(postKey, &newCounter)
				counterPtr := val.(*int32)
				atomic.AddInt32(counterPtr, 1)
			}

			return nil
		})
	}

	// Multiplier TWO means that we handle updates for each of the two messages posted to reviewers.
	for i := 0; i < 2*s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			post, err := s.fakeMattermost.CheckPostUpdate(ctx)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			postKey := MattermostDataPost{ChannelID: post.ChannelID, PostID: post.ID}
			var newCounter int32
			val, _ := postUpdateCounters.LoadOrStore(postKey, &newCounter)
			counterPtr := val.(*int32)
			atomic.AddInt32(counterPtr, 1)

			return nil
		})
	}

	process.Terminate()
	<-process.Done()
	require.NoError(t, raceErr)

	assert.Equal(t, int32(2*s.raceNumber), postsCount)
	postIDs.Range(func(key, value interface{}) bool {
		next := true

		val, loaded := reviewCommentCounters.LoadAndDelete(key)
		next = next && assert.True(t, loaded)
		counterPtr := val.(*int32)
		next = next && assert.Equal(t, int32(2), *counterPtr)

		val, loaded = postUpdateCounters.LoadAndDelete(key)
		next = next && assert.True(t, loaded)
		counterPtr = val.(*int32)
		next = next && assert.Equal(t, int32(1), *counterPtr)

		return next
	})
}

func parsePostField(post Post, field string) (string, error) {
	text := post.Message
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
