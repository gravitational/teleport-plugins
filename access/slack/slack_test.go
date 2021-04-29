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

var msgFieldRegexp = regexp.MustCompile(`(?im)^\*([a-zA-Z ]+)\*: (.+)$`)

type SlackSuite struct {
	ctx        context.Context
	cancel     context.CancelFunc
	appConfig  Config
	app        *App
	raceNumber int
	me         *user.User
	fakeSlack  *FakeSlack
	slackUser  User
	teleport   *integration.TeleInstance
	tmpFiles   []*os.File
}

var _ = Suite(&SlackSuite{})

func TestSlackbot(t *testing.T) { TestingT(t) }

func (s *SlackSuite) SetUpSuite(c *C) {
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
			Request: &services.AccessRequestConditions{Roles: []string{"admin"}},
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

func (s *SlackSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Second)
	s.fakeSlack = NewFakeSlack(User{Name: "slackbot"}, s.raceNumber)
	s.slackUser = s.fakeSlack.StoreUser(User{
		Name: s.me.Username,
		Profile: UserProfile{
			Email: s.me.Username + "@example.com",
		},
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
	conf.Slack.Token = "000000"
	conf.Slack.APIURL = s.fakeSlack.URL() + "/"

	s.appConfig = conf
}

func (s *SlackSuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakeSlack.Close()
	s.cancel()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *SlackSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *SlackSuite) startApp(c *C) {
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

func (s *SlackSuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(s.app.Err(), IsNil)
}

func (s *SlackSuite) newAccessRequest(c *C, reviewers []User) services.AccessRequest {
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	req.SetRequestReason("because of")
	var suggestedReviewers []string
	for _, user := range reviewers {
		suggestedReviewers = append(suggestedReviewers, user.Profile.Email)
	}
	req.SetSuggestedReviewers(suggestedReviewers)
	return req
}

func (s *SlackSuite) createAccessRequest(c *C, reviewers []User) services.AccessRequest {
	req := s.newAccessRequest(c, reviewers)
	err := s.teleport.CreateAccessRequest(s.ctx, req)
	c.Assert(err, IsNil)
	return req
}

func (s *SlackSuite) createExpiredAccessRequest(c *C, reviewers []User) services.AccessRequest {
	req := s.newAccessRequest(c, reviewers)
	err := s.teleport.CreateExpiredAccessRequest(s.ctx, req)
	c.Assert(err, IsNil)
	return req
}

func (s *SlackSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "slack", reqID)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

// Tests if Interactive Mode posts Slack message with buttons correctly
func (s *SlackSuite) TestMessagePosting(c *C) {
	reviewer1 := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user1@example.com",
		},
	})
	reviewer2 := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user2@example.com",
		},
	})

	s.startApp(c)
	request := s.createAccessRequest(c, []User{reviewer2, reviewer1})

	pluginData := s.checkPluginData(c, request.GetName())
	c.Assert(pluginData.SlackData, HasLen, 2)

	var messages []Msg
	messageSet := make(SlackDataMessageSet)
	for i := 0; i < 2; i++ {
		msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
		c.Assert(err, IsNil)
		messageSet.Add(SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp})
		messages = append(messages, msg)
	}

	c.Assert(messageSet, HasLen, 2)
	c.Assert(messageSet.Contains(pluginData.SlackData[0]), Equals, true)
	c.Assert(messageSet.Contains(pluginData.SlackData[1]), Equals, true)

	sort.Sort(SlackMessageSlice(messages))

	c.Assert(messages[0].Channel, Equals, reviewer1.ID)
	c.Assert(messages[1].Channel, Equals, reviewer2.ID)

	msgReason, err := parseMessageField(messages[0], "Reason")
	c.Assert(err, IsNil)
	c.Assert(msgReason, Equals, "because of")

	statusLine, err := getStatusLine(messages[0])
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "*Status:* ⏳ PENDING")
}

