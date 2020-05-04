package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"sync"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	mm "github.com/mattermost/mattermost-server/model"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport/integration"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/utils"

	. "gopkg.in/check.v1"
)

const (
	Host   = "localhost"
	HostID = "00000000-0000-0000-0000-000000000000"
	Site   = "local-site"
)

type MattermostSuite struct {
	app               *App
	appPort           string
	webhookUrl        string
	me                *user.User
	fakeMattermostSrv *httptest.Server
	posts             sync.Map
	newPosts          chan *mm.Post
	teleport          *integration.TeleInstance
	tmpFiles          []*os.File
}

var _ = Suite(&MattermostSuite{})

func TestMattermost(t *testing.T) { TestingT(t) }

func (s *MattermostSuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	portList, err := utils.GetFreeTCPPorts(6, 20200)
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
	s.webhookUrl = "http://" + Host + ":" + s.appPort + "/"
}

func (s *MattermostSuite) SetUpTest(c *C) {
	s.startFakeMattermost(c)
	s.startApp(c)
	time.Sleep(time.Millisecond * 500) // Wait some time for services to start up
}

func (s *MattermostSuite) TearDownTest(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*250)
	defer cancel()
	err := s.app.Shutdown(ctx)
	c.Assert(err, IsNil)
	s.fakeMattermostSrv.Close()
	close(s.newPosts)
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

func (s *MattermostSuite) startFakeMattermost(c *C) {
	s.newPosts = make(chan *mm.Post, 1)

	fakeMattermost := httprouter.New()
	fakeMattermost.GET("/api/v4/teams/name/test-team", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		team := mm.Team{
			Id:   "1111",
			Name: "test-team",
		}
		err := json.NewEncoder(rw).Encode(&team)
		c.Assert(err, IsNil)
	})
	fakeMattermost.GET("/api/v4/teams/1111/channels/name/test-channel", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		channel := mm.Channel{
			Id:     "2222",
			TeamId: "1111",
			Name:   "test-channel",
		}
		err := json.NewEncoder(rw).Encode(&channel)
		c.Assert(err, IsNil)
	})
	fakeMattermost.POST("/api/v4/posts", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		post := &mm.Post{}
		err := json.NewDecoder(r.Body).Decode(post)
		c.Assert(err, IsNil)

		if post.ChannelId != "2222" {
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}

		post.Id = fmt.Sprintf("%v", time.Now().UnixNano())
		s.posts.Store(post.Id, *post)
		s.newPosts <- post

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(post)
		c.Assert(err, IsNil)

	})
	fakeMattermost.PUT("/api/v4/posts/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		post := s.getPost(id)
		if post == nil {
			fmt.Printf("Not found %s", id)
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}

		var newPost mm.Post
		err := json.NewDecoder(r.Body).Decode(&newPost)
		c.Assert(err, IsNil)

		post.Message = newPost.Message
		post.Props = newPost.Props
		s.posts.Store(post.Id, *post)

		rw.WriteHeader(http.StatusOK)
		err = json.NewEncoder(rw).Encode(post)
		c.Assert(err, IsNil)
	})

	s.fakeMattermostSrv = httptest.NewServer(fakeMattermost)
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

	var conf Config
	conf.Teleport.AuthServer = s.teleport.Config.Auth.SSHAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.Mattermost.URL = s.fakeMattermostSrv.URL
	conf.Mattermost.Team = "test-team"
	conf.Mattermost.Channel = "test-channel"
	conf.Mattermost.Secret = "1234567812345678123456781234567812345678123456781234567812345678"
	conf.HTTP.Listen = ":" + s.appPort
	conf.HTTP.RawBaseURL = "http://" + Host + ":" + s.appPort + "/"
	conf.HTTP.Insecure = true

	s.app, err = NewApp(conf)
	c.Assert(err, IsNil)

	go func() {
		err = s.app.Run(context.TODO())
		c.Assert(err, IsNil)
	}()
}

func (s *MattermostSuite) createAccessRequest(c *C) services.AccessRequest {
	client, err := s.teleport.NewClient(integration.ClientConfig{Login: s.me.Username})
	c.Assert(err, IsNil)
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	err = client.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)
	time.Sleep(time.Millisecond * 500) // Wait some time for watcher to receive a request
	return req
}

func (s *MattermostSuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	client, err := s.teleport.NewClient(integration.ClientConfig{Login: s.me.Username})
	c.Assert(err, IsNil)
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	ttl := time.Millisecond * 500
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

func (s *MattermostSuite) getPost(id string) *mm.Post {
	if obj, ok := s.posts.Load(id); ok {
		post := obj.(mm.Post)
		return &post
	} else {
		return nil
	}
}

func (s *MattermostSuite) postWebhook(c *C, post *mm.Post, actionName string) {
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
}

func (s *MattermostSuite) TestMattermostMessagePosting(c *C) {
	_ = s.createAccessRequest(c)

	var post *mm.Post
	select {
	case post = <-s.newPosts:
		c.Assert(post, NotNil)
	case <-time.After(time.Millisecond * 500):
		c.Fatal("post wasn't created")
	}

	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	attachment := attachments[0]
	c.Assert(attachment.Actions, HasLen, 2)
	c.Assert(attachment.Actions[0].Name, Equals, "Approve")
	c.Assert(attachment.Actions[1].Name, Equals, "Deny")
}

func (s *MattermostSuite) TestApproval(c *C) {
	request := s.createAccessRequest(c)

	var post *mm.Post
	select {
	case post = <-s.newPosts:
		c.Assert(post, NotNil)
	case <-time.After(time.Millisecond * 500):
		c.Fatal("post wasn't created")
	}

	s.postWebhook(c, post, "Approve")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *MattermostSuite) TestDenial(c *C) {
	request := s.createAccessRequest(c)

	var post *mm.Post
	select {
	case post = <-s.newPosts:
		c.Assert(post, NotNil)
	case <-time.After(time.Millisecond * 500):
		c.Fatal("post wasn't created")
	}

	s.postWebhook(c, post, "Deny")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *MattermostSuite) TestApproveExpired(c *C) {
	s.createExpiredAccessRequest(c)

	var post *mm.Post
	select {
	case post = <-s.newPosts:
		c.Assert(post, NotNil)
	case <-time.After(time.Millisecond * 500):
		c.Fatal("post wasn't created")
	}

	s.postWebhook(c, post, "Approve")

	post = s.getPost(post.Id)
	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)
}

func (s *MattermostSuite) TestDenyExpired(c *C) {
	s.createExpiredAccessRequest(c)

	var post *mm.Post
	select {
	case post = <-s.newPosts:
		c.Assert(post, NotNil)
	case <-time.After(time.Millisecond * 500):
		c.Fatal("post wasn't created")
	}

	s.postWebhook(c, post, "Deny")

	post = s.getPost(post.Id)
	attachments := post.Attachments()
	c.Assert(attachments, HasLen, 1)
	c.Assert(attachments[0].Actions, HasLen, 0)
}
