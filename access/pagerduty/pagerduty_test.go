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
	"sort"
	"sync"
	"testing"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/teleport/integration"
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
	app              *App
	appPort          string
	webhookUrl       string
	me               *user.User
	fakePagerdutySrv *httptest.Server
	extensions       sync.Map
	newExtensions    chan *pd.Extension
	incidents        sync.Map
	newIncidents     chan *pd.Incident
	newIncidentNotes chan *pd.IncidentNote
	teleport         *integration.TeleInstance
	tmpFiles         []*os.File
}

var _ = Suite(&PagerdutySuite{})

func TestPagerduty(t *testing.T) { TestingT(t) }

func (s *PagerdutySuite) SetUpSuite(c *C) {
	var err error
	log.SetLevel(log.DebugLevel)
	priv, pub, err := testauthority.New().GenerateKeyPair("")
	c.Assert(err, IsNil)
	portList, err := utils.GetFreeTCPPortsForTests(6)
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

func (s *PagerdutySuite) SetUpTest(c *C) {
	s.startFakePagerduty(c)
	s.startApp(c)
	time.Sleep(time.Millisecond * 250) // Wait some time for services to start up
}

func (s *PagerdutySuite) TearDownTest(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := s.app.Shutdown(ctx)
	c.Assert(err, IsNil)
	s.fakePagerdutySrv.Close()
	close(s.newExtensions)
	close(s.newIncidents)
	close(s.newIncidentNotes)
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

func (s *PagerdutySuite) startFakePagerduty(c *C) {
	s.newExtensions = make(chan *pd.Extension, 2)
	s.newIncidents = make(chan *pd.Incident, 1)
	s.newIncidentNotes = make(chan *pd.IncidentNote, 3)

	fakePagerduty := httprouter.New()
	fakePagerduty.GET("/services/1111", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		service := pd.Service{
			APIObject: pd.APIObject{ID: "1111"},
			Name:      "Test Service",
		}
		resp := map[string]pd.Service{"service": service}
		err := json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})
	fakePagerduty.GET("/extension_schemas", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		resp := pd.ListExtensionSchemaResponse{
			APIListObject: pd.APIListObject{
				More:  false,
				Total: 1,
			},
			ExtensionSchemas: []pd.ExtensionSchema{
				pd.ExtensionSchema{
					APIObject: pd.APIObject{ID: "11"},
					Key:       "custom_webhook",
				},
			},
		}
		err := json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})
	fakePagerduty.GET("/extensions", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		extensions := []pd.Extension{}
		s.extensions.Range(func(key, value interface{}) bool {
			extension := value.(pd.Extension)
			extension.ID = key.(string)
			extensions = append(extensions, extension)
			return true
		})
		resp := pd.ListExtensionResponse{
			APIListObject: pd.APIListObject{
				More:  false,
				Total: uint(len(extensions)),
			},
			Extensions: extensions,
		}
		err := json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})
	fakePagerduty.POST("/extensions", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		extension := &pd.Extension{}
		err := json.NewDecoder(r.Body).Decode(&extension)
		c.Assert(err, IsNil)

		counter := 0
		s.extensions.Range(func(_, _ interface{}) bool {
			counter += 1
			return true
		})
		extension.ID = fmt.Sprintf("extension-%v-%v", counter+1, time.Now().UnixNano())

		s.extensions.Store(extension.ID, *extension)
		s.newExtensions <- extension

		resp := map[string]*pd.Extension{"extension": extension}
		err = json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})
	fakePagerduty.PUT("/extensions/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		val, ok := s.extensions.Load(id)
		if !ok {
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}
		extension := val.(pd.Extension)
		err := json.NewDecoder(r.Body).Decode(&extension)
		c.Assert(err, IsNil)

		extension.ID = id
		s.extensions.Store(extension.ID, extension)
		s.newExtensions <- &extension

		rw.WriteHeader(http.StatusOK)
		resp := map[string]pd.Extension{"extension": extension}
		err = json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})
	fakePagerduty.POST("/incidents", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		payload := make(map[string]*pd.CreateIncidentOptions)
		err := json.NewDecoder(r.Body).Decode(&payload)
		c.Assert(err, IsNil)

		createOpts := payload["incident"]
		c.Assert(createOpts, NotNil)

		counter := 0
		s.incidents.Range(func(_, _ interface{}) bool {
			counter += 1
			return true
		})
		id := fmt.Sprintf("incident-%v-%v", counter+1, time.Now().UnixNano())
		incident := pd.Incident{
			APIObject:   pd.APIObject{ID: id},
			Id:          id,
			IncidentKey: createOpts.IncidentKey,
			Title:       createOpts.Title,
			Status:      "triggered",
			Service: pd.APIObject{
				Type: createOpts.Service.Type,
				ID:   createOpts.Service.ID,
			},
			Body: pd.IncidentBody{
				Type:    createOpts.Body.Type,
				Details: createOpts.Body.Details,
			},
		}

		s.incidents.Store(incident.ID, incident)
		s.newIncidents <- &incident

		resp := map[string]pd.Incident{"incident": incident}
		err = json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})
	fakePagerduty.PUT("/incidents", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		payload := make(map[string][]pd.ManageIncidentsOptions)
		err := json.NewDecoder(r.Body).Decode(&payload)
		c.Assert(err, IsNil)

		incidents := []pd.Incident{}
		for _, opt := range payload["incidents"] {
			incident := s.getIncident(opt.ID)
			if incident == nil {
				http.Error(rw, `{}`, http.StatusNotFound)
				return
			}
			incident.Status = opt.Status
			incidents = append(incidents, *incident)
		}

		for _, incident := range incidents {
			s.incidents.Store(incident.Id, incident)
		}

		rw.WriteHeader(http.StatusOK)
		resp := map[string][]pd.Incident{"incidents": incidents}
		err = json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})
	fakePagerduty.POST("/incidents/:id/notes", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		payload := make(map[string]*pd.IncidentNote)
		err := json.NewDecoder(r.Body).Decode(&payload)
		c.Assert(err, IsNil)

		note := payload["note"]
		c.Assert(note, NotNil)

		s.newIncidentNotes <- note

		resp := pd.CreateIncidentNoteResponse{IncidentNote: *note}
		err = json.NewEncoder(rw).Encode(&resp)
		c.Assert(err, IsNil)
	})

	s.fakePagerdutySrv = httptest.NewServer(fakePagerduty)
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

	var conf Config
	conf.Teleport.AuthServer = s.teleport.Config.Auth.SSHAddr.Addr
	conf.Teleport.ClientCrt = certFile.Name()
	conf.Teleport.ClientKey = keyFile.Name()
	conf.Teleport.RootCAs = casFile.Name()
	conf.Pagerduty.APIEndpoint = s.fakePagerdutySrv.URL
	conf.Pagerduty.UserEmail = "bot@example.com"
	conf.Pagerduty.ServiceId = "1111"
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

