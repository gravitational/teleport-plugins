/*
Copyright 2015-2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific languap governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	// multipartFormBufSize is a buffer size for ParseMultipartForm
	multipartFormBufSize = 8192
)

// MockMailgunMessage is a mock mailgun message
type MockMailgunMessage struct {
	ID         string
	Sender     string
	Recipient  string
	Subject    string
	Body       string
	References string
}

// mockMailgun is a mock mailgun server
type MockMailgunServer struct {
	server     *httptest.Server
	chMessages chan MockMailgunMessage
}

// NewMockMailgun creates unstarted mock mailgun server instance.
// Standard server from mailgun-go does not catch message texts.
func NewMockMailgunServer(concurrency int) *MockMailgunServer {
	mg := &MockMailgunServer{
		chMessages: make(chan MockMailgunMessage, concurrency*50),
	}

	s := httptest.NewUnstartedServer(func(mg *MockMailgunServer) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(multipartFormBufSize); err != nil {
				log.Error(err)
			}

			id := uuid.New().String()

			message := MockMailgunMessage{
				ID:         id,
				Sender:     r.PostFormValue("from"),
				Recipient:  r.PostFormValue("to"),
				Subject:    r.PostFormValue("subject"),
				Body:       r.PostFormValue("text"),
				References: r.PostFormValue("references"),
			}

			mg.chMessages <- message

			fmt.Fprintf(w, `{"id": "%v"}`, id)
		}
	}(mg))

	mg.server = s

	return mg
}

// Start starts server
func (m *MockMailgunServer) Start() {
	m.server.Start()
}

// GetURL returns server url
func (m *MockMailgunServer) GetURL() string {
	return m.server.URL + "/v4"
}

// GetMessage gets the new Mailgun message from a queue
func (m *MockMailgunServer) GetMessage(ctx context.Context) (MockMailgunMessage, error) {
	select {
	case message := <-m.chMessages:
		return message, nil
	case <-ctx.Done():
		return MockMailgunMessage{}, trace.Wrap(ctx.Err())
	}
}

// Close stops servers
func (m *MockMailgunServer) Stop() {
	m.server.Close()
}
