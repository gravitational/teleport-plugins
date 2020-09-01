package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"

	log "github.com/sirupsen/logrus"
)

type FakePagerduty struct {
	srv *httptest.Server

	objects sync.Map
	// Extensions
	extensionIDCounter uint64
	newExtensions      chan Extension
	// Inicidents
	incidentIDCounter uint64
	newIncidents      chan Incident
	incidentUpdates   chan Incident
	// Incident notes
	newIncidentNotes      chan IncidentNote
	incidentNoteIDCounter uint64
	// Services
	serviceIDCounter uint64
	// Users
	userIDCounter uint64
	// OnCalls
	onCallIDCounter uint64
}

func NewFakePagerduty(concurrency int) *FakePagerduty {
	router := httprouter.New()

	pagerduty := &FakePagerduty{
		newExtensions:    make(chan Extension, concurrency),
		newIncidents:     make(chan Incident, concurrency),
		incidentUpdates:  make(chan Incident, concurrency),
		newIncidentNotes: make(chan IncidentNote, concurrency),
		srv:              httptest.NewServer(router),
	}

	router.GET("/services/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		service, found := pagerduty.GetService(id)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(&ErrorResult{Message: "Service not found"})
			fatalIf(err)
			return
		}

		err := json.NewEncoder(rw).Encode(&ServiceResult{Service: service})
		fatalIf(err)
	})
	router.GET("/extension_schemas", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		resp := ListExtensionSchemasResult{
			PaginationResult: PaginationResult{
				More:  false,
				Total: 1,
			},
			ExtensionSchemas: []ExtensionSchema{
				{
					ID:  "11",
					Key: "custom_webhook",
				},
			},
		}
		err := json.NewEncoder(rw).Encode(&resp)
		fatalIf(err)
	})
	router.GET("/extensions", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		extensions := []Extension{}
		pagerduty.objects.Range(func(key, value interface{}) bool {
			if extension, ok := value.(Extension); ok {
				extensions = append(extensions, extension)
			}
			return true
		})
		resp := ListExtensionsResult{
			PaginationResult: PaginationResult{
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

		var body ExtensionBodyWrap
		err := json.NewDecoder(r.Body).Decode(&body)
		fatalIf(err)

		extension := pagerduty.StoreExtension(Extension{
			Name:             body.Extension.Name,
			EndpointURL:      body.Extension.EndpointURL,
			ExtensionObjects: body.Extension.ExtensionObjects,
			ExtensionSchema:  body.Extension.ExtensionSchema,
		})
		pagerduty.newExtensions <- extension

		err = json.NewEncoder(rw).Encode(&ExtensionResult{Extension: extension})
		fatalIf(err)
	})
	router.PUT("/extensions/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		extension, found := pagerduty.GetExtension(ps.ByName("id"))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(&ErrorResult{Message: "Extension not found"})
			fatalIf(err)
			return
		}

		err := json.NewDecoder(r.Body).Decode(&extension)
		fatalIf(err)

		pagerduty.StoreExtension(extension)
		pagerduty.newExtensions <- extension

		err = json.NewEncoder(rw).Encode(&ExtensionResult{Extension: extension})
		fatalIf(err)
	})
	router.GET("/users", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		users := []User{}
		pagerduty.objects.Range(func(key, value interface{}) bool {
			if user, ok := value.(User); ok {
				users = append(users, user)
			}
			return true
		})
		err := json.NewEncoder(rw).Encode(&ListUsersResult{Users: users})
		fatalIf(err)
	})
	router.GET("/users/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		user, found := pagerduty.GetUser(id)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(&ErrorResult{Message: "User not found"})
			fatalIf(err)
			return
		}

		err := json.NewEncoder(rw).Encode(&UserResult{User: user})
		fatalIf(err)
	})
	router.POST("/incidents", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		var body IncidentBodyWrap
		err := json.NewDecoder(r.Body).Decode(&body)
		fatalIf(err)

		incident := pagerduty.StoreIncident(Incident{
			IncidentKey: body.Incident.IncidentKey,
			Title:       body.Incident.Title,
			Status:      "triggered",
			Service:     body.Incident.Service,
			Body:        body.Incident.Body,
		})
		pagerduty.newIncidents <- incident

		err = json.NewEncoder(rw).Encode(&IncidentResult{Incident: incident})
		fatalIf(err)
	})
	router.PUT("/incidents/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		incident, found := pagerduty.GetIncident(ps.ByName("id"))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(&ErrorResult{Message: "Incident not found"})
			fatalIf(err)
			return
		}

		var body IncidentBodyWrap
		err := json.NewDecoder(r.Body).Decode(&body)
		fatalIf(err)

		incident.Status = body.Incident.Status
		pagerduty.StoreIncident(incident)
		pagerduty.incidentUpdates <- incident

		err = json.NewEncoder(rw).Encode(&IncidentResult{Incident: incident})
		fatalIf(err)
	})
	router.POST("/incidents/:id/notes", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)

		var body IncidentNoteBodyWrap
		err := json.NewDecoder(r.Body).Decode(&body)
		fatalIf(err)

		note := pagerduty.StoreIncidentNote(IncidentNote{Content: body.Note.Content})
		pagerduty.newIncidentNotes <- note

		err = json.NewEncoder(rw).Encode(&IncidentNoteResult{Note: note})
		fatalIf(err)
	})
	router.GET("/oncalls", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		query := r.URL.Query()
		var onCalls []OnCall
		pagerduty.objects.Range(func(key, value interface{}) bool {
			onCall, ok := value.(OnCall)
			if !ok {
				return true
			}
			// Filter by user_ids
			if ids := query["user_ids[]"]; len(ids) > 0 {
				ok := false
				for _, id := range ids {
					if onCall.User.ID == id {
						ok = true
						break
					}
				}
				if !ok {
					return true
				}
			}
			// Filter by escalation_policy_ids
			if ids := query["escalation_policy_ids[]"]; len(ids) > 0 {
				ok := false
				for _, id := range ids {
					if onCall.EscalationPolicy.ID == id {
						ok = true
						break
					}
				}
				if !ok {
					return true
				}
			}
			onCalls = append(onCalls, onCall)
			return true
		})

		err := json.NewEncoder(rw).Encode(&ListOnCallsResult{OnCalls: onCalls})
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

func (s *FakePagerduty) GetService(id string) (Service, bool) {
	if obj, ok := s.objects.Load(id); ok {
		service, ok := obj.(Service)
		return service, ok
	}
	return Service{}, false
}

func (s *FakePagerduty) StoreService(service Service) Service {
	if service.ID == "" {
		service.ID = fmt.Sprintf("service-%v", atomic.AddUint64(&s.serviceIDCounter, 1))
	}
	s.objects.Store(service.ID, service)
	return service
}

func (s *FakePagerduty) GetExtension(id string) (Extension, bool) {
	if obj, ok := s.objects.Load(id); ok {
		extension, ok := obj.(Extension)
		return extension, ok
	}
	return Extension{}, false
}

func (s *FakePagerduty) StoreExtension(extension Extension) Extension {
	if extension.ID == "" {
		extension.ID = fmt.Sprintf("extension-%v", atomic.AddUint64(&s.extensionIDCounter, 1))
	}
	s.objects.Store(extension.ID, extension)
	return extension
}

func (s *FakePagerduty) GetUser(id string) (User, bool) {
	if obj, ok := s.objects.Load(id); ok {
		user, ok := obj.(User)
		return user, ok
	}
	return User{}, false
}

func (s *FakePagerduty) StoreUser(user User) User {
	if user.ID == "" {
		user.ID = fmt.Sprintf("user-%v", atomic.AddUint64(&s.userIDCounter, 1))
	}
	s.objects.Store(user.ID, user)
	return user
}

func (s *FakePagerduty) GetIncident(id string) (Incident, bool) {
	if obj, ok := s.objects.Load(id); ok {
		incident, ok := obj.(Incident)
		return incident, ok
	}
	return Incident{}, false
}

func (s *FakePagerduty) StoreIncident(incident Incident) Incident {
	if incident.ID == "" {
		incident.ID = fmt.Sprintf("incident-%v", atomic.AddUint64(&s.incidentIDCounter, 1))
	}
	s.objects.Store(incident.ID, incident)
	return incident
}

func (s *FakePagerduty) StoreIncidentNote(note IncidentNote) IncidentNote {
	if note.ID == "" {
		note.ID = fmt.Sprintf("incident_note-%v", atomic.AddUint64(&s.incidentNoteIDCounter, 1))
	}
	s.objects.Store(note.ID, note)
	return note
}

func (s *FakePagerduty) StoreOnCall(onCall OnCall) OnCall {
	id := fmt.Sprintf("oncall-%v", atomic.AddUint64(&s.onCallIDCounter, 1))
	s.objects.Store(id, onCall)
	return onCall
}

func (s *FakePagerduty) CheckNewExtension(ctx context.Context) (Extension, error) {
	select {
	case extension := <-s.newExtensions:
		return extension, nil
	case <-ctx.Done():
		return Extension{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakePagerduty) CheckNewIncident(ctx context.Context) (Incident, error) {
	select {
	case incident := <-s.newIncidents:
		return incident, nil
	case <-ctx.Done():
		return Incident{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakePagerduty) CheckIncidentUpdate(ctx context.Context) (Incident, error) {
	select {
	case incident := <-s.incidentUpdates:
		return incident, nil
	case <-ctx.Done():
		return Incident{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakePagerduty) CheckNewIncidentNote(ctx context.Context) (IncidentNote, error) {
	select {
	case note := <-s.newIncidentNotes:
		return note, nil
	case <-ctx.Done():
		return IncidentNote{}, trace.Wrap(ctx.Err())
	}
}

func fatalIf(err error) {
	if err != nil {
		log.Fatalf("%v at %v", err, string(debug.Stack()))
	}
}
