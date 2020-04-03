package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
)

type WebhookPayload struct {
	Messages []WebhookMessage `json:"messages"`
}

type WebhookMessage struct {
	ID       string       `json:"id"`
	Event    string       `json:"event"`
	Incident *pd.Incident `json:"incident"`
}

type WebhookAction struct {
	HttpID      string
	MessageID   string
	Event       string
	Name        string
	IncidentID  string
	IncidentKey string
}

type WebhookFunc func(ctx context.Context, action WebhookAction) error

type WebhookServer struct {
	http     *utils.HTTP
	onAction WebhookFunc
	counter  uint64
}

func NewWebhookServer(conf utils.HTTPConfig, onAction WebhookFunc) *WebhookServer {
	srv := &WebhookServer{
		http:     utils.NewHTTP(conf),
		onAction: onAction,
	}
	srv.http.POST("/"+pdApproveAction, func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		srv.processWebhook(pdApproveAction, rw, r)
	})
	srv.http.POST("/"+pdDenyAction, func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		srv.processWebhook(pdDenyAction, rw, r)
	})
	return srv
}

func (s *WebhookServer) ActionURL(actionName string) (string, error) {
	url, err := s.http.NewURL(actionName, nil)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return url.String(), nil
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

func (s *WebhookServer) processWebhook(actionName string, rw http.ResponseWriter, r *http.Request) {
	// Custom incident actions are required to respond within 16 seconds.
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*16-pdHttpTimeout)
	defer cancel()

	webhookID := r.Header.Get("X-Webhook-Id")
	httpRequestID := fmt.Sprintf("%v-%v", webhookID, atomic.AddUint64(&s.counter, 1))
	log := log.WithField("pd_http_id", httpRequestID)

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		log.Errorf(`Invalid "Content-Type" header %q`, contentType)
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	var payload WebhookPayload

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("Failed to read webhook payload")
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		log.WithError(err).Error("Failed to parse webhook payload")
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	for _, msg := range payload.Messages {
		log = log.WithField("pd_msg_id", msg.ID)

		action := WebhookAction{
			HttpID:      httpRequestID,
			MessageID:   msg.ID,
			Event:       msg.Event,
			Name:        actionName,
			IncidentID:  msg.Incident.Id,
			IncidentKey: msg.Incident.IncidentKey,
		}
		if err := s.onAction(ctx, action); err != nil {
			log.WithError(err).Error("Failed to process webhook")
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
	}

	rw.WriteHeader(http.StatusNoContent)
}
