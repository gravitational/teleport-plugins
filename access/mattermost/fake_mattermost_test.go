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
	mm "github.com/mattermost/mattermost-server/v5/model"

	log "github.com/sirupsen/logrus"
)

type FakeMattermost struct {
	srv         *httptest.Server
	objects     sync.Map
	newPosts    chan Post
	postUpdates chan Post

	postIDCounter uint64
	userIDCounter uint64
}

func NewFakeMattermost(concurrency int) *FakeMattermost {
	router := httprouter.New()

	mattermost := &FakeMattermost{
		newPosts:    make(chan Post, concurrency),
		postUpdates: make(chan Post, concurrency),
		srv:         httptest.NewServer(router),
	}

	router.GET("/api/v4/teams/name/test-team", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		team := mm.Team{
			Id:   "1111",
			Name: "test-team",
		}
		err := json.NewEncoder(rw).Encode(&team)
		panicIf(err)
	})
	router.GET("/api/v4/users/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		user, found := mattermost.GetUser(id)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(&mm.AppError{Message: "User not found"})
			panicIf(err)
			return
		}
		err := json.NewEncoder(rw).Encode(&user)
		panicIf(err)
	})
	router.GET("/api/v4/teams/1111/channels/name/test-channel", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		channel := mm.Channel{
			Id:     "2222",
			TeamId: "1111",
			Name:   "test-channel",
		}
		err := json.NewEncoder(rw).Encode(&channel)
		panicIf(err)
	})
	router.POST("/api/v4/posts", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var post Post
		err := json.NewDecoder(r.Body).Decode(&post)
		panicIf(err)

		if post.ChannelID != "2222" {
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}

		post = mattermost.StorePost(post)
		mattermost.newPosts <- post

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(post)
		panicIf(err)

	})
	router.PUT("/api/v4/posts/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		post, found := mattermost.GetPost(id)
		if !found {
			fmt.Printf("Not found %s", id)
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}

		var newPost Post
		err := json.NewDecoder(r.Body).Decode(&newPost)
		panicIf(err)

		post.Message = newPost.Message
		post.Props = newPost.Props
		post = mattermost.UpdatePost(post)

		rw.WriteHeader(http.StatusOK)
		err = json.NewEncoder(rw).Encode(post)
		panicIf(err)
	})

	return mattermost
}

func (s *FakeMattermost) URL() string {
	return s.srv.URL
}

func (s *FakeMattermost) Close() {
	s.srv.Close()
	close(s.newPosts)
	close(s.postUpdates)
}

func (s *FakeMattermost) GetPost(id string) (Post, bool) {
	if obj, ok := s.objects.Load(id); ok {
		post, ok := obj.(Post)
		return post, ok
	}
	return Post{}, false
}

func (s *FakeMattermost) StorePost(post Post) Post {
	if post.ID == "" {
		post.ID = fmt.Sprintf("post-%v", atomic.AddUint64(&s.postIDCounter, 1))
	}
	s.objects.Store(post.ID, post)
	return post
}

func (s *FakeMattermost) UpdatePost(post Post) Post {
	post = s.StorePost(post)
	s.postUpdates <- post
	return post
}

func (s *FakeMattermost) GetUser(id string) (mm.User, bool) {
	if obj, ok := s.objects.Load(id); ok {
		user, ok := obj.(mm.User)
		return user, ok
	}
	return mm.User{}, false
}

func (s *FakeMattermost) StoreUser(user mm.User) mm.User {
	if user.Id == "" {
		user.Id = fmt.Sprintf("user-%v", atomic.AddUint64(&s.userIDCounter, 1))
	}
	s.objects.Store(user.Id, user)
	return user
}

func (s *FakeMattermost) CheckNewPost(ctx context.Context) (Post, error) {
	select {
	case post := <-s.newPosts:
		return post, nil
	case <-ctx.Done():
		return Post{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeMattermost) CheckPostUpdate(ctx context.Context) (Post, error) {
	select {
	case post := <-s.postUpdates:
		return post, nil
	case <-ctx.Done():
		return Post{}, trace.Wrap(ctx.Err())
	}
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("%v at %v", err, string(debug.Stack()))
	}
}
