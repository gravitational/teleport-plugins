package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
)

type WebhookIssue struct {
	ID   string `json:"id"`
	Self string `json:"self"`
	Key  string `json:"key"`
}

type Webhook struct {
	Timestamp          int    `json:"timestamp"`
	WebhookEvent       string `json:"webhookEvent"`
	IssueEventTypeName string `json:"issue_event_type_name"`
	User               *struct {
		Self        string `json:"self"`
		AccountID   string `json:"accountId"`
		AccountType string `json:"accountType"`
		DisplayName string `json:"displayName"`
		Active      bool   `json:"active"`
	} `json:"user"`
	Issue *WebhookIssue `json:"issue"`
}
type WebhookFunc func(ctx context.Context, webhook Webhook) error

// WebhookServer is a wrapper around http.Server that processes JIRA webhook events.
// It verifies incoming requests and calls onWebhook for valid ones
type WebhookServer struct {
	http      *lib.HTTP
	onWebhook WebhookFunc
	counter   uint64
}

func NewWebhookServer(conf lib.HTTPConfig, onWebhook WebhookFunc) (*WebhookServer, error) {
	httpSrv, err := lib.NewHTTP(conf)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	srv := &WebhookServer{
		http:      httpSrv,
		onWebhook: onWebhook,
	}
	httpSrv.POST("/", srv.processWebhook)
	httpSrv.GET("/status", srv.processStatus)
	return srv, nil
}

func (s *WebhookServer) ServiceJob() lib.ServiceJob {
	return s.http.ServiceJob()
}

func (s *WebhookServer) BaseURL() *url.URL {
	return s.http.BaseURL()
}

func (s *WebhookServer) EnsureCert() error {
	return s.http.EnsureCert(DefaultDir + "/server")
}

func (s *WebhookServer) processWebhook(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond*2500)
	defer cancel()

	httpRequestID := fmt.Sprintf("%v-%v", time.Now().Unix(), atomic.AddUint64(&s.counter, 1))
	ctx, log := logger.WithField(ctx, "jira_http_id", httpRequestID)

	var webhook Webhook
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("Failed to read webhook payload")
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}
	if err = json.Unmarshal(body, &webhook); err != nil {
		log.WithError(err).Error("Failed to parse webhook payload")
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	if err = s.onWebhook(ctx, webhook); err != nil {
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
	} else {
		rw.WriteHeader(http.StatusOK)
	}
}

func (s *WebhookServer) processStatus(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	rw.WriteHeader(http.StatusOK)
}
