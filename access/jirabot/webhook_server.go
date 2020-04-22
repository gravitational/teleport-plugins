package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

type WebhookIssue struct {
	ID   string `json:"id"`
	Self string `json:"self"`
	Key  string `json:"key"`
}

type Webhook struct {
	RequestId string

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
	http      *utils.HTTP
	onWebhook WebhookFunc
	counter   uint64
}

func NewWebhookServer(conf *Config, onWebhook WebhookFunc) (*WebhookServer, error) {
	httpSrv, err := utils.NewHTTP(conf.HTTP)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	srv := &WebhookServer{
		http:      httpSrv,
		onWebhook: onWebhook,
	}
	httpSrv.POST("/", srv.processWebhook)
	return srv, nil
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

func (s *WebhookServer) processWebhook(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond*2500)
	defer cancel()

	requestId := fmt.Sprintf("%v-%v", time.Now().Unix(), atomic.AddUint64(&s.counter, 1))
	log := log.WithField("jira_http_id", requestId)

	webhook := Webhook{RequestId: requestId}

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
		var code int
		switch {
		case utils.IsCanceled(err) || utils.IsDeadline(err):
			code = http.StatusServiceUnavailable
		default:
			code = http.StatusInternalServerError
		}
		http.Error(rw, "", code)
	} else {
		rw.WriteHeader(http.StatusOK)
	}
}
