package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"testing"
	"time"

	mm "github.com/mattermost/mattermost-server/model"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/access/integration"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/services"

	. "gopkg.in/check.v1"
)

const (
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"
)

type MattermostSuite struct {
	ctx            context.Context
	cancel         context.CancelFunc
	app            *App
	publicURL      string
	me             *user.User
	fakeMattermost *FakeMattermost
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

	err = t.Create(nil, nil)
	c.Assert(err, IsNil)
	if err := t.Start(); err != nil {
		c.Fatalf("Unexpected response from Start: %v", err)
	}
	s.teleport = t
}

func (s *MattermostSuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), time.Second)
	s.publicURL = ""
	s.fakeMattermost = NewFakeMattermost()
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
	conf.Mattermost.Team = "test-team"
	conf.Mattermost.Channel = "test-channel"
	conf.Mattermost.Secret = "1234567812345678123456781234567812345678123456781234567812345678"
	conf.HTTP.ListenAddr = ":0"
	if s.publicURL != "" {
		conf.HTTP.PublicAddr = s.publicURL
	}
	conf.HTTP.Insecure = true

	s.app, err = NewApp(conf)
	c.Assert(err, IsNil)

	go func() {
		err = s.app.Run(s.ctx)
		c.Assert(err, IsNil)
	}()
	ctx, cancel := context.WithTimeout(s.ctx, time.Millisecond*250)
	defer cancel()
	ok, err := s.app.WaitReady(ctx)
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)
	if s.publicURL == "" {
		s.publicURL = s.app.PublicURL().String()
	}
}

func (s *MattermostSuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
}

func (s *MattermostSuite) createAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *MattermostSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateExpiredAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *MattermostSuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "mattermost", reqID)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *MattermostSuite) postWebhook(c *C, post mm.Post, actionName string) {
	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	var action *mm.PostAction
	for _, a := range attachments[0].Actions {
		if a.Name == actionName {
			action = a
			break
		}
	}
	c.Assert(action, NotNil)

	payload := mm.PostActionIntegrationRequest{
		PostId:    post.Id,
		TeamId:    "1111",
		ChannelId: "2222",
		UserId:    "3333",
		Context:   action.Integration.Context,
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&payload)
	c.Assert(err, IsNil)
	response, err := http.Post(action.Integration.URL, "application/json", &buf)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	err = response.Body.Close()
	c.Assert(err, IsNil)
}

func (s *MattermostSuite) TestMattermostMessagePosting(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	post, err := s.fakeMattermost.CheckNewPost(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new messages posted"))

	pluginData := s.checkPluginData(c, request.GetName())
	c.Assert(pluginData.PostID, Equals, post.Id)

	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	attachment := attachments[0]
	c.Assert(attachment.Actions, HasLen, 2)
	c.Assert(attachment.Actions[0].Name, Equals, "Approve")
	c.Assert(attachment.Actions[1].Name, Equals, "Deny")
}

func (s *MattermostSuite) TestApproval(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	post, err := s.fakeMattermost.CheckNewPost(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new messages posted"))
	s.postWebhook(c, post, "Approve")

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *MattermostSuite) TestDenial(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	post, err := s.fakeMattermost.CheckNewPost(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new messages posted"))
	s.postWebhook(c, post, "Deny")

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *MattermostSuite) TestExpiration(c *C) {
	s.startApp(c)
	s.createExpiredAccessRequest(c)

	post, err := s.fakeMattermost.CheckNewPost(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new messages posted"))
	s.postWebhook(c, post, "Approve")
	postID := post.Id

	post, err = s.fakeMattermost.CheckPostUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no messages updated"))
	c.Assert(post.Id, Equals, postID)
	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)
}
