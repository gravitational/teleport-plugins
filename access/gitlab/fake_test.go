package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"

	. "gopkg.in/check.v1"
)

type FakeGitlab struct {
	srv                *httptest.Server
	idCounter          uint64
	projectHooks       sync.Map
	newProjectHooks    chan ProjectHook
	projectHookUpdates chan ProjectHook
	labels             sync.Map
	newLabels          chan Label
	issues             sync.Map
	newIssues          chan Issue
	issueUpdates       chan Issue
}

func NewFakeGitLab(c *C, projectID IntID) *FakeGitlab {
	router := httprouter.New()

	self := &FakeGitlab{
		newIssues:          make(chan Issue, 20),
		issueUpdates:       make(chan Issue, 20),
		newProjectHooks:    make(chan ProjectHook, 20),
		projectHookUpdates: make(chan ProjectHook, 20),
		newLabels:          make(chan Label, 20),
		srv:                httptest.NewServer(router),
	}

	// Projects

	router.GET("/api/v4/projects/:project_id", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		err := json.NewEncoder(rw).Encode(&Project{ID: projectID})
		c.Assert(err, IsNil)
	})

	// Hooks

	router.GET("/api/v4/projects/:project_id/hooks", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var hooks []ProjectHook
		self.projectHooks.Range(func(key, value interface{}) bool {
			hook := value.(ProjectHook)
			hook.ID = key.(IntID)
			hooks = append(hooks, hook)
			return true
		})

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		err := json.NewEncoder(rw).Encode(hooks)
		c.Assert(err, IsNil)
	})
	router.POST("/api/v4/projects/:project_id/hooks", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var hook ProjectHook
		err := json.NewDecoder(r.Body).Decode(&hook)
		c.Assert(err, IsNil)

		hook = self.storeProjectHook(hook)
		self.newProjectHooks <- hook

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(&hook)
		c.Assert(err, IsNil)
	})
	router.PUT("/api/v4/projects/:project_id/hooks/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id, err := strconv.ParseUint(ps.ByName("id"), 10, 64)
		c.Assert(err, IsNil)

		hook, found := self.getProjectHook(IntID(id))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Hook not found"})
			c.Assert(err, IsNil)
			return
		}

		err = json.NewDecoder(r.Body).Decode(&hook)
		c.Assert(err, IsNil)
		hook.ID = IntID(id)

		self.storeProjectHook(hook)
		self.projectHookUpdates <- hook

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		err = json.NewEncoder(rw).Encode(&hook)
		c.Assert(err, IsNil)
	})

	// Labels

	router.GET("/api/v4/projects/:project_id/labels", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var labels []Label
		self.labels.Range(func(key, value interface{}) bool {
			label := value.(Label)
			id, ok := key.(IntID)
			if ok {
				label.ID = id
				labels = append(labels, label)
			}
			return true
		})

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		err := json.NewEncoder(rw).Encode(labels)
		c.Assert(err, IsNil)
	})
	router.POST("/api/v4/projects/:project_id/labels", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var label Label
		err := json.NewDecoder(r.Body).Decode(&label)
		c.Assert(err, IsNil)

		label, ok := self.storeLabelIfNotExists(label)
		if !ok {
			rw.WriteHeader(http.StatusBadRequest)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Label already exists"})
			c.Assert(err, IsNil)
			return
		}
		self.newLabels <- label

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(&label)
		c.Assert(err, IsNil)
	})

	// Issues

	router.POST("/api/v4/projects/:project_id/issues", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID, err := strconv.ParseUint(ps.ByName("project_id"), 10, 64)
		c.Assert(err, IsNil)

		var params IssueParams
		err = json.NewDecoder(r.Body).Decode(&params)
		c.Assert(err, IsNil)
		issue := self.storeIssue(Issue{
			ProjectID:   IntID(projectID),
			Title:       params.Title,
			Description: params.Description,
			State:       "opened",
			Labels:      self.getLabelsFromStr(params.Labels),
		})
		self.newIssues <- issue

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(&issue)
		c.Assert(err, IsNil)
	})
	router.PUT("/api/v4/projects/:project_id/issues/:iid", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		iid, err := strconv.ParseUint(ps.ByName("iid"), 10, 64)
		c.Assert(err, IsNil)

		issue, found := self.getIssue(IntID(iid))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Issue not found"})
			c.Assert(err, IsNil)
			return
		}

		var params IssueParams
		err = json.NewDecoder(r.Body).Decode(&params)
		c.Assert(err, IsNil)

		issue.Title = params.Title
		issue.Labels = self.getLabelsFromStr(params.Labels)
		switch params.StateEvent {
		case "close":
			issue.State = "closed"
		default:
			c.Fatalf("unknown StateEvent=%q", params.StateEvent)
		}

		self.storeIssue(issue)
		self.issueUpdates <- issue

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		err = json.NewEncoder(rw).Encode(&issue)
		c.Assert(err, IsNil)
	})
	router.GET("/api/v4/projects/:project_id/issues/:iid", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		issueIID, err := strconv.ParseUint(ps.ByName("iid"), 10, 64)
		c.Assert(err, IsNil)

		issue, found := self.getIssue(IntID(issueIID))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Hook not found"})
			c.Assert(err, IsNil)
			return
		}

		rw.WriteHeader(http.StatusOK)
		err = json.NewEncoder(rw).Encode(&issue)
		c.Assert(err, IsNil)
	})

	return self
}

