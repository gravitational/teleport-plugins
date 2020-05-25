package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
)

const (
	gitlabWebhookPath = "/webhook"
)

type WebhookServer struct {
	http      *utils.HTTP
	onWebhook WebhookFunc
	secret    string
	counter   uint64
}

func NewWebhookServer(conf utils.HTTPConfig, secret string, onWebhook WebhookFunc) (*WebhookServer, error) {
	httpSrv, err := utils.NewHTTP(conf)
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

func (s *WebhookServer) WebhookURL() string {
	return s.http.NewURL(gitlabWebhookPath, nil).String()
}

func (s *WebhookServer) Run(ctx context.Context) error {
	if err := s.http.EnsureCert(DefaultDir + "/server"); err != nil {
		return err
	}
	return s.http.ListenAndServe(ctx)
}

func (s *WebhookServer) Shutdown(ctx context.Context) error {
	return s.http.ShutdownWithTimeout(ctx, time.Second*5)
}

func (s *WebhookServer) processWebhook(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// TODO: figure out timeout
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*5)
	defer cancel()

	httpRequestID := fmt.Sprintf("%v-%v", time.Now().Unix(), atomic.AddUint64(&s.counter, 1))
	log := log.WithField("gitlab_http_id", httpRequestID)

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		log.Errorf(`Invalid "Content-Type" header %q`, contentType)
		http.Error(rw, "", http.StatusBadRequest)
		return
	}
	if r.Header.Get("X-Gitlab-Token") != s.secret {
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

	if err := s.onWebhook(ctx, Webhook{HTTPID: httpRequestID, Event: event}); err != nil {
		log.WithError(err).Error("Failed to process webhook")
		log.Debugf("%v", trace.DebugReport(err))
		var code int
		switch {
		case utils.IsCanceled(err) || utils.IsDeadline(err):
			code = http.StatusServiceUnavailable
		default:
			code = http.StatusInternalServerError
		}
		http.Error(rw, "", code)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
