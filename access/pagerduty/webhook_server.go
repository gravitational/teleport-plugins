package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
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
	ID         string            `json:"id"`
	Event      string            `json:"event"`
	Incident   *pd.Incident      `json:"incident"`
	LogEntries []WebhookLogEntry `json:"log_entries"`
}

type WebhookLogEntry struct {
	ID    string       `json:"id"`
	Type  string       `json:"type"`
	Agent pd.APIObject `json:"agent"`
}

type WebhookAction struct {
	HTTPRequestID string

	Agent       pd.APIObject
	MessageID   string
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

func NewWebhookServer(conf utils.HTTPConfig, onAction WebhookFunc) (*WebhookServer, error) {
	conf.TLS.VerifyClientCertificateFunc = func(chains [][]*x509.Certificate) error {
		cert := chains[0][0]
		if subj := cert.Subject.String(); subj != "CN=webhooks.pagerduty.com,O=PagerDuty Inc,L=San Francisco,ST=California,C=US" {
			return trace.Errorf("wrong certificate subject: %q", subj)
		}
		return nil
	}

	httpSrv, err := utils.NewHTTP(conf)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	srv := &WebhookServer{
		http:     httpSrv,
		onAction: onAction,
	}
	httpSrv.POST("/"+pdApproveAction, func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		srv.processWebhook(pdApproveAction, rw, r)
	})
	httpSrv.POST("/"+pdDenyAction, func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		srv.processWebhook(pdDenyAction, rw, r)
	})
	return srv, nil
}

func (s *WebhookServer) ServiceJob() utils.ServiceJob {
	return s.http.ServiceJob()
}

func (s *WebhookServer) ActionURL(actionName string) string {
	return s.http.NewURL(actionName, nil).String()
}

func (s *WebhookServer) BaseURL() *url.URL {
	return s.http.BaseURL()
}

func (s *WebhookServer) EnsureCert() error {
	return s.http.EnsureCert(DefaultDir + "/server")
}

func (s *WebhookServer) processWebhook(actionName string, rw http.ResponseWriter, r *http.Request) {
	// Custom incident actions are required to respond within 16 seconds.
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*16-pdHTTPTimeout)
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

		if msg.Event != "incident.custom" {
			log.Warningf("Got %q event, ignoring", msg.Event)
			continue
		}

		var agent pd.APIObject
		for _, logEntry := range msg.LogEntries {
			if logEntry.Type == "custom_log_entry" {
				agent = logEntry.Agent
				break
			}
		}

		action := WebhookAction{
			HTTPRequestID: httpRequestID,
			MessageID:     msg.ID,
			Agent:         agent,
			Name:          actionName,
			IncidentID:    msg.Incident.Id,
			IncidentKey:   msg.Incident.IncidentKey,
		}
		if err := s.onAction(ctx, action); err != nil {
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
	}

	rw.WriteHeader(http.StatusNoContent)
}
