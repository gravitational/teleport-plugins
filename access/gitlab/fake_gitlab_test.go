package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"

	log "github.com/sirupsen/logrus"
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

func NewFakeGitLab(projectID IntID, concurrency int) *FakeGitlab {
	router := httprouter.New()

	gitlab := &FakeGitlab{
		newIssues:          make(chan Issue, concurrency),
		issueUpdates:       make(chan Issue, concurrency),
		newProjectHooks:    make(chan ProjectHook, concurrency),
		projectHookUpdates: make(chan ProjectHook, concurrency),
		newLabels:          make(chan Label, concurrency),
		srv:                httptest.NewServer(router),
	}

	// Projects

	router.GET("/api/v4/projects/:project_id", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(&Project{ID: projectID})
		fatalIf(err)
	})

	// Hooks

	router.GET("/api/v4/projects/:project_id/hooks", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var hooks []ProjectHook
		gitlab.projectHooks.Range(func(key, value interface{}) bool {
			hook := value.(ProjectHook)
			hook.ID = key.(IntID)
			hooks = append(hooks, hook)
			return true
		})

		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(hooks)
		fatalIf(err)
	})
	router.POST("/api/v4/projects/:project_id/hooks", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var hook ProjectHook
		err := json.NewDecoder(r.Body).Decode(&hook)
		fatalIf(err)

		hook = gitlab.StoreProjectHook(hook)
		gitlab.newProjectHooks <- hook

		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(&hook)
		fatalIf(err)
	})
	router.PUT("/api/v4/projects/:project_id/hooks/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id, err := strconv.ParseUint(ps.ByName("id"), 10, 64)
		fatalIf(err)

		hook, found := gitlab.GetProjectHook(IntID(id))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Hook not found"})
			fatalIf(err)
			return
		}

		err = json.NewDecoder(r.Body).Decode(&hook)
		fatalIf(err)
		hook.ID = IntID(id)

		gitlab.StoreProjectHook(hook)
		gitlab.projectHookUpdates <- hook

		rw.Header().Add("Content-Type", "application/json")
		err = json.NewEncoder(rw).Encode(&hook)
		fatalIf(err)
	})

	// Labels

	router.GET("/api/v4/projects/:project_id/labels", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var labels []Label
		gitlab.labels.Range(func(key, value interface{}) bool {
			label := value.(Label)
			id, ok := key.(IntID)
			if ok {
				label.ID = id
				labels = append(labels, label)
			}
			return true
		})

		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(labels)
		fatalIf(err)
	})
	router.POST("/api/v4/projects/:project_id/labels", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var label Label
		err := json.NewDecoder(r.Body).Decode(&label)
		fatalIf(err)

		label, ok := gitlab.StoreLabelIfNotExists(label)
		if !ok {
			rw.WriteHeader(http.StatusBadRequest)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Label already exists"})
			fatalIf(err)
			return
		}
		gitlab.newLabels <- label

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(&label)
		fatalIf(err)
	})

	// Issues

	router.POST("/api/v4/projects/:project_id/issues", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID, err := strconv.ParseUint(ps.ByName("project_id"), 10, 64)
		fatalIf(err)

		var params IssueParams
		err = json.NewDecoder(r.Body).Decode(&params)
		fatalIf(err)
		issue := gitlab.StoreIssue(Issue{
			ProjectID:   IntID(projectID),
			Title:       params.Title,
			Description: params.Description,
			State:       "opened",
			Labels:      gitlab.GetLabelsFromStr(params.Labels),
		})
		gitlab.newIssues <- issue

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(&issue)
		fatalIf(err)
	})
	router.PUT("/api/v4/projects/:project_id/issues/:iid", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		iid, err := strconv.ParseUint(ps.ByName("iid"), 10, 64)
		fatalIf(err)

		issue, found := gitlab.GetIssue(IntID(iid))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Issue not found"})
			fatalIf(err)
			return
		}

		var params IssueParams
		err = json.NewDecoder(r.Body).Decode(&params)
		fatalIf(err)

		issue.Title = params.Title
		issue.Labels = gitlab.GetLabelsFromStr(params.Labels)
		switch params.StateEvent {
		case "close":
			issue.State = "closed"
		default:
			log.Fatalf("unknown StateEvent=%q", params.StateEvent)
		}

		gitlab.StoreIssue(issue)
		gitlab.issueUpdates <- issue

		rw.Header().Add("Content-Type", "application/json")
		err = json.NewEncoder(rw).Encode(&issue)
		fatalIf(err)
	})
	router.GET("/api/v4/projects/:project_id/issues/:iid", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		issueIID, err := strconv.ParseUint(ps.ByName("iid"), 10, 64)
		fatalIf(err)

		issue, found := gitlab.GetIssue(IntID(issueIID))
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err = json.NewEncoder(rw).Encode(&ErrorResult{Message: "Hook not found"})
			fatalIf(err)
			return
		}

		err = json.NewEncoder(rw).Encode(&issue)
		fatalIf(err)
	})

	return gitlab
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

