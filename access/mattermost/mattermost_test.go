package main

import (
	"context"
	"io/ioutil"
	"os"
	"os/user"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/access/integration"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"

	. "gopkg.in/check.v1"
)

const (
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"
)

var msgFieldRegexp = regexp.MustCompile(`(?im)^\*\*([a-zA-Z ]+)\*\*:\ +(.+)$`)

type MattermostSuite struct {
	ctx            context.Context
	cancel         context.CancelFunc
	appConfig      Config
	app            *App
	raceNumber     int
	me             *user.User
	fakeMattermost *FakeMattermost
	mmUser         User
	teleport       *integration.TeleInstance
	tmpFiles       []*os.File
}

var _ = Suite(&MattermostSuite{})

func TestMattermost(t *testing.T) { TestingT(t) }

func (s *MattermostSuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	t := integration.NewInstance(integration.InstanceConfig{ClusterName: Site, HostID: HostID, NodeName: Host, Priv: priv, Pub: pub})

	s.raceNumber = runtime.GOMAXPROCS(0)
	s.me, err = user.Current()
	c.Assert(err, IsNil)
	userRole, err := types.NewRole("foo", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Logins:  []string{s.me.Username}, // cannot be empty
			Request: &types.AccessRequestConditions{Roles: []string{"admin"}},
		},
	})
	c.Assert(err, IsNil)
	t.AddUserWithRole(s.me.Username, userRole)

	accessPluginRole, err := types.NewRole("access-plugin", types.RoleSpecV3{
		Allow: types.RoleConditions{
			Logins: []string{"access-plugin"}, // cannot be empty
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read"}),
				types.NewRule("access_plugin_data", []string{"update"}),
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

func (s *MattermostSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Second)
	s.fakeMattermost = NewFakeMattermost(User{Username: "bot", Email: "bot@example.com"}, s.raceNumber)
	s.mmUser = s.fakeMattermost.StoreUser(User{
		FirstName: "User",
		LastName:  "Test",
		Username:  s.me.Username,
		Email:     s.me.Username + "@example.com",
	})

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
	conf.Mattermost.URL = s.fakeMattermost.URL()

	s.appConfig = conf
}

func (s *MattermostSuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakeMattermost.Close()
	s.cancel()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *MattermostSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *MattermostSuite) startApp(c *C) {
	var err error

	s.app, err = NewApp(s.appConfig)
	c.Assert(err, IsNil)

	go func() {
		if err := s.app.Run(s.ctx); err != nil {
			panic(err)
		}
	}()
	ok, err := s.app.WaitReady(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)
}

func (s *MattermostSuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(s.app.Err(), IsNil)
}

func (s *MattermostSuite) newAccessRequest(c *C, reviewers []User) types.AccessRequest {
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	req.SetRequestReason("because of")
	var suggestedReviewers []string
	for _, user := range reviewers {
		suggestedReviewers = append(suggestedReviewers, user.Email)
	}
	req.SetSuggestedReviewers(suggestedReviewers)
	return req
}

func (s *MattermostSuite) createAccessRequest(c *C, reviewers []User) types.AccessRequest {
	req := s.newAccessRequest(c, reviewers)
	err := s.teleport.CreateAccessRequest(s.ctx, req)
	c.Assert(err, IsNil)
	return req
}

func (s *MattermostSuite) createExpiredAccessRequest(c *C, reviewers []User) types.AccessRequest {
	req := s.newAccessRequest(c, reviewers)
	err := s.teleport.CreateExpiredAccessRequest(s.ctx, req)
	c.Assert(err, IsNil)
	return req
}

func (s *MattermostSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "mattermost", reqID)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *MattermostSuite) TestMattermostMessagePosting(c *C) {
	reviewer1 := s.fakeMattermost.StoreUser(User{Email: "user1@example.com"})
	reviewer2 := s.fakeMattermost.StoreUser(User{Email: "user2@example.com"})
	directChannel1 := s.fakeMattermost.GetDirectChannel(s.fakeMattermost.GetBotUser(), reviewer1)
	directChannel2 := s.fakeMattermost.GetDirectChannel(s.fakeMattermost.GetBotUser(), reviewer2)

	s.startApp(c)
	request := s.createAccessRequest(c, []User{reviewer2, reviewer1})

	pluginData := s.checkPluginData(c, request.GetName())
	c.Assert(pluginData.MattermostData, HasLen, 2)

	var posts []Post
	postSet := make(MattermostDataPostSet)
	for i := 0; i < 2; i++ {
		post, err := s.fakeMattermost.CheckNewPost(s.ctx)
		c.Assert(err, IsNil, Commentf("no new messages posted"))
		postSet.Add(MattermostDataPost{ChannelID: post.ChannelID, PostID: post.ID})
		posts = append(posts, post)
	}

	c.Assert(postSet, HasLen, 2)
	c.Assert(postSet.Contains(pluginData.MattermostData[0]), Equals, true)
	c.Assert(postSet.Contains(pluginData.MattermostData[1]), Equals, true)

	sort.Sort(MattermostPostSlice(posts))

	c.Assert(posts[0].ChannelID, Equals, directChannel1.ID)
	c.Assert(posts[1].ChannelID, Equals, directChannel2.ID)

	post := posts[0]
	reqID, err := parsePostField(post, "Request ID")
	c.Assert(err, IsNil)
	c.Assert(reqID, Equals, request.GetName())

	username, err := parsePostField(post, "User")
	c.Assert(err, IsNil)
	c.Assert(username, Equals, s.me.Username)

	reason, err := parsePostField(post, "Reason")
	c.Assert(err, IsNil)
	c.Assert(reason, Equals, "because of")

	statusLine, err := parsePostField(post, "Status")
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "⏳ PENDING")
}

