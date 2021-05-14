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

	log "github.com/sirupsen/logrus"
)

type FakeSlack struct {
	srv *httptest.Server

	botUser                    User
	objects                    sync.Map
	newMessages                chan Msg
	messageUpdatesByAPI        chan Msg
	messageUpdatesByResponding chan Msg
	messageCounter             uint64
	userIDCounter              uint64
	startTime                  time.Time
}

func NewFakeSlack(botUser User, concurrency int) *FakeSlack {
	router := httprouter.New()

	s := &FakeSlack{
		newMessages:                make(chan Msg, concurrency*6),
		messageUpdatesByAPI:        make(chan Msg, concurrency*2),
		messageUpdatesByResponding: make(chan Msg, concurrency),
		startTime:                  time.Now(),
		srv:                        httptest.NewServer(router),
	}

	s.botUser = s.StoreUser(botUser)

	router.POST("/auth.test", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(Response{Ok: true})
		panicIf(err)
	})

	router.POST("/chat.postMessage", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var payload Msg
		err := json.NewDecoder(r.Body).Decode(&payload)
		panicIf(err)

		msg := s.StoreMessage(Msg{
			Type:       "message",
			Channel:    payload.Channel,
			ThreadTs:   payload.ThreadTs,
			Text:       payload.Text,
			BlockItems: payload.BlockItems,
			User:       s.botUser.ID,
			Username:   s.botUser.Name,
		})
		s.newMessages <- msg

		response := ChatMsgResponse{
			Response:  Response{Ok: true},
			Channel:   msg.Channel,
			Timestamp: msg.Timestamp,
			Text:      msg.Text,
		}
		err = json.NewEncoder(rw).Encode(response)
		panicIf(err)
	})

	router.POST("/chat.update", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var payload Msg
		err := json.NewDecoder(r.Body).Decode(&payload)
		panicIf(err)

		msg, found := s.GetMessage(payload.Timestamp)
		if !found {
			err := json.NewEncoder(rw).Encode(Response{Ok: false, Error: "message_not_found"})
			panicIf(err)
			return
		}

		msg.Text = payload.Text
		msg.BlockItems = payload.BlockItems

		s.messageUpdatesByAPI <- s.StoreMessage(msg)

		response := ChatMsgResponse{
			Response:  Response{Ok: true},
			Channel:   msg.Channel,
			Timestamp: msg.Timestamp,
			Text:      msg.Text,
		}
		err = json.NewEncoder(rw).Encode(&response)
		panicIf(err)
	})

	router.POST("/_response/:ts", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var payload struct {
			Msg
			ReplaceOriginal bool `json:"replace_original"`
		}
		err := json.NewDecoder(r.Body).Decode(&payload)
		panicIf(err)

		timestamp := ps.ByName("ts")
		msg, found := s.GetMessage(timestamp)
		if !found {
			err := json.NewEncoder(rw).Encode(Response{Ok: false, Error: "message_not_found"})
			panicIf(err)
			return
		}

		if payload.ReplaceOriginal {
			msg.BlockItems = payload.BlockItems
			s.messageUpdatesByResponding <- s.StoreMessage(msg)
		} else {
			newMsg := s.StoreMessage(Msg{
				Type:       "message",
				Channel:    msg.Channel,
				BlockItems: payload.BlockItems,
				User:       s.botUser.ID,
				Username:   s.botUser.Name,
			})
			s.newMessages <- newMsg
		}
		err = json.NewEncoder(rw).Encode(Response{Ok: true})
		panicIf(err)
	})

	router.GET("/users.info", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		id := r.URL.Query().Get("user")
		if id == "" {
			err := json.NewEncoder(rw).Encode(Response{Ok: false, Error: "invalid_arguments"})
			panicIf(err)
			return
		}

		user, found := s.GetUser(id)
		if !found {
			err := json.NewEncoder(rw).Encode(Response{Ok: false, Error: "user_not_found"})
			panicIf(err)
			return
		}

		err := json.NewEncoder(rw).Encode(struct {
			User User `json:"user"`
			Ok   bool `json:"ok"`
		}{user, true})
		panicIf(err)
	})

	router.GET("/users.lookupByEmail", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		email := r.URL.Query().Get("email")
		if email == "" {
			err := json.NewEncoder(rw).Encode(Response{Ok: false, Error: "invalid_arguments"})
			panicIf(err)
			return
		}

		user, found := s.GetUserByEmail(email)
		if !found {
			err := json.NewEncoder(rw).Encode(Response{Ok: false, Error: "users_not_found"})
			panicIf(err)
			return
		}

		err := json.NewEncoder(rw).Encode(struct {
			User User `json:"user"`
			Ok   bool `json:"ok"`
		}{user, true})
		panicIf(err)
	})

	return s
}

func (s *FakeSlack) URL() string {
	return s.srv.URL
}

func (s *FakeSlack) Close() {
	s.srv.Close()
	close(s.newMessages)
	close(s.messageUpdatesByAPI)
	close(s.messageUpdatesByResponding)
}

func (s *FakeSlack) StoreMessage(msg Msg) Msg {
	if msg.Timestamp == "" {
		now := s.startTime.Add(time.Since(s.startTime)) // get monotonic timestamp
		uniq := atomic.AddUint64(&s.messageCounter, 1)  // generate uniq int to prevent races
		msg.Timestamp = fmt.Sprintf("%d.%d", now.UnixNano(), uniq)
	}
	s.objects.Store(fmt.Sprintf("msg-%s", msg.Timestamp), msg)
	return msg
}

func (s *FakeSlack) GetMessage(id string) (Msg, bool) {
	if obj, ok := s.objects.Load(fmt.Sprintf("msg-%s", id)); ok {
		msg, ok := obj.(Msg)
		return msg, ok
	}
	return Msg{}, false
}

func (s *FakeSlack) StoreUser(user User) User {
	if user.ID == "" {
		user.ID = fmt.Sprintf("U%d", atomic.AddUint64(&s.userIDCounter, 1))
	}
	s.objects.Store(fmt.Sprintf("user-%s", user.ID), user)
	s.objects.Store(fmt.Sprintf("userByEmail-%s", user.Profile.Email), user)
	return user
}

func (s *FakeSlack) GetUser(id string) (User, bool) {
	if obj, ok := s.objects.Load(fmt.Sprintf("user-%s", id)); ok {
		user, ok := obj.(User)
		return user, ok
	}
	return User{}, false
}

func (s *FakeSlack) GetUserByEmail(email string) (User, bool) {
	if obj, ok := s.objects.Load(fmt.Sprintf("userByEmail-%s", email)); ok {
		user, ok := obj.(User)
		return user, ok
	}
	return User{}, false
}

func (s *FakeSlack) CheckNewMessage(ctx context.Context) (Msg, error) {
	select {
	case message := <-s.newMessages:
		return message, nil
	case <-ctx.Done():
		return Msg{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeSlack) CheckMessageUpdateByAPI(ctx context.Context) (Msg, error) {
	select {
	case message := <-s.messageUpdatesByAPI:
		return message, nil
	case <-ctx.Done():
		return Msg{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeSlack) CheckMessageUpdateByResponding(ctx context.Context) (Msg, error) {
	select {
	case message := <-s.messageUpdatesByResponding:
		return message, nil
	case <-ctx.Done():
		return Msg{}, trace.Wrap(ctx.Err())
	}
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("%v at %v", err, string(debug.Stack()))
	}
}
