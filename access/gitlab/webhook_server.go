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
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

const (
	gitlabWebhookPath = "/webhook"
)

type WebhookServer struct {
	http      *lib.HTTP
	onWebhook WebhookFunc
	secret    string
	counter   uint64
}

func NewWebhookServer(conf lib.HTTPConfig, secret string, onWebhook WebhookFunc) (*WebhookServer, error) {
	httpSrv, err := lib.NewHTTP(conf)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	srv := &WebhookServer{
		http:      httpSrv,
		onWebhook: onWebhook,
		secret:    secret,
	}
	srv.http.POST(gitlabWebhookPath, srv.processWebhook)
	return srv, nil
}

func (s *WebhookServer) ServiceJob() lib.ServiceJob {
	return s.http.ServiceJob()
}

func (s *WebhookServer) WebhookURL() string {
	return s.http.NewURL(gitlabWebhookPath, nil).String()
}

func (s *WebhookServer) BaseURL() *url.URL {
	return s.http.BaseURL()
}

func (s *WebhookServer) EnsureCert() error {
	return s.http.EnsureCert(DefaultDir + "/server")
}

func (s *WebhookServer) processWebhook(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// TODO: figure out timeout
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*5)
	defer cancel()

	httpRequestID := fmt.Sprintf("%v-%v", time.Now().Unix(), atomic.AddUint64(&s.counter, 1))
	ctx, log := logger.WithField(ctx, "gitlab_http_id", httpRequestID)

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		log.Errorf(`Invalid "Content-Type" header %q`, contentType)
		http.Error(rw, "", http.StatusBadRequest)
		return
	}
	// the length of the secret token is not particularly confidential, so it's ok to leak it here
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Gitlab-Token")), []byte(s.secret)) == 0 {
		log.Error(`Invalid webhook secret provided`)
		http.Error(rw, "", http.StatusUnauthorized)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("Failed to read webhook payload")
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}

	var event interface{}
	switch eventType := r.Header.Get("X-Gitlab-Event"); eventType {
	case "Issue Hook":
		var issueEvent IssueEvent
		if err = json.Unmarshal(body, &issueEvent); err != nil {
			log.WithError(err).Error("Failed to parse webhook payload")
			http.Error(rw, "", http.StatusBadRequest)
			return
		}
		event = issueEvent
	default:
		log.Warningf(`Received unsupported hook %q`, eventType)
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	if err := s.onWebhook(ctx, Webhook{Event: event}); err != nil {
		log.WithError(err).Error("Failed to process webhook")
		log.Debugf("%v", trace.DebugReport(err))
		var code int
		switch {
		case lib.IsCanceled(err) || lib.IsDeadline(err):
			code = http.StatusServiceUnavailable
		default:
			code = http.StatusInternalServerError
		}
		http.Error(rw, "", code)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
