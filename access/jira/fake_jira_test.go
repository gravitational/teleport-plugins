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

	"github.com/andygrunwald/go-jira"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"

	log "github.com/sirupsen/logrus"
)

type FakeJIRA struct {
	srv              *httptest.Server
	issues           sync.Map
	newIssues        chan Issue
	issueTransitions chan Issue
	author           jira.User
	issueIDCounter   uint64
}

func NewFakeJIRA(author jira.User, concurrency int) *FakeJIRA {
	router := httprouter.New()

	self := &FakeJIRA{
		newIssues:        make(chan Issue, concurrency),
		issueTransitions: make(chan Issue, concurrency),
		srv:              httptest.NewServer(router),
		author:           author,
	}

	router.GET("/rest/api/2/myself", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
	})
	router.GET("/rest/api/2/project/PROJ", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		project := jira.Project{
			Key:  "PROJ",
			Name: "The Project",
		}
		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(&project)
		panicIf(err)
	})
	router.GET("/rest/api/2/mypermissions", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		permissions := Permissions{
			Permissions: map[string]Permission{
				"BROWSE_PROJECTS": {
					HavePermission: true,
				},
				"CREATE_ISSUES": {
					HavePermission: true,
				},
			},
		}
		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(&permissions)
		panicIf(err)
	})
	router.POST("/rest/api/2/issue", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var issueInput IssueInput

		err := json.NewDecoder(r.Body).Decode(&issueInput)
		panicIf(err)

		issue := Issue{
			Fields:     issueInput.Fields,
			Properties: make(map[string]interface{}),
		}
		for _, property := range issueInput.Properties {
			issue.Properties[property.Key] = property.Value
		}
		if issue.Fields == nil {
			issue.Fields = &jira.IssueFields{}
		}
		issue.Fields.Status = &jira.Status{Name: "Pending"}
		issue.Transitions = []jira.Transition{
			{
				ID: "100001", To: jira.Status{Name: "Approved"},
			},
			{
				ID: "100002", To: jira.Status{Name: "Denied"},
			},
			{
				ID: "100003", To: jira.Status{Name: "Expired"},
			},
		}
		issue = self.StoreIssue(issue)
		self.newIssues <- issue

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(issue)
		panicIf(err)
	})
	router.GET("/rest/api/2/issue/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		issue, found := self.GetIssue(ps.ByName("id"))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(issue)
		panicIf(err)
	})
	router.POST("/rest/api/2/issue/:id/transitions", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		issue, found := self.GetIssue(ps.ByName("id"))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		var payload jira.CreateTransitionPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		panicIf(err)

		switch payload.Transition.ID {
		case "100001":
			self.TransitionIssue(issue, "Approved")
		case "100002":
			self.TransitionIssue(issue, "Denied")
		case "100003":
			self.TransitionIssue(issue, "Expired")
		default:
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusNoContent)
	})
	return self
}

func (s *FakeJIRA) URL() string {
	return s.srv.URL
}

func (s *FakeJIRA) Close() {
	s.srv.Close()
	close(s.newIssues)
	close(s.issueTransitions)
}

func (s *FakeJIRA) GetAuthor() jira.User {
	return s.author
}

func (s *FakeJIRA) StoreIssue(issue Issue) Issue {
	if issue.ID == "" {
		id := atomic.AddUint64(&s.issueIDCounter, 1)
		issue.ID = fmt.Sprintf("%v", id)
		issue.Key = fmt.Sprintf("ISSUE-%v", id)
	}
	s.issues.Store(issue.ID, issue)
	s.issues.Store(issue.Key, issue)
	return issue
}

func (s *FakeJIRA) GetIssue(idOrKey string) (Issue, bool) {
	if obj, ok := s.issues.Load(idOrKey); ok {
		return obj.(Issue), true
	}
	return Issue{}, false
}

func (s *FakeJIRA) TransitionIssue(issue Issue, status string) Issue {
	if issue.Fields == nil {
		issue.Fields = &jira.IssueFields{}
	} else {
		copy := *issue.Fields
		issue.Fields = &copy
	}
	issue.Fields.Status = &jira.Status{Name: status}
	if issue.Changelog == nil {
		issue.Changelog = &jira.Changelog{}
	} else {
		copy := *issue.Changelog
		issue.Changelog = &copy
	}

	history := jira.ChangelogHistory{
		Author: s.author,
		Items: []jira.ChangelogItems{
			{
				FieldType: "jira",
				Field:     "status",
				ToString:  status,
			},
		},
	}
	issue.Changelog.Histories = append([]jira.ChangelogHistory{history}, issue.Changelog.Histories...)
	issue = s.StoreIssue(issue)
	s.issueTransitions <- issue
	return issue
}

func (s *FakeJIRA) CheckNewIssue(ctx context.Context) (Issue, error) {
	select {
	case issue := <-s.newIssues:
		return issue, nil
	case <-ctx.Done():
		return Issue{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeJIRA) CheckIssueTransition(ctx context.Context) (Issue, error) {
	select {
	case issue := <-s.issueTransitions:
		return issue, nil
	case <-ctx.Done():
		return Issue{}, trace.Wrap(ctx.Err())
	}
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("%v at %v", err, string(debug.Stack()))
	}
}
