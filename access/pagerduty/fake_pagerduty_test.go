package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"sync"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"

	log "github.com/sirupsen/logrus"
)

type FakePagerduty struct {
	srv              *httptest.Server
	extensions       sync.Map
	newExtensions    chan pd.Extension
	incidents        sync.Map
	newIncidents     chan pd.Incident
	incidentUpdates  chan pd.Incident
	newIncidentNotes chan pd.IncidentNote
}

func NewFakePagerduty() *FakePagerduty {
	router := httprouter.New()

	pagerduty := &FakePagerduty{
		newExtensions:    make(chan pd.Extension, 20),
		newIncidents:     make(chan pd.Incident, 20),
		incidentUpdates:  make(chan pd.Incident, 20),
		newIncidentNotes: make(chan pd.IncidentNote, 20),
		srv:              httptest.NewServer(router),
	}

	router.GET("/services/1111", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		service := pd.Service{
			APIObject: pd.APIObject{ID: "1111"},
			Name:      "Test Service",
		}
		resp := map[string]pd.Service{"service": service}
		err := json.NewEncoder(rw).Encode(&resp)
		fatalIf(err)
	})
	router.GET("/extension_schemas", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
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
		fatalIf(err)
	})
	router.GET("/extensions", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		extensions := []pd.Extension{}
		pagerduty.extensions.Range(func(key, value interface{}) bool {
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
		fatalIf(err)
	})
	router.POST("/extensions", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		var extension pd.Extension
		err := json.NewDecoder(r.Body).Decode(&extension)
		fatalIf(err)

		counter := 0
		pagerduty.extensions.Range(func(_, _ interface{}) bool {
			counter++
			return true
		})
		extension.ID = fmt.Sprintf("extension-%v-%v", counter+1, time.Now().UnixNano())

		pagerduty.extensions.Store(extension.ID, extension)
		pagerduty.newExtensions <- extension

		resp := map[string]pd.Extension{"extension": extension}
		err = json.NewEncoder(rw).Encode(&resp)
		fatalIf(err)
	})
	router.PUT("/extensions/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		val, ok := pagerduty.extensions.Load(id)
		if !ok {
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}
		extension := val.(pd.Extension)
		err := json.NewDecoder(r.Body).Decode(&extension)
		fatalIf(err)

		extension.ID = id
		pagerduty.extensions.Store(extension.ID, extension)
		pagerduty.newExtensions <- extension

		rw.WriteHeader(http.StatusOK)
		resp := map[string]pd.Extension{"extension": extension}
		err = json.NewEncoder(rw).Encode(&resp)
		fatalIf(err)
	})
	router.POST("/incidents", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		payload := make(map[string]*pd.CreateIncidentOptions)
		err := json.NewDecoder(r.Body).Decode(&payload)
		fatalIf(err)

		createOpts := payload["incident"]
		if createOpts == nil {
			log.Fatalf("no \"incident\" parameter in the request body")
		}

		counter := 0
		pagerduty.incidents.Range(func(_, _ interface{}) bool {
			counter++
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

		pagerduty.incidents.Store(incident.ID, incident)
		pagerduty.newIncidents <- incident

		resp := map[string]pd.Incident{"incident": incident}
		err = json.NewEncoder(rw).Encode(&resp)
		fatalIf(err)
	})
	router.PUT("/incidents", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		payload := make(map[string][]pd.ManageIncidentsOptions)
		err := json.NewDecoder(r.Body).Decode(&payload)
		fatalIf(err)

		var incidents []pd.Incident
		for _, opt := range payload["incidents"] {
			incident, found := pagerduty.GetIncident(opt.ID)
			if !found {
				http.Error(rw, `{}`, http.StatusNotFound)
				return
			}
			incident.Status = opt.Status
			incidents = append(incidents, incident)
		}

		for _, incident := range incidents {
			pagerduty.incidents.Store(incident.Id, incident)
			pagerduty.incidentUpdates <- incident
		}

		rw.WriteHeader(http.StatusOK)
		resp := map[string][]pd.Incident{"incidents": incidents}
		err = json.NewEncoder(rw).Encode(&resp)
		fatalIf(err)
	})
	router.POST("/incidents/:id/notes", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		payload := make(map[string]*pd.IncidentNote)
		err := json.NewDecoder(r.Body).Decode(&payload)
		fatalIf(err)

		notePtr := payload["note"]
		if notePtr == nil {
			log.Fatalf("no \"note\" parameter in the request body")
		}
		note := *notePtr

		pagerduty.newIncidentNotes <- note

		resp := pd.CreateIncidentNoteResponse{IncidentNote: note}
		err = json.NewEncoder(rw).Encode(&resp)
		fatalIf(err)
	})

	return pagerduty
}

func (s *FakePagerduty) URL() string {
	return s.srv.URL
}

func (s *FakePagerduty) Close() {
	s.srv.Close()
	close(s.newExtensions)
	close(s.newIncidents)
	close(s.incidentUpdates)
	close(s.newIncidentNotes)
}

func (s *FakePagerduty) CheckNewExtension(ctx context.Context, timeout time.Duration) (pd.Extension, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case extension := <-s.newExtensions:
		return extension, nil
	case <-ctx.Done():
		return pd.Extension{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakePagerduty) GetIncident(id string) (pd.Incident, bool) {
	if obj, ok := s.incidents.Load(id); ok {
		return obj.(pd.Incident), true
	}
	return pd.Incident{}, false
}

func (s *FakePagerduty) CheckNewIncident(ctx context.Context, timeout time.Duration) (pd.Incident, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case incident := <-s.newIncidents:
		return incident, nil
	case <-ctx.Done():
		return pd.Incident{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakePagerduty) CheckIncidentUpdate(ctx context.Context, timeout time.Duration) (pd.Incident, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case incident := <-s.incidentUpdates:
		return incident, nil
	case <-ctx.Done():
		return pd.Incident{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakePagerduty) CheckNewIncidentNote(ctx context.Context, timeout time.Duration) (pd.IncidentNote, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case note := <-s.newIncidentNotes:
		return note, nil
	case <-ctx.Done():
		return pd.IncidentNote{}, trace.Wrap(ctx.Err())
	}
}

func fatalIf(err error) {
	if err != nil {
		log.Fatalf("%v at %v", err, string(debug.Stack()))
	}
}
