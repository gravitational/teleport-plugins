/*
Copyright 2020-2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gravitational/teleport-plugins/lib/stringset"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"

	log "github.com/sirupsen/logrus"
)

type FakeGitlab struct {
	srv *httptest.Server

	objects sync.Map
	// Issues
	issueIDCounters sync.Map
	newIssues       chan Issue
	issueUpdates    chan Issue
	// Notes
	noteIDCounter uint64
	newNotes      chan Note
	// Project hooks
	projectHookIDCounter uint64
	newProjectHooks      chan ProjectHook
	projectHookUpdates   chan ProjectHook
	// Labels
	labelIDCounter uint64
	newLabels      chan Label
}

type fakeLabelByName string
type fakeProjectHookKey struct {
	ProjectID IntID
	HookIID   IntID
}
type fakeProjectIssueKey struct {
	ProjectID IntID
	IssueIID  IntID
}

func NewFakeGitLab(projectID IntID, concurrency int) *FakeGitlab {
	router := httprouter.New()

	gitlab := &FakeGitlab{
		newIssues:          make(chan Issue, concurrency),
		issueUpdates:       make(chan Issue, concurrency),
		newNotes:           make(chan Note, concurrency),
		newProjectHooks:    make(chan ProjectHook, concurrency),
		projectHookUpdates: make(chan ProjectHook, concurrency),
		newLabels:          make(chan Label, concurrency),
		srv:                httptest.NewServer(router),
	}

	// Projects

	router.GET("/api/v4/projects/:project_id", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(Project{ID: projectID})
		panicIf(err)
	})

	// Hooks

	router.GET("/api/v4/projects/:project_id/hooks", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		projectID := parseIntID(ps.ByName("project_id"))

		var hooks []ProjectHook
		gitlab.objects.Range(func(key, value interface{}) bool {
			_, ok := key.(fakeProjectHookKey)
			if !ok {
				return true
			}
			hook := value.(ProjectHook)
			if hook.ProjectID != projectID {
				return true
			}
			hooks = append(hooks, hook)
			return true
		})

		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(hooks)
		panicIf(err)
	})
	router.POST("/api/v4/projects/:project_id/hooks", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID := parseIntID(ps.ByName("project_id"))

		var hook ProjectHook
		err := json.NewDecoder(r.Body).Decode(&hook)
		panicIf(err)

		hook.ProjectID = projectID
		hook = gitlab.StoreProjectHook(hook)
		gitlab.newProjectHooks <- hook

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(&hook)
		panicIf(err)
	})
	router.PUT("/api/v4/projects/:project_id/hooks/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID := parseIntID(ps.ByName("project_id"))
		id := parseIntID(ps.ByName("id"))

		hook, found := gitlab.GetProjectHook(projectID, id)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{Message: "Hook not found"})
			panicIf(err)
			return
		}

		err := json.NewDecoder(r.Body).Decode(&hook)
		panicIf(err)
		hook.ID = id

		gitlab.StoreProjectHook(hook)
		gitlab.projectHookUpdates <- hook

		rw.Header().Add("Content-Type", "application/json")
		err = json.NewEncoder(rw).Encode(hook)
		panicIf(err)
	})

	// Labels

	router.GET("/api/v4/projects/:project_id/labels", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var labels []Label
		gitlab.objects.Range(func(key, value interface{}) bool {
			keyStr, ok := key.(string)
			if !ok {
				return true
			}
			if !strings.HasPrefix(keyStr, "label-") {
				return true
			}
			label := value.(Label)
			labels = append(labels, label)
			return true
		})

		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(labels)
		panicIf(err)
	})
	router.POST("/api/v4/projects/:project_id/labels", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var label Label
		err := json.NewDecoder(r.Body).Decode(&label)
		panicIf(err)

		label, ok := gitlab.StoreLabelIfNotExists(label)
		if !ok {
			rw.WriteHeader(http.StatusBadRequest)
			err = json.NewEncoder(rw).Encode(ErrorResult{Message: "Label already exists"})
			panicIf(err)
			return
		}
		gitlab.newLabels <- label

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(label)
		panicIf(err)
	})

	// Issues

	router.POST("/api/v4/projects/:project_id/issues", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID := parseIntID(ps.ByName("project_id"))

		var params IssueParams
		err := json.NewDecoder(r.Body).Decode(&params)
		panicIf(err)
		issue := gitlab.StoreIssue(Issue{
			ProjectID:   projectID,
			Title:       params.Title,
			Description: params.Description,
			State:       "opened",
			Labels:      gitlab.GetLabelTitles(strings.Split(params.Labels, ",")...),
		})
		gitlab.newIssues <- issue

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(issue)
		panicIf(err)
	})
	router.PUT("/api/v4/projects/:project_id/issues/:issue_iid", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID := parseIntID(ps.ByName("project_id"))
		issueIID := parseIntID(ps.ByName("issue_iid"))

		issue, found := gitlab.GetProjectIssue(projectID, issueIID)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{Message: "Issue not found"})
			panicIf(err)
			return
		}

		var params IssueParams
		err := json.NewDecoder(r.Body).Decode(&params)
		panicIf(err)

		issue.Title = params.Title

		labels := stringset.New(issue.Labels...)
		for _, label := range gitlab.GetLabelTitles(strings.Split(params.AddLabels, ",")...) {
			labels.Add(label)
		}
		for _, label := range gitlab.GetLabelTitles(strings.Split(params.RemoveLabels, ",")...) {
			labels.Del(label)
		}
		issue.Labels = labels.ToSlice()

		switch params.StateEvent {
		case "close":
			issue.State = "closed"
		default:
			log.Panicf("unknown StateEvent=%q", params.StateEvent)
		}

		gitlab.StoreIssue(issue)
		gitlab.issueUpdates <- issue

		rw.Header().Add("Content-Type", "application/json")
		err = json.NewEncoder(rw).Encode(issue)
		panicIf(err)
	})
	router.GET("/api/v4/projects/:project_id/issues/:issue_iid", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID := parseIntID(ps.ByName("project_id"))
		issueIID := parseIntID(ps.ByName("issue_iid"))

		issue, found := gitlab.GetProjectIssue(projectID, issueIID)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{Message: "Issue not found"})
			panicIf(err)
			return
		}

		err := json.NewEncoder(rw).Encode(issue)
		panicIf(err)
	})

	// Issue notes

	router.POST("/api/v4/projects/:project_id/issues/:issue_iid/notes", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		projectID := parseIntID(ps.ByName("project_id"))
		issueIID := parseIntID(ps.ByName("issue_iid"))

		issue, found := gitlab.GetProjectIssue(projectID, issueIID)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{Message: "Issue not found"})
			panicIf(err)
			return
		}

		var params NoteParams
		err := json.NewDecoder(r.Body).Decode(&params)
		panicIf(err)
		note := gitlab.StoreNote(Note{
			NoteableType: "Issue",
			NoteableID:   issue.ID,
			Body:         params.Body,
			Confidential: params.Confidential,
		})
		gitlab.newNotes <- note

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(note)
		panicIf(err)
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
	if obj, ok := s.objects.Load(fmt.Sprintf("issue-%d", id)); ok {
		return obj.(Issue), true
	}
	return Issue{}, false
}

func (s *FakeGitlab) GetProjectIssue(projectID, issueIID IntID) (Issue, bool) {
	if obj, ok := s.objects.Load(fakeProjectIssueKey{ProjectID: projectID, IssueIID: issueIID}); ok {
		return obj.(Issue), true
	}
	return Issue{}, false
}

func (s *FakeGitlab) StoreIssue(issue Issue) Issue {
	if issue.IID == 0 {
		var newID uint64
		val, _ := s.issueIDCounters.LoadOrStore(issue.ProjectID, &newID)
		idPtr := val.(*uint64)
		issue.IID = IntID(atomic.AddUint64(idPtr, 1))
		issue.ID = issue.ProjectID*1000000 + issue.IID
	}
	s.objects.Store(fmt.Sprintf("issue-%d", issue.ID), issue)
	s.objects.Store(fakeProjectIssueKey{ProjectID: issue.ProjectID, IssueIID: issue.IID}, issue)
	return issue
}

func (s *FakeGitlab) StoreNote(note Note) Note {
	if note.ID == 0 {
		note.ID = IntID(atomic.AddUint64(&s.noteIDCounter, 1))
	}
	s.objects.Store(fmt.Sprintf("note-%d", note.ID), note)
	return note
}

func (s *FakeGitlab) GetLabelByName(name string) (Label, bool) {
	if obj, ok := s.objects.Load(fakeLabelByName(name)); ok {
		return obj.(Label), true
	}
	return Label{}, false
}

func (s *FakeGitlab) GetLabels(names ...string) (labels []Label) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if label, exists := s.GetLabelByName(name); exists {
			labels = append(labels, label)
		}
	}
	return
}

func (s *FakeGitlab) GetLabelTitles(names ...string) (titles []string) {
	for _, label := range s.GetLabels(names...) {
		titles = append(titles, label.Title)
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
		label.ID = IntID(atomic.AddUint64(&s.labelIDCounter, 1))
	}
	s.objects.Store(fmt.Sprintf("label-%d", label.ID), label)
	s.objects.Store(fakeLabelByName(label.Name), label)
	return label
}

func (s *FakeGitlab) StoreLabelIfNotExists(label Label) (Label, bool) {
	var name string
	if label.Name != "" {
		name = label.Name
	} else {
		name = label.Title
	}
	if existingLabel, exists := s.GetLabelByName(name); exists {
		return existingLabel, false
	}

	return s.StoreLabel(label), true
}

func (s *FakeGitlab) GetProjectHook(projectID, hookIID IntID) (ProjectHook, bool) {
	if obj, ok := s.objects.Load(fakeProjectHookKey{ProjectID: projectID, HookIID: hookIID}); ok {
		return obj.(ProjectHook), true
	}
	return ProjectHook{}, false
}

func (s *FakeGitlab) StoreProjectHook(hook ProjectHook) ProjectHook {
	if hook.ID == 0 {
		hook.ID = IntID(atomic.AddUint64(&s.projectHookIDCounter, 1))
	}
	s.objects.Store(fakeProjectHookKey{ProjectID: hook.ProjectID, HookIID: hook.ID}, hook)
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

func (s *FakeGitlab) CheckNewNote(ctx context.Context) (Note, error) {
	select {
	case note := <-s.newNotes:
		return note, nil
	case <-ctx.Done():
		return Note{}, trace.Wrap(ctx.Err())
	}
}

func parseIntID(str string) IntID {
	val, err := strconv.ParseUint(str, 10, 64)
	panicIf(err)
	return IntID(val)
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("%v at %v", err, string(debug.Stack()))
	}
}
