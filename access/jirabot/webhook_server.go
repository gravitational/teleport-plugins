package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

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
	Issue *struct {
		ID   string `json:"id"`
		Self string `json:"self"`
		Key  string `json:"key"`
	}
}
type WebhookFunc func(ctx context.Context, webhook Webhook) error

// WebhookServer is a wrapper around http.Server that processes JIRA webhook events.
// It verifies incoming requests and calls onWebhook for valid ones
type WebhookServer struct {
	http      *utils.HTTP
	onWebhook WebhookFunc
}

func NewWebhookServer(conf *Config, onWebhook WebhookFunc) *WebhookServer {
	srv := &WebhookServer{
		utils.NewHTTP(conf.HTTP),
		onWebhook,
	}
	srv.http.POST("/", srv.processWebhook)
	return srv
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

	var webhook Webhook

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("Cannot read webhook payload")
		http.Error(rw, "", 500)
		return
	}
	err = json.Unmarshal(body, &webhook)
	if err != nil {
		log.WithError(err).Error("Invalid webhook payload")
		http.Error(rw, "", 500)
		return
	}

	if err = s.onWebhook(ctx, webhook); err != nil {
		log.WithError(err).Error("Failed to process webhook")
		var code int
		switch {
		case utils.IsCanceled(err) || utils.IsDeadline(err):
			code = 503
		default:
			code = 500
		}
		http.Error(rw, "", code)
	} else {
		rw.WriteHeader(http.StatusOK)
	}
}
