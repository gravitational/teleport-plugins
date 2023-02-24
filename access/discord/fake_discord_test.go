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
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

type FakeDiscord struct {
	srv *httptest.Server

	objects                    sync.Map
	newMessages                chan DiscordMsg
	messageUpdatesByAPI        chan DiscordMsg
	messageUpdatesByResponding chan DiscordMsg
	messageCounter             uint64
	startTime                  time.Time
}

func NewFakeDiscord(concurrency int) *FakeDiscord {
	router := httprouter.New()

	s := &FakeDiscord{
		newMessages:                make(chan DiscordMsg, concurrency*6),
		messageUpdatesByAPI:        make(chan DiscordMsg, concurrency*2),
		messageUpdatesByResponding: make(chan DiscordMsg, concurrency),
		startTime:                  time.Now(),
		srv:                        httptest.NewServer(router),
	}

	router.GET("/users/@me", func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(rw).Encode(DiscordResponse{Code: http.StatusOK})
		panicIf(err)
	})

	router.POST("/channels/:channelID/messages", func(rw http.ResponseWriter, r *http.Request, params httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var payload DiscordMsg
		err := json.NewDecoder(r.Body).Decode(&payload)
		panicIf(err)

		channel := params.ByName("channelID")

		msg := s.StoreMessage(DiscordMsg{Msg: Msg{
			Channel: channel,
		},
			Text: payload.Text,
		})

		s.newMessages <- msg

		response := ChatMsgResponse{
			DiscordResponse: DiscordResponse{Code: http.StatusOK},
			Channel:         channel,
			Text:            payload.Text,
			DiscordID:       msg.DiscordID,
		}
		err = json.NewEncoder(rw).Encode(response)
		panicIf(err)
	})

	router.PATCH("/channels/:channelID/messages/:messageID", func(rw http.ResponseWriter, r *http.Request, params httprouter.Params) {
		rw.Header().Add("Content-Type", "application/json")

		var payload DiscordMsg
		err := json.NewDecoder(r.Body).Decode(&payload)
		panicIf(err)

		channel := params.ByName("channelID")
		messageID := params.ByName("messageID")

		_, found := s.GetMessage(messageID)
		if !found {
			err := json.NewEncoder(rw).Encode(DiscordResponse{Code: 10008, Message: "Unknown Message"})
			panicIf(err)
			return
		}

		msg := s.StoreMessage(DiscordMsg{Msg: Msg{
			Channel:   channel,
			DiscordID: messageID,
		},
			Text:   payload.Text,
			Embeds: payload.Embeds,
		})

		s.messageUpdatesByAPI <- msg

		response := ChatMsgResponse{
			DiscordResponse: DiscordResponse{Code: http.StatusOK},
			Channel:         channel,
			Text:            payload.Text,
			DiscordID:       msg.DiscordID,
		}
		err = json.NewEncoder(rw).Encode(response)
		panicIf(err)
	})

	return s
}

func (s *FakeDiscord) URL() string {
	return s.srv.URL
}

func (s *FakeDiscord) Close() {
	s.srv.Close()
	close(s.newMessages)
	close(s.messageUpdatesByAPI)
	close(s.messageUpdatesByResponding)
}

func (s *FakeDiscord) StoreMessage(msg DiscordMsg) DiscordMsg {
	if msg.DiscordID == "" {
		msg.DiscordID = strconv.FormatUint(atomic.AddUint64(&s.messageCounter, 1), 10)
	}
	s.objects.Store(fmt.Sprintf("msg-%s", msg.DiscordID), msg)
	return msg
}

func (s *FakeDiscord) GetMessage(id string) (DiscordMsg, bool) {
	if obj, ok := s.objects.Load(fmt.Sprintf("msg-%s", id)); ok {
		msg, ok := obj.(DiscordMsg)
		return msg, ok
	}
	return DiscordMsg{}, false
}

func (s *FakeDiscord) CheckNewMessage(ctx context.Context) (DiscordMsg, error) {
	select {
	case message := <-s.newMessages:
		return message, nil
	case <-ctx.Done():
		return DiscordMsg{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeDiscord) CheckMessageUpdateByAPI(ctx context.Context) (DiscordMsg, error) {
	select {
	case message := <-s.messageUpdatesByAPI:
		return message, nil
	case <-ctx.Done():
		return DiscordMsg{}, trace.Wrap(ctx.Err())
	}
}

func (s *FakeDiscord) CheckMessageUpdateByResponding(ctx context.Context) (DiscordMsg, error) {
	select {
	case message := <-s.messageUpdatesByResponding:
		return message, nil
	case <-ctx.Done():
		return DiscordMsg{}, trace.Wrap(ctx.Err())
	}
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("%v at %v", err, string(debug.Stack()))
	}
}