func (s *SlackSuite) TestRecipientsConfig(c *C) {
	reviewer1 := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user1@example.com",
		},
	})
	reviewer2 := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user2@example.com",
		},
	})

	s.appConfig.Slack.Recipients = []string{reviewer2.Profile.Email, reviewer1.ID}
	s.startApp(c)
	request := s.createAccessRequest(c, nil)

	pluginData := s.checkPluginData(c, request.GetName())
	c.Assert(pluginData.SlackData, HasLen, 2)

	var (
		msg      Msg
		messages []Msg
	)

	messageSet := make(SlackDataMessageSet)

	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	messageSet.Add(SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp})
	messages = append(messages, msg)

	msg, err = s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	messageSet.Add(SlackDataMessage{ChannelID: msg.Channel, Timestamp: msg.Timestamp})
	messages = append(messages, msg)

	c.Assert(messageSet, HasLen, 2)
	c.Assert(messageSet.Contains(pluginData.SlackData[0]), Equals, true)
	c.Assert(messageSet.Contains(pluginData.SlackData[1]), Equals, true)

	sort.Sort(SlackMessageSlice(messages))

	c.Assert(messages[0].Channel, Equals, reviewer1.ID)
	c.Assert(messages[1].Channel, Equals, reviewer2.ID)
}

func (s *SlackSuite) TestApproval(c *C) {
	reviewer := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user@example.com",
		},
	})
	s.startApp(c)
	req := s.createAccessRequest(c, []User{reviewer})
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg.Channel, Equals, reviewer.ID)

	err = s.teleport.SetAccessRequestState(s.ctx, services.AccessRequestUpdate{
		RequestID: req.GetName(),
		State:     types.RequestState_APPROVED,
	})
	c.Assert(err, IsNil)

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByAPI(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msgUpdate.Channel, Equals, reviewer.ID)
	c.Assert(msgUpdate.Timestamp, Equals, msg.Timestamp)

	statusLine, err := getStatusLine(msgUpdate)
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "*Status:* ✅ APPROVED")
}

func (s *SlackSuite) TestDenial(c *C) {
	reviewer := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user@example.com",
		},
	})
	s.startApp(c)
	req := s.createAccessRequest(c, []User{reviewer})
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg.Channel, Equals, reviewer.ID)

	err = s.teleport.SetAccessRequestState(s.ctx, services.AccessRequestUpdate{
		RequestID: req.GetName(),
		State:     types.RequestState_DENIED,
	})
	c.Assert(err, IsNil)

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByAPI(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msgUpdate.Channel, Equals, reviewer.ID)
	c.Assert(msgUpdate.Timestamp, Equals, msg.Timestamp)

	statusLine, err := getStatusLine(msgUpdate)
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "*Status:* ❌ DENIED")
}

func (s *SlackSuite) TestExpiration(c *C) {
	reviewer := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user@example.com",
		},
	})
	s.startApp(c)
	s.createExpiredAccessRequest(c, []User{reviewer})
	msg, err := s.fakeSlack.CheckNewMessage(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msg.Channel, Equals, reviewer.ID)

	msgUpdate, err := s.fakeSlack.CheckMessageUpdateByAPI(s.ctx)
	c.Assert(err, IsNil)
	c.Assert(msgUpdate.Channel, Equals, reviewer.ID)
	c.Assert(msgUpdate.Timestamp, Equals, msg.Timestamp)

	statusLine, err := getStatusLine(msgUpdate)
	c.Assert(err, IsNil)
	c.Assert(statusLine, Equals, "*Status:* ⌛ EXPIRED")
}

func (s *SlackSuite) TestRace(c *C) {
	prevLogLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel) // Turn off noisy debug logging
	defer log.SetLevel(prevLogLevel)

	reviewer := s.fakeSlack.StoreUser(User{
		Profile: UserProfile{
			Email: "user@example.com",
		},
	})

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
			req.SetSuggestedReviewers([]string{reviewer.Profile.Email})
			if err := s.teleport.CreateAccessRequest(ctx, req); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			msg, err := s.fakeSlack.CheckNewMessage(ctx)
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			reqID, err := parseMessageField(msg, "ID")
			if err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			if _, err := s.teleport.PollAccessRequestPluginData(s.ctx, "slack", reqID); err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			if err = s.teleport.SetAccessRequestState(ctx, services.AccessRequestUpdate{
				RequestID: reqID,
				State:     types.RequestState_APPROVED,
			}); err != nil {
				return setRaceErr(trace.Wrap(err))
			}

			return nil
		})
		process.SpawnCritical(func(ctx context.Context) error {
			if _, err := s.fakeSlack.CheckMessageUpdateByAPI(ctx); err != nil {
				return setRaceErr(trace.Wrap(err))
			}
			return nil
		})
	}
	process.Terminate()
	<-process.Done()
	c.Assert(raceErr, IsNil)
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