func (s *FakeGitlab) URL() string {
	return s.srv.URL
}

func (s *FakeGitlab) Close() {
	s.srv.Close()
	close(s.newIssues)
	close(s.issueUpdates)
	close(s.newProjectHooks)
	close(s.projectHookUpdates)
	close(s.newLabels)
}

func (s *FakeGitlab) getIssue(id IntID) (Issue, bool) {
	if obj, ok := s.issues.Load(id); ok {
		return obj.(Issue), true
	}
	return Issue{}, false
}

func (s *FakeGitlab) storeIssue(issue Issue) Issue {
	if issue.ID == 0 {
		issue.ID = IntID(atomic.AddUint64(&s.idCounter, 1))
	}
	issue.IID = issue.ID
	s.issues.Store(issue.ID, issue)
	return issue
}

func (s *FakeGitlab) getLabel(key interface{}) (Label, bool) {
	if obj, ok := s.labels.Load(key); ok {
		return obj.(Label), true
	}
	return Label{}, false
}

func (s *FakeGitlab) getLabelsFromStr(str string) (labels []Label) {
	names := strings.Split(str, ",")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if label, exists := s.getLabel(name); exists {
			labels = append(labels, label)
		}
	}
	return
}

func (s *FakeGitlab) storeLabel(label Label) Label {
	if label.Name != "" {
		label.Title = label.Name
	} else {
		label.Name = label.Title
	}

	if label.ID == 0 {
		label.ID = IntID(atomic.AddUint64(&s.idCounter, 1))
	}
	s.labels.Store(label.ID, label)
	s.labels.Store(label.Name, label)
	return label
}

func (s *FakeGitlab) storeLabelIfNotExists(label Label) (Label, bool) {
	var name string
	if label.Name != "" {
		name = label.Name
	} else {
		name = label.Title
	}
	if existingLabel, exists := s.getLabel(name); exists {
		return existingLabel, false
	}

	return s.storeLabel(label), true
}

func (s *FakeGitlab) getProjectHook(id IntID) (ProjectHook, bool) {
	if obj, ok := s.projectHooks.Load(id); ok {
		return obj.(ProjectHook), true
	}
	return ProjectHook{}, false
}

func (s *FakeGitlab) storeProjectHook(hook ProjectHook) ProjectHook {
	if hook.ID == 0 {
		hook.ID = IntID(atomic.AddUint64(&s.idCounter, 1))
	}
	s.projectHooks.Store(hook.ID, hook)
	return hook
}

func (s *FakeGitlab) checkNewProjectHook(c *C) ProjectHook {
	select {
	case hook := <-s.newProjectHooks:
		return hook
	default:
		c.Fatal("no new project hooks stored")
	}
	return ProjectHook{}
}

func (s *FakeGitlab) checkProjectHookUpdate(c *C) ProjectHook {
	select {
	case hook := <-s.projectHookUpdates:
		return hook
	default:
		c.Fatal("no project hooks updated")
	}
	return ProjectHook{}
}

func (s *FakeGitlab) checkNoNewProjectHooks(c *C) {
	select {
	case <-s.newProjectHooks:
		c.Fatal("extra project hooks stored")
	default:
		// OK
	}
}

func (s *FakeGitlab) checkNewLabel(c *C) Label {
	select {
	case label := <-s.newLabels:
		return label
	default:
		c.Fatal("no new labels stored")
	}
	return Label{}
}

func (s *FakeGitlab) checkNewLabels(c *C, n int) map[string]Label {
	newLabels := make(map[string]Label)
	for i := 0; i < n; i++ {
		label := s.checkNewLabel(c)
		newLabels[LabelName(label.Name).Reduced()] = label
	}
	s.checkNoNewLabels(c)
	return newLabels
}

func (s *FakeGitlab) checkNoNewLabels(c *C) {
	select {
	case <-s.newLabels:
		c.Fatal("extra labels stored")
	default:
		// OK
	}
}

func (s *FakeGitlab) checkNewIssue(c *C) Issue {
	select {
	case issue := <-s.newIssues:
		return issue
	case <-time.After(time.Millisecond * 250):
		c.Fatal("no new issues stored")
	}
	return Issue{}
}

func (s *FakeGitlab) checkIssueUpdate(c *C) Issue {
	select {
	case issue := <-s.issueUpdates:
		return issue
	case <-time.After(time.Millisecond * 250):
		c.Fatal("no issues updates")
	}
	return Issue{}
}