func (s *MattermostSuite) TestApproval(c *C) {
	reviewer := s.fakeMattermost.StoreUser(User{Email: "user@example.com"})

	s.startApp(c)

	req := s.createAccessRequest(c, []User{reviewer})
	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))
	c.Assert(post.ChannelID, Equals, s.fakeMattermost.GetDirectChannel(s.fakeMattermost.GetBotUser(), reviewer).ID)

	err = s.teleport.SetAccessRequestState(s.ctx, types.AccessRequestUpdate{
		RequestID: req.GetName(),
		State:     types.RequestState_APPROVED,
	})
	c.Assert(err, IsNil)

	postUpdate, err := s.fakeMattermost.CheckPostUpdate(s.ctx)
	c.Assert(err, IsNil, Commentf("no messages updated"))
	c.Assert(postUpdate.ID, Equals, post.ID)
	c.Assert(postUpdate.ChannelID, Equals, post.ChannelID)

	statusLine, err := parsePostField(postUpdate, "Status")
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "✅ APPROVED")
}

func (s *MattermostSuite) TestDenial(c *C) {
	reviewer := s.fakeMattermost.StoreUser(User{Email: "user@example.com"})

	s.startApp(c)

	req := s.createAccessRequest(c, []User{reviewer})
	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))
	c.Assert(post.ChannelID, Equals, s.fakeMattermost.GetDirectChannel(s.fakeMattermost.GetBotUser(), reviewer).ID)

	err = s.teleport.SetAccessRequestState(s.ctx, types.AccessRequestUpdate{
		RequestID: req.GetName(),
		State:     types.RequestState_DENIED,
	})
	c.Assert(err, IsNil)

	postUpdate, err := s.fakeMattermost.CheckPostUpdate(s.ctx)
	c.Assert(err, IsNil, Commentf("no messages updated"))
	c.Assert(postUpdate.ID, Equals, post.ID)
	c.Assert(postUpdate.ChannelID, Equals, post.ChannelID)

	statusLine, err := parsePostField(postUpdate, "Status")
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "❌ DENIED")
}

func (s *MattermostSuite) TestExpiration(c *C) {
	reviewer := s.fakeMattermost.StoreUser(User{Email: "user@example.com"})

	s.startApp(c)
	s.createExpiredAccessRequest(c, []User{reviewer})

	post, err := s.fakeMattermost.CheckNewPost(s.ctx)
	c.Assert(err, IsNil, Commentf("no new messages posted"))
	c.Assert(post.ChannelID, Equals, s.fakeMattermost.GetDirectChannel(s.fakeMattermost.GetBotUser(), reviewer).ID)

	postUpdate, err := s.fakeMattermost.CheckPostUpdate(s.ctx)
	c.Assert(err, IsNil, Commentf("no messages updated"))
	c.Assert(postUpdate.ID, Equals, post.ID)
	c.Assert(postUpdate.ChannelID, Equals, post.ChannelID)

	statusLine, err := parsePostField(postUpdate, "Status")
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "⌛ EXPIRED")
}

func (s *MattermostSuite) TestRace(c *C) {
	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	reviewer := s.fakeMattermost.StoreUser(User{Email: "user@example.com"})

	s.cancel() // Cancel the default timeout
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 20*time.Second)
	s.startApp(c)

	var (
		raceErr     error
		raceErrOnce sync.Once
	)
	setRaceErr := func(err error) error {
		raceErrOnce.Do(func() {
			raceErr = err
		})
		return err
	}

	process := lib.NewProcess(s.ctx)
	for i := 0; i < s.raceNumber; i++ {
		process.SpawnCritical(func(ctx context.Context) error {
			req, err := services.NewAccessRequest(s.me.Username, "admin")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			req.SetSuggestedReviewers([]string{reviewer.Email})
			if err := s.teleport.CreateAccessRequest(ctx, req); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			post, err := s.fakeMattermost.CheckNewPost(ctx)
			if err := trace.Wrap(err); err != nil {
				return setRaceErr(err)
			}

			reqID, err := parsePostField(post, "Request ID")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			if _, err := s.teleport.PollAccessRequestPluginData(s.ctx, "mattermost", reqID); err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			if err = s.teleport.SetAccessRequestState(ctx, types.AccessRequestUpdate{
				RequestID: reqID,
				State:     types.RequestState_APPROVED,
			}); err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			if _, err := s.fakeMattermost.CheckPostUpdate(ctx); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
	}
	process.Terminate()
	<-process.Done()
	c.Assert(raceErr, IsNil)
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
