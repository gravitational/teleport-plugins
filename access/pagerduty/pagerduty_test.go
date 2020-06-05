package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"sort"
	"testing"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
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

type PagerdutySuite struct {
	ctx           context.Context
	cancel        context.CancelFunc
	app           *App
	publicURL     string
	me            *user.User
	fakePagerduty *FakePagerduty
	teleport      *integration.TeleInstance
	tmpFiles      []*os.File
}

var _ = Suite(&PagerdutySuite{})

func TestPagerduty(t *testing.T) { TestingT(t) }

func (s *PagerdutySuite) SetUpSuite(c *C) {
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

func (s *PagerdutySuite) SetUpTest(c *C) {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), time.Second)
	s.publicURL = ""
	s.fakePagerduty = NewFakePagerduty()
}

func (s *PagerdutySuite) TearDownTest(c *C) {
	s.shutdownApp(c)
	s.fakePagerduty.Close()
	s.cancel()
	for _, tmp := range s.tmpFiles {
		err := os.Remove(tmp.Name())
		c.Assert(err, IsNil)
	}
	s.tmpFiles = []*os.File{}
}

func (s *PagerdutySuite) newTmpFile(c *C, pattern string) (file *os.File) {
	file, err := ioutil.TempFile("", pattern)
	c.Assert(err, IsNil)
	s.tmpFiles = append(s.tmpFiles, file)
	return
}

func (s *PagerdutySuite) startApp(c *C) {
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
	conf.Pagerduty.APIEndpoint = s.fakePagerduty.URL()
	conf.Pagerduty.UserEmail = "bot@example.com"
	conf.Pagerduty.ServiceID = "1111"
	if s.publicURL != "" {
		conf.HTTP.PublicAddr = s.publicURL
	}
	conf.HTTP.ListenAddr = ":0"
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

func (s *PagerdutySuite) shutdownApp(c *C) {
	err := s.app.Shutdown(s.ctx)
	c.Assert(err, IsNil)
}

func (s *PagerdutySuite) createAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *PagerdutySuite) createExpiredAccessRequest(c *C) services.AccessRequest {
	req, err := s.teleport.CreateExpiredAccessRequest(s.ctx, s.me.Username, "admin")
	c.Assert(err, IsNil)
	return req
}

func (s *PagerdutySuite) checkPluginData(c *C, reqID string) PluginData {
	rawData, err := s.teleport.PollAccessRequestPluginData(s.ctx, "pagerduty", reqID, time.Millisecond*250)
	c.Assert(err, IsNil)
	return DecodePluginData(rawData)
}

func (s *PagerdutySuite) postAction(c *C, incident pd.Incident, action string) {
	payload := WebhookPayload{
		Messages: []WebhookMessage{
			WebhookMessage{
				ID:       "MSG1",
				Event:    "incident.custom",
				Incident: &incident,
			},
		},
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&payload)
	c.Assert(err, IsNil)
	req, err := http.NewRequest("POST", s.publicURL+"/"+action, &buf)
	c.Assert(err, IsNil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Webhook-Id", "Webhook-123")
	response, err := http.DefaultClient.Do(req)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)

	err = response.Body.Close()
	c.Assert(err, IsNil)
}

func (s *PagerdutySuite) TestExtensionCreation(c *C) {
	s.startApp(c)
	s.shutdownApp(c)
	extension1, err := s.fakePagerduty.CheckNewExtension(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("first extension wasn't created"))
	extension2, err := s.fakePagerduty.CheckNewExtension(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("second extension wasn't created"))

	extTitles := []string{extension1.Name, extension2.Name}
	sort.Strings(extTitles)
	c.Assert(extTitles[0], Equals, pdApproveActionLabel)
	c.Assert(extTitles[1], Equals, pdDenyActionLabel)

	extEndpoints := []string{extension1.EndpointURL, extension2.EndpointURL}
	sort.Strings(extEndpoints)
	c.Assert(extEndpoints[0], Equals, s.publicURL+"/"+pdApproveAction)
	c.Assert(extEndpoints[1], Equals, s.publicURL+"/"+pdDenyAction)
}

func (s *PagerdutySuite) TestIncidentCreation(c *C) {
	s.startApp(c)
	req := s.createAccessRequest(c)
	pluginData := s.checkPluginData(c, req.GetName())

	incident, err := s.fakePagerduty.CheckNewIncident(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new incidents stored"))

	c.Assert(pluginData.ID, Equals, incident.Id)
	c.Assert(incident.IncidentKey, Equals, pdIncidentKeyPrefix+"/"+req.GetName())
}

func (s *PagerdutySuite) TestApproval(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	incident, err := s.fakePagerduty.CheckNewIncident(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new incidents stored"))
	c.Assert(incident.Status, Equals, "triggered")
	incidentID := incident.Id

	s.postAction(c, incident, pdApproveAction)

	incident, err = s.fakePagerduty.CheckIncidentUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no incidents updated"))
	c.Assert(incident.Id, Equals, incidentID)
	c.Assert(incident.Status, Equals, "resolved")

	note, err := s.fakePagerduty.CheckNewIncidentNote(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new notes stored"))
	c.Assert(note.Content, Equals, "Access request has been approved")

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *PagerdutySuite) TestDenial(c *C) {
	s.startApp(c)
	request := s.createAccessRequest(c)
	s.checkPluginData(c, request.GetName()) // when plugin data created, we are sure that request is completely served.

	incident, err := s.fakePagerduty.CheckNewIncident(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new incidents stored"))
	c.Assert(incident.Status, Equals, "triggered")
	incidentID := incident.Id

	s.postAction(c, incident, pdDenyAction)

	incident, err = s.fakePagerduty.CheckIncidentUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no incidents updated"))
	c.Assert(incident.Id, Equals, incidentID)
	c.Assert(incident.Status, Equals, "resolved")

	note, err := s.fakePagerduty.CheckNewIncidentNote(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new notes stored"))
	c.Assert(note.Content, Equals, "Access request has been denied")

	request, err = s.teleport.GetAccessRequest(s.ctx, request.GetName())
	c.Assert(err, IsNil)
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *PagerdutySuite) TestExpiration(c *C) {
	s.startApp(c)
	s.createExpiredAccessRequest(c)

	incident, err := s.fakePagerduty.CheckNewIncident(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new incidents stored"))
	c.Assert(incident.Status, Equals, "triggered")
	incidentID := incident.Id

	incident, err = s.fakePagerduty.CheckIncidentUpdate(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no incidents updated"))
	c.Assert(incident.Id, Equals, incidentID)
	c.Assert(incident.Status, Equals, "resolved")

	note, err := s.fakePagerduty.CheckNewIncidentNote(s.ctx, 250*time.Millisecond)
	c.Assert(err, IsNil, Commentf("no new notes stored"))
	c.Assert(note.Content, Equals, "Access request has been expired")
}
