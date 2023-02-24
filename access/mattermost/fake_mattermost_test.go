// Copyright 2023 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

type FakeMattermost struct {
	srv         *httptest.Server
	objects     sync.Map
	botUserID   string
	newPosts    chan Post
	postUpdates chan Post

	postIDCounter    uint64
	userIDCounter    uint64
	teamIDCounter    uint64
	channelIDCounter uint64
}

type fakeUserByEmailKey string
type fakeTeamByNameKey string
type fakeChannelByTeamNameAndNameKey struct {
	team    string
	channel string
}
type fakeDirectChannelUsersKey struct {
	user1ID string
	user2ID string
}
type fakeDirectChannelKey string

type FakeDirectChannel struct {
	User1ID string
	User2ID string
	Channel
}

func NewFakeMattermost(botUser User, concurrency int) *FakeMattermost {
	router := httprouter.New()

	mattermost := &FakeMattermost{
		newPosts:    make(chan Post, concurrency*6),
		postUpdates: make(chan Post, concurrency*2),
		srv:         httptest.NewServer(router),
	}
	mattermost.botUserID = mattermost.StoreUser(botUser).ID

	router.GET("/api/v4/teams/name/:team", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		name := ps.ByName("team")
		team, found := mattermost.GetTeamByName(name)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusNotFound, Message: "Unable to find the team."})
			panicIf(err)
			return
		}
		err := json.NewEncoder(rw).Encode(team)
		panicIf(err)
	})

	router.GET("/api/v4/teams/name/:team/channels/name/:channel", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		teamName := ps.ByName("team")
		name := ps.ByName("channel")
		channel, found := mattermost.GetChannelByTeamNameAndName(teamName, name)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusNotFound, Message: "Unable to find the channel."})
			panicIf(err)
			return
		}
		err := json.NewEncoder(rw).Encode(channel)
		panicIf(err)
	})

	router.POST("/api/v4/channels/direct", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var userIDs []string
		err := json.NewDecoder(r.Body).Decode(&userIDs)
		panicIf(err)
		if len(userIDs) != 2 {
			rw.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusBadRequest, Message: "Expected only two user IDs."})
			panicIf(err)
			return
		}

		user1, found := mattermost.GetUser(userIDs[0])
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusNotFound, Message: "Unable to find the user."})
			panicIf(err)
			return
		}

		user2, found := mattermost.GetUser(userIDs[1])
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusNotFound, Message: "Unable to find the user."})
			panicIf(err)
			return
		}

		err = json.NewEncoder(rw).Encode(mattermost.GetDirectChannelFor(user1, user2).Channel)
		panicIf(err)
	})

	router.GET("/api/v4/users/me", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		err := json.NewEncoder(rw).Encode(mattermost.GetBotUser())
		panicIf(err)
	})

	router.GET("/api/v4/users/email/:email", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		email := ps.ByName("email")
		user, found := mattermost.GetUserByEmail(email)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusNotFound, Message: "Unable to find the user."})
			panicIf(err)
			return
		}
		err := json.NewEncoder(rw).Encode(user)
		panicIf(err)
	})

	router.GET("/api/v4/users/:id", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := ps.ByName("id")
		user, found := mattermost.GetUser(id)
		if !found {
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusNotFound, Message: "Unable to find the user."})
			panicIf(err)
			return
		}
		err := json.NewEncoder(rw).Encode(user)
		panicIf(err)
	})

	router.POST("/api/v4/posts", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var post Post
		err := json.NewDecoder(r.Body).Decode(&post)
		panicIf(err)

		// message size limit as per
		// https://github.com/mattermost/mattermost-server/blob/3d412b14af49701d842e72ef208f0ec0a35ce063/model/post.go#L54
		// (current master at time of writing)
		if len(post.Message) > 4000 {
			rw.WriteHeader(http.StatusBadRequest)
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
			rw.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(rw).Encode(ErrorResult{StatusCode: http.StatusNotFound, Message: "Unable to find the post."})
			panicIf(err)
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

func (s *FakeMattermost) GetBotUser() User {
	user, ok := s.GetUser(s.botUserID)
	if !ok {
		panic("bot user not found")
	}
	return user
}

func (s *FakeMattermost) GetUser(id string) (User, bool) {
	if obj, ok := s.objects.Load(id); ok {
		user, ok := obj.(User)
		return user, ok
	}
	return User{}, false
}

func (s *FakeMattermost) GetUserByEmail(email string) (User, bool) {
	if obj, ok := s.objects.Load(fakeUserByEmailKey(email)); ok {
		user, ok := obj.(User)
		return user, ok
	}
	return User{}, false
}

func (s *FakeMattermost) StoreUser(user User) User {
	if user.ID == "" {
		user.ID = fmt.Sprintf("user-%v", atomic.AddUint64(&s.userIDCounter, 1))
	}
	s.objects.Store(user.ID, user)
	s.objects.Store(fakeUserByEmailKey(user.Email), user)
	return user
}

func (s *FakeMattermost) GetTeam(id string) (Team, bool) {
	if obj, ok := s.objects.Load(id); ok {
		channel, ok := obj.(Team)
		return channel, ok
	}
	return Team{}, false
}

func (s *FakeMattermost) GetTeamByName(name string) (Team, bool) {
	if obj, ok := s.objects.Load(fakeTeamByNameKey(name)); ok {
		channel, ok := obj.(Team)
		return channel, ok
	}
	return Team{}, false
}

func (s *FakeMattermost) StoreTeam(team Team) Team {
	if team.ID == "" {
		team.ID = fmt.Sprintf("team-%v", atomic.AddUint64(&s.teamIDCounter, 1))
	}
	s.objects.Store(team.ID, team)
	s.objects.Store(fakeTeamByNameKey(team.Name), team)
	return team
}

func (s *FakeMattermost) GetChannel(id string) (Channel, bool) {
	if obj, ok := s.objects.Load(id); ok {
		channel, ok := obj.(Channel)
		return channel, ok
	}
	return Channel{}, false
}

func (s *FakeMattermost) GetDirectChannelFor(user1, user2 User) FakeDirectChannel {
	ids := []string{user1.ID, user2.ID}
	sort.Strings(ids)
	user1ID, user2ID := ids[0], ids[1]
	key := fakeDirectChannelUsersKey{user1ID, user2ID}
	if obj, ok := s.objects.Load(key); ok {
		directChannel, ok := obj.(FakeDirectChannel)
		if !ok {
			panic(fmt.Sprintf("bad channel type %T", obj))
		}
		return directChannel
	}

	channel := s.StoreChannel(Channel{})
	directChannel := FakeDirectChannel{
		User1ID: user1ID,
		User2ID: user2ID,
		Channel: channel,
	}
	s.objects.Store(key, directChannel)
	s.objects.Store(fakeDirectChannelKey(channel.ID), directChannel)
	return directChannel
}

func (s *FakeMattermost) GetDirectChannel(id string) (FakeDirectChannel, bool) {
	if obj, ok := s.objects.Load(fakeDirectChannelKey(id)); ok {
		directChannel, ok := obj.(FakeDirectChannel)
		return directChannel, ok
	}
	return FakeDirectChannel{}, false
}

func (s *FakeMattermost) GetChannelByTeamNameAndName(team, name string) (Channel, bool) {
	if obj, ok := s.objects.Load(fakeChannelByTeamNameAndNameKey{team: team, channel: name}); ok {
		channel, ok := obj.(Channel)
		return channel, ok
	}
	return Channel{}, false
}

func (s *FakeMattermost) StoreChannel(channel Channel) Channel {
	if channel.ID == "" {
		channel.ID = fmt.Sprintf("channel-%v", atomic.AddUint64(&s.channelIDCounter, 1))
	}
	s.objects.Store(channel.ID, channel)

	if channel.TeamID != "" {
		team, ok := s.GetTeam(channel.TeamID)
		if !ok {
			panic(fmt.Sprintf("team id %q is not found", channel.TeamID))
		}
		s.objects.Store(fakeChannelByTeamNameAndNameKey{team: team.Name, channel: channel.Name}, channel)
	}
	return channel
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
