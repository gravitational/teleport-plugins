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
	"time"

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	mm "github.com/mattermost/mattermost-server/v5/model"

	log "github.com/sirupsen/logrus"
)

type FakeMattermost struct {
	srv         *httptest.Server
	objects     sync.Map
	newPosts    chan *mm.Post
	postUpdates chan *mm.Post

	postIDCounter uint64
	userIDCounter uint64
}

func NewFakeMattermost() *FakeMattermost {
	router := httprouter.New()

	mattermost := &FakeMattermost{
		newPosts:    make(chan *mm.Post, 20),
		postUpdates: make(chan *mm.Post, 20),
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
		fatalIf(err)
	})
	router.GET("/api/v4/users/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		user, found := mattermost.GetUser(id)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(&mm.AppError{Message: "User not found"})
			fatalIf(err)
			return
		}
		err := json.NewEncoder(rw).Encode(&user)
		fatalIf(err)
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
		fatalIf(err)
	})
	router.POST("/api/v4/posts", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		post := new(mm.Post)
		err := json.NewDecoder(r.Body).Decode(post)
		fatalIf(err)

		if post.ChannelId != "2222" {
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}

		post = mattermost.StorePost(post)
		mattermost.newPosts <- post

		rw.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(rw).Encode(post)
		fatalIf(err)

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

		newPost := new(mm.Post)
		err := json.NewDecoder(r.Body).Decode(newPost)
		fatalIf(err)

		post.Message = newPost.Message
		post.Props = newPost.Props
		post = mattermost.StorePost(post)
		mattermost.postUpdates <- post

		rw.WriteHeader(http.StatusOK)
		err = json.NewEncoder(rw).Encode(post)
		fatalIf(err)
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

func (s *FakeMattermost) GetPost(id string) (*mm.Post, bool) {
	if obj, ok := s.objects.Load(id); ok {
		post, ok := obj.(*mm.Post)
		return post, ok
	}
	return nil, false
}

func (s *FakeMattermost) StorePost(post *mm.Post) *mm.Post {
	if post.Id == "" {
		post.Id = fmt.Sprintf("post-%v", atomic.AddUint64(&s.postIDCounter, 1))
	}
	s.objects.Store(post.Id, post)
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

func (s *FakeMattermost) CheckNewPost(ctx context.Context, timeout time.Duration) (*mm.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case post := <-s.newPosts:
		return post, nil
	case <-ctx.Done():
		return nil, trace.Wrap(ctx.Err())
	}
}

func (s *FakeMattermost) CheckPostUpdate(ctx context.Context, timeout time.Duration) (*mm.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case post := <-s.postUpdates:
		return post, nil
	case <-ctx.Done():
		return nil, trace.Wrap(ctx.Err())
	}
}

func fatalIf(err error) {
	if err != nil {
		log.Fatalf("%v at %v", err, string(debug.Stack()))
	}
}
