package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport/integration"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/utils"
	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slacktest"

	. "gopkg.in/check.v1"
)

const (
	Host        = "localhost"
	HostID      = "00000000-0000-0000-0000-000000000000"
	Site        = "local-site"
	SlackSecret = "f9e77a2814566fe23d33dee5b853955b"
)

type SlackbotSuite struct {
	app         *App
	appPort     string
	callbackUrl string
	me          *user.User
	slackServer *slacktest.Server
	teleport    *integration.TeleInstance
	tmpFiles    []*os.File
}

var _ = Suite(&SlackbotSuite{})

func TestSlackbot(t *testing.T) { TestingT(t) }

func (s *SlackbotSuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	portList, err := utils.GetFreeTCPPorts(6, 20400)
	c.Assert(err, IsNil)
	ports := portList.PopIntSlice(5)
	t := integration.NewInstance(integration.InstanceConfig{ClusterName: Site, HostID: HostID, NodeName: Host, Ports: ports, Priv: priv, Pub: pub})

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

	err = t.Create(nil, true, nil)
	c.Assert(err, IsNil)
	if err := t.Start(); err != nil {
		c.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = t
	s.appPort = portList.Pop()
	s.callbackUrl = "http://" + Host + ":" + s.appPort + "/"
}

func (s *SlackbotSuite) SetUpTest(c *C) {
	s.startSlack(c)
	s.startApp(c)
	time.Sleep(time.Millisecond * 250) // Wait some time for services to start up
}

func (s *SlackbotSuite) TearDownTest(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*250)
	defer cancel()
	err := s.app.Shutdown(ctx)
	c.Assert(err, IsNil)
	s.slackServer.Stop()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *SlackbotSuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *SlackbotSuite) startSlack(c *C) {
	s.slackServer = slacktest.NewTestServer()
	s.slackServer.SetBotName("access_bot")
	go s.slackServer.Start()
}

func (s *SlackbotSuite) startApp(c *C) {
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

	var conf Config
	conf.Teleport.AuthServer = s.teleport.Config.Auth.SSHAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.Slack.Secret = SlackSecret
	conf.Slack.Token = "000000"
	conf.Slack.Channel = "test"
	conf.Slack.APIURL = "http://" + s.slackServer.ServerAddr + "/"
	conf.HTTP.Listen = ":" + s.appPort
	conf.HTTP.Insecure = true

	s.app, err = NewApp(conf)
	c.Assert(err, IsNil)

	go func() {
		err = s.app.Run(context.TODO())
		c.Assert(err, IsNil)
	}()
}

func (s *SlackbotSuite) createAccessRequest(c *C) services.AccessRequest {
	client, err := s.teleport.NewClient(integration.ClientConfig{Login: s.me.Username})
	c.Assert(err, IsNil)
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	err = client.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)
	time.Sleep(time.Millisecond * 250) // Wait some time for watcher to receive a request
	return req
}

func (s *SlackbotSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	client, err := s.teleport.NewClient(integration.ClientConfig{Login: s.me.Username})
	c.Assert(err, IsNil)
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	ttl := time.Millisecond * 250
	req.SetAccessExpiry(time.Now().Add(ttl))
	err = client.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)
	time.Sleep(ttl + time.Millisecond*50)
	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: req.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 0)
	return req
}

func (s *SlackbotSuite) postCallback(c *C, actionId, reqID string) {
	cb := &slack.InteractionCallback{
		ActionCallback: slack.ActionCallbacks{
			BlockActions: []*slack.BlockAction{
				&slack.BlockAction{
					ActionID: actionId,
					Value:    reqID,
				},
			},
		},
	}

	payload, err := json.Marshal(cb)
	c.Assert(err, IsNil)
	data := url.Values{
		"payload": {string(payload)},
	}
	body := data.Encode()

	stimestamp := fmt.Sprintf("%d", time.Now().Unix())
	hash := hmac.New(sha256.New, []byte(SlackSecret))
	_, err = hash.Write([]byte(fmt.Sprintf("v0:%s:%s", stimestamp, body)))
	c.Assert(err, IsNil)

	signature := hash.Sum(nil)

	req, err := http.NewRequest("POST", s.callbackUrl, strings.NewReader(body))
	c.Assert(err, IsNil)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("X-Slack-Request-Timestamp", stimestamp)
	req.Header.Add("X-Slack-Signature", "v0="+hex.EncodeToString(signature))
	response, err := http.DefaultClient.Do(req)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
}

// fetchSlackMessage and all the tests using it heavily rely on changes in slacktest package, see 13c57c4 commit.
func (s *SlackbotSuite) fetchSlackMessage(c *C) slack.Msg {
	var msg slack.Msg
	select {
	case data := <-s.slackServer.SeenFeed:
		err := json.Unmarshal([]byte(data), &msg)
		c.Assert(err, IsNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("no messages were sent to a channel")
	}
	return msg
}

func (s *SlackbotSuite) TestSlackMessagePosting(c *C) {
	request := s.createAccessRequest(c)
	msg := s.fetchSlackMessage(c)
	var blockAction *slack.ActionBlock
	for _, blk := range msg.Blocks.BlockSet {
		if a, ok := blk.(*slack.ActionBlock); ok && a.BlockID == "approve_or_deny" {
			blockAction = a
		}
	}
	c.Assert(blockAction, NotNil)
	c.Assert(blockAction.Elements.ElementSet, HasLen, 2)

	c.Assert(blockAction.Elements.ElementSet[0], FitsTypeOf, &slack.ButtonBlockElement{})
	approveButton := blockAction.Elements.ElementSet[0].(*slack.ButtonBlockElement)
	c.Assert(approveButton.ActionID, Equals, "approve_request")
	c.Assert(approveButton.Value, Equals, request.GetName())

	c.Assert(blockAction.Elements.ElementSet[1], FitsTypeOf, &slack.ButtonBlockElement{})
	denyButton := blockAction.Elements.ElementSet[1].(*slack.ButtonBlockElement)
	c.Assert(denyButton.ActionID, Equals, "deny_request")
	c.Assert(denyButton.Value, Equals, request.GetName())
}

func (s *SlackbotSuite) TestApproval(c *C) {
	request := s.createAccessRequest(c)

	s.postCallback(c, "approve_request", request.GetName())

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *SlackbotSuite) TestDenial(c *C) {
	request := s.createAccessRequest(c)

	s.postCallback(c, "deny_request", request.GetName())

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *SlackbotSuite) TestApproveExpired(c *C) {
	request := s.createExpiredAccessRequest(c)
	msg1 := s.fetchSlackMessage(c)

	s.postCallback(c, "approve_request", request.GetName())

	// Get updated message
	msg2 := s.fetchSlackMessage(c)
	c.Assert(msg1.Timestamp, Equals, msg2.Timestamp)
}

func (s *SlackbotSuite) TestDenyExpired(c *C) {
	request := s.createExpiredAccessRequest(c)
	msg1 := s.fetchSlackMessage(c)

	s.postCallback(c, "deny_request", request.GetName())

	// Get updated message
	msg2 := s.fetchSlackMessage(c)
	c.Assert(msg1.Timestamp, Equals, msg2.Timestamp)
}