func (s *PagerdutySuite) createAccessRequest(c *C) services.AccessRequest {
	client, err := s.teleport.NewClient(integration.ClientConfig{Login: s.me.Username})
	c.Assert(err, IsNil)
	req, err := services.NewAccessRequest(s.me.Username, "admin")
	c.Assert(err, IsNil)
	err = client.CreateAccessRequest(context.TODO(), req)
	c.Assert(err, IsNil)
	time.Sleep(time.Millisecond * 250) // Wait some time for watcher to receive a request
	return req
}

func (s *PagerdutySuite) createExpiredAccessRequest(c *C) services.AccessRequest {
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

func (s *PagerdutySuite) getIncident(id string) *pd.Incident {
	if obj, ok := s.incidents.Load(id); ok {
		incident := obj.(pd.Incident)
		return &incident
	} else {
		return nil
	}
}

func (s *PagerdutySuite) postAction(c *C, incident *pd.Incident, action string) {
	payload := WebhookPayload{
		Messages: []WebhookMessage{
			WebhookMessage{
				ID:       "MSG1",
				Event:    "incident.custom",
				Incident: incident,
			},
		},
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&payload)
	c.Assert(err, IsNil)
	req, err := http.NewRequest("POST", s.webhookUrl+action, &buf)
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
	var extension1 *pd.Extension
	select {
	case extension1 = <-s.newExtensions:
		c.Assert(extension1, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("first extension wasn't created")
	}

	var extension2 *pd.Extension
	select {
	case extension2 = <-s.newExtensions:
		c.Assert(extension2, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("second extension wasn't created")
	}

	extTitles := []string{extension1.Name, extension2.Name}
	sort.Strings(extTitles)
	c.Assert(extTitles[0], Equals, pdApproveActionLabel)
	c.Assert(extTitles[1], Equals, pdDenyActionLabel)

	extEndpoints := []string{extension1.EndpointURL, extension2.EndpointURL}
	sort.Strings(extEndpoints)
	c.Assert(extEndpoints[0], Equals, s.webhookUrl+pdApproveAction)
	c.Assert(extEndpoints[1], Equals, s.webhookUrl+pdDenyAction)
}

func (s *PagerdutySuite) TestIncidentCreation(c *C) {
	req := s.createAccessRequest(c)

	var incident *pd.Incident
	select {
	case incident = <-s.newIncidents:
		c.Assert(incident, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("incident wasn't created")
	}

	c.Assert(incident.IncidentKey, Equals, pdIncidentKeyPrefix+"/"+req.GetName())
}

func (s *PagerdutySuite) TestApproval(c *C) {
	request := s.createAccessRequest(c)

	var incident *pd.Incident
	select {
	case incident = <-s.newIncidents:
		c.Assert(incident, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("incident wasn't created")
	}
	c.Assert(incident.Status, Equals, "triggered")

	s.postAction(c, incident, pdApproveAction)

	incident = s.getIncident(incident.Id)
	c.Assert(incident, NotNil)
	c.Assert(incident.Status, Equals, "resolved")

	var note *pd.IncidentNote
	select {
	case note = <-s.newIncidentNotes:
		c.Assert(note, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("note wasn't created")
	}
	c.Assert(note.Content, Equals, "Access request has been approved")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_APPROVED)
}

func (s *PagerdutySuite) TestDenial(c *C) {
	request := s.createAccessRequest(c)

	var incident *pd.Incident
	select {
	case incident = <-s.newIncidents:
		c.Assert(incident, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("incident wasn't created")
	}
	c.Assert(incident.Status, Equals, "triggered")

	s.postAction(c, incident, pdDenyAction)

	incident = s.getIncident(incident.Id)
	c.Assert(incident, NotNil)
	c.Assert(incident.Status, Equals, "resolved")

	var note *pd.IncidentNote
	select {
	case note = <-s.newIncidentNotes:
		c.Assert(note, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("note wasn't created")
	}
	c.Assert(note.Content, Equals, "Access request has been denied")

	auth := s.teleport.Process.GetAuthServer()
	requests, err := auth.GetAccessRequests(context.TODO(), services.AccessRequestFilter{ID: request.GetName()})
	c.Assert(err, IsNil)
	c.Assert(requests, HasLen, 1)
	request = requests[0]
	c.Assert(request.GetState(), Equals, services.RequestState_DENIED)
}

func (s *PagerdutySuite) TestApproveExpired(c *C) {
	s.createExpiredAccessRequest(c)

	var incident *pd.Incident
	select {
	case incident = <-s.newIncidents:
		c.Assert(incident, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("incident wasn't created")
	}

	s.postAction(c, incident, pdApproveAction)

	incident = s.getIncident(incident.Id)
	c.Assert(incident, NotNil)
	c.Assert(incident.Status, Equals, "resolved")

	var note *pd.IncidentNote
	select {
	case note = <-s.newIncidentNotes:
		c.Assert(note, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("note wasn't created")
	}
	c.Assert(note.Content, Equals, "Access request has been expired")
}

func (s *PagerdutySuite) TestDenyExpired(c *C) {
	s.createExpiredAccessRequest(c)

	var incident *pd.Incident
	select {
	case incident = <-s.newIncidents:
		c.Assert(incident, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("incident wasn't created")
	}

	s.postAction(c, incident, pdApproveAction)

	incident = s.getIncident(incident.Id)
	c.Assert(incident, NotNil)
	c.Assert(incident.Status, Equals, "resolved")

	var note *pd.IncidentNote
	select {
	case note = <-s.newIncidentNotes:
		c.Assert(note, NotNil)
	case <-time.After(time.Millisecond * 250):
		c.Fatal("note wasn't created")
	}
	c.Assert(note.Content, Equals, "Access request has been expired")
}