func (s *FakeGitlab) GetIssue(id IntID) (Issue, bool) {
	if obj, ok := s.issues.Load(id); ok {
		return obj.(Issue), true
	}
	return Issue{}, false
}

func (s *FakeGitlab) StoreIssue(issue Issue) Issue {
	if issue.ID == 0 {
		issue.ID = IntID(atomic.AddUint64(&s.idCounter, 1))
	}
	issue.IID = issue.ID
	s.issues.Store(issue.ID, issue)
	return issue
}

func (s *FakeGitlab) GetLabel(key interface{}) (Label, bool) {
	if obj, ok := s.labels.Load(key); ok {
		return obj.(Label), true
	}
	return Label{}, false
}

func (s *FakeGitlab) GetLabelsFromStr(str string) (labels []Label) {
	names := strings.Split(str, ",")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if label, exists := s.GetLabel(name); exists {
			labels = append(labels, label)
		}
	}
	return
}

func (s *FakeGitlab) StoreLabel(label Label) Label {
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

func (s *FakeGitlab) StoreLabelIfNotExists(label Label) (Label, bool) {
	var name string
	if label.Name != "" {
		name = label.Name
	} else {
		name = label.Title
	}
	if existingLabel, exists := s.GetLabel(name); exists {
		return existingLabel, false
	}

	return s.StoreLabel(label), true
}

func (s *FakeGitlab) GetProjectHook(id IntID) (ProjectHook, bool) {
	if obj, ok := s.projectHooks.Load(id); ok {
		return obj.(ProjectHook), true
	}
	return ProjectHook{}, false
}

func (s *FakeGitlab) StoreProjectHook(hook ProjectHook) ProjectHook {
	if hook.ID == 0 {
		hook.ID = IntID(atomic.AddUint64(&s.idCounter, 1))
	}
	s.projectHooks.Store(hook.ID, hook)
	return hook
}

func (s *FakeGitlab) CheckNewProjectHook(ctx context.Context) (ProjectHook, error) {
	select {
	case hook := <-s.newProjectHooks:
		return hook, nil
	case <-ctx.Done():
		return ProjectHook{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeGitlab) CheckProjectHookUpdate(ctx context.Context) (ProjectHook, error) {
	select {
	case hook := <-s.projectHookUpdates:
		return hook, nil
	case <-ctx.Done():
		return ProjectHook{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeGitlab) CheckNoNewProjectHooks() bool {
	select {
	case <-s.newProjectHooks:
		return false
	default:
		return true
	}
}

func (s *FakeGitlab) GetAllNewLabels() map[string]Label {
	newLabels := make(map[string]Label)
	for {
		select {
		case label := <-s.newLabels:
			newLabels[LabelName(label.Name).Reduced()] = label
		default:
			return newLabels
		}
	}
}

func (s *FakeGitlab) CheckNewIssue(ctx context.Context) (Issue, error) {
	select {
	case issue := <-s.newIssues:
		return issue, nil
	case <-ctx.Done():
		return Issue{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeGitlab) CheckIssueUpdate(ctx context.Context) (Issue, error) {
	select {
	case issue := <-s.issueUpdates:
		return issue, nil
	case <-ctx.Done():
		return Issue{}, trace.Wrap(ctx.Err())
	}
}

func fatalIf(err error) {
	if err != nil {
		log.Fatalf("%v at %v", err, string(debug.Stack()))
	}
}
