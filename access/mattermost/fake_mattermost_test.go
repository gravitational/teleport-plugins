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

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	mm "github.com/mattermost/mattermost-server/model"

	log "github.com/sirupsen/logrus"
)

type FakeMattermost struct {
	srv         *httptest.Server
	posts       sync.Map
	newPosts    chan mm.Post
	postUpdates chan mm.Post
}

func NewFakeMattermost() *FakeMattermost {
	router := httprouter.New()

	mattermost := &FakeMattermost{
		newPosts:    make(chan mm.Post, 20),
		postUpdates: make(chan mm.Post, 20),
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

		post := mm.Post{}
		err := json.NewDecoder(r.Body).Decode(&post)
		fatalIf(err)

		if post.ChannelId != "2222" {
			http.Error(rw, `{}`, http.StatusNotFound)
			return
		}

		post.Id = fmt.Sprintf("%v", time.Now().UnixNano())
		mattermost.posts.Store(post.Id, post)
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

		var newPost mm.Post
		err := json.NewDecoder(r.Body).Decode(&newPost)
		fatalIf(err)

		post.Message = newPost.Message
		post.Props = newPost.Props
		mattermost.posts.Store(post.Id, post)
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

func (s *FakeMattermost) GetPost(id string) (mm.Post, bool) {
	if obj, ok := s.posts.Load(id); ok {
		return obj.(mm.Post), true
	}
	return mm.Post{}, false
}

func (s *FakeMattermost) CheckNewPost(ctx context.Context, timeout time.Duration) (mm.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case post := <-s.newPosts:
		return post, nil
	case <-ctx.Done():
		return mm.Post{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeMattermost) CheckPostUpdate(ctx context.Context, timeout time.Duration) (mm.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case post := <-s.postUpdates:
		return post, nil
	case <-ctx.Done():
		return mm.Post{}, trace.Wrap(ctx.Err())
	}
}

func fatalIf(err error) {
	if err != nil {
		log.Fatalf("%v at %v", err, string(debug.Stack()))
	}
}
