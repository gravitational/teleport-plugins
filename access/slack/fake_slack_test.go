package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	"github.com/nlopes/slack"

	log "github.com/sirupsen/logrus"
)

type FakeSlack struct {
	srv *httptest.Server

	botUser                    slack.User
	objects                    sync.Map
	newMessages                chan slack.Msg
	messageUpdatesByAPI        chan slack.Msg
	messageUpdatesByResponding chan slack.Msg
	messageCounter             uint64
	userIDCounter              uint64
	startTime                  time.Time
}

type chatMessageResponse struct {
	Channel string `json:"channel"`
	TS      string `json:"ts"`
	Text    string `json:"text"`
	Ok      bool   `json:"ok"`
}

func NewFakeSlack(botUser slack.User, concurrency int) *FakeSlack {
	router := httprouter.New()

	s := &FakeSlack{
		newMessages:                make(chan slack.Msg, concurrency),
		messageUpdatesByAPI:        make(chan slack.Msg, concurrency),
		messageUpdatesByResponding: make(chan slack.Msg, concurrency),
		startTime:                  time.Now(),
		srv:                        httptest.NewServer(router),
	}

	s.botUser = s.StoreUser(botUser)

	router.POST("/chat.postMessage", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		data, err := ioutil.ReadAll(r.Body)
		panicIf(err)
		values, err := url.ParseQuery(string(data))
		panicIf(err)

		var msgBlocks slack.Blocks
		if blocksRaw := values.Get("blocks"); blocksRaw != "" {
			unescaped, err := url.QueryUnescape(blocksRaw)
			panicIf(err)
			err = json.Unmarshal([]byte(unescaped), &msgBlocks)
			panicIf(err)
		}

		msg := s.StoreMessage(slack.Msg{
			Type:     "message",
			Channel:  values.Get("channel"),
			Blocks:   msgBlocks,
			User:     s.botUser.ID,
			Username: s.botUser.Name,
		})
		s.newMessages <- msg

		response := chatMessageResponse{
			Channel: msg.Channel,
			TS:      msg.Timestamp,
			Text:    msg.Text,
			Ok:      true,
		}
		err = json.NewEncoder(rw).Encode(&response)
		panicIf(err)
	})

	router.POST("/chat.update", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		data, err := ioutil.ReadAll(r.Body)
		panicIf(err)
		values, err := url.ParseQuery(string(data))
		panicIf(err)

		timestamp := values.Get("ts")
		msg, found := s.GetMessage(timestamp)
		if !found {
			http.Error(rw, "", http.StatusNotFound)
			return
		}

		if blocksRaw := values.Get("blocks"); blocksRaw != "" {
			unescaped, err := url.QueryUnescape(blocksRaw)
			panicIf(err)
			err = json.Unmarshal([]byte(unescaped), &msg.Blocks)
			panicIf(err)
		}

		s.messageUpdatesByAPI <- s.StoreMessage(msg)

		response := chatMessageResponse{
			Channel: msg.Channel,
			TS:      msg.Timestamp,
			Text:    msg.Text,
			Ok:      true,
		}
		err = json.NewEncoder(rw).Encode(&response)
		panicIf(err)
	})

	router.POST("/_response/:ts", func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		var payload slack.Msg
		err := json.NewDecoder(r.Body).Decode(&payload)
		panicIf(err)

		timestamp := ps.ByName("ts")
		msg, found := s.GetMessage(timestamp)
		if !found {
			_, err = rw.Write([]byte(`{"ok":false}`))
			panicIf(err)
			return
		}

		if payload.ReplaceOriginal {
			msg.Blocks = payload.Blocks
			s.messageUpdatesByResponding <- s.StoreMessage(msg)
		} else {
			newMsg := s.StoreMessage(slack.Msg{
				Type:     "message",
				Channel:  msg.Channel,
				Blocks:   payload.Blocks,
				User:     s.botUser.ID,
				Username: s.botUser.Name,
			})
			s.newMessages <- newMsg
		}
		_, err = rw.Write([]byte(`{"ok":true}`))
		panicIf(err)
	})

	router.POST("/users.info", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		data, err := ioutil.ReadAll(r.Body)
		panicIf(err)
		values, err := url.ParseQuery(string(data))
		panicIf(err)

		user, found := s.GetUser(values.Get("user"))
		if !found {
			http.Error(rw, "", http.StatusNotFound)
			return
		}

		err = json.NewEncoder(rw).Encode(&struct {
			User slack.User `json:"user"`
		}{user})
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

func (s *FakeSlack) StoreMessage(msg slack.Msg) slack.Msg {
	if msg.Timestamp == "" {
		now := s.startTime.Add(time.Now().Sub(s.startTime)) // get monotonic timestamp
		uniq := atomic.AddUint64(&s.messageCounter, 1)      // generate uniq int to prevent races
		msg.Timestamp = fmt.Sprintf("%d.%d", now.UnixNano(), uniq)
	}
	s.objects.Store(fmt.Sprintf("msg-%s", msg.Timestamp), msg)
	return msg
}

func (s *FakeSlack) GetMessage(id string) (slack.Msg, bool) {
	if obj, ok := s.objects.Load(fmt.Sprintf("msg-%s", id)); ok {
		msg, ok := obj.(slack.Msg)
		return msg, ok
	}
	return slack.Msg{}, false
}

func (s *FakeSlack) StoreUser(user slack.User) slack.User {
	if user.ID == "" {
		user.ID = fmt.Sprintf("U%d", atomic.AddUint64(&s.userIDCounter, 1))
	}
	s.objects.Store(fmt.Sprintf("user-%s", user.ID), user)
	return user
}

func (s *FakeSlack) GetUser(id string) (slack.User, bool) {
	if obj, ok := s.objects.Load(fmt.Sprintf("user-%s", id)); ok {
		user, ok := obj.(slack.User)
		return user, ok
	}
	return slack.User{}, false
}

func (s *FakeSlack) CheckNewMessage(ctx context.Context) (slack.Msg, error) {
	select {
	case message := <-s.newMessages:
		return message, nil
	case <-ctx.Done():
		return slack.Msg{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeSlack) CheckMessageUpdateByAPI(ctx context.Context) (slack.Msg, error) {
	select {
	case message := <-s.messageUpdatesByAPI:
		return message, nil
	case <-ctx.Done():
		return slack.Msg{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeSlack) CheckMessageUpdateByResponding(ctx context.Context) (slack.Msg, error) {
	select {
	case message := <-s.messageUpdatesByResponding:
		return message, nil
	case <-ctx.Done():
		return slack.Msg{}, trace.Wrap(ctx.Err())
	}
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("%v at %v", err, string(debug.Stack()))
	}
}
