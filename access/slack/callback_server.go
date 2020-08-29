package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
)

// Callback struct represents an HTTP request that is a callback from Slack,
// and wraps around slack client's InteractionCallback.
type Callback struct {
	HTTPRequestID string
	slack.InteractionCallback
}

// CallbackFunc type represents a callback handler that takes a context and
// a callback in, handles it, and optionally returns an error.
type CallbackFunc func(ctx context.Context, callback Callback) error

// CallbackServer is a wrapper around http.Server that processes Slack interaction events.
// It verifies incoming requests and calls onCallback for valid ones
type CallbackServer struct {
	http       *utils.HTTP
	secret     string
	onCallback CallbackFunc
	counter    uint64
}

// NewCallbackServer initializes and returns an HTTP server that handles Slack callback (webhook) requests.
func NewCallbackServer(conf utils.HTTPConfig, secret string, onCallback CallbackFunc) (*CallbackServer, error) {
	httpSrv, err := utils.NewHTTP(conf)
	if err != nil {
		return nil, err
	}
	srv := &CallbackServer{
		http:       httpSrv,
		secret:     secret,
		onCallback: onCallback,
	}
	httpSrv.POST("/", srv.processCallback)
	return srv, nil
}

// ServiceJob returns a service job object from the Callback HTTP server.
func (s *CallbackServer) ServiceJob() utils.ServiceJob {
	return s.http.ServiceJob()
}

// BaseURL returns Slack Webhook (callback) HTTP server base URL.
func (s *CallbackServer) BaseURL() *url.URL {
	return s.http.BaseURL()
}

// EnsureCert uses http util's EnsureCert to make sure that TLS certificates
// are there and are accessible and valid.
// If using self-signed certs, this will also generate self-signed TLS certificates if they're missing.
// Please note that self-signed certs would not work by default, since Slack won't respect / validate
// the self-signed certs.
func (s *CallbackServer) EnsureCert() error {
	return s.http.EnsureCert(DefaultDir + "/server")
}

func (s *CallbackServer) processCallback(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond*2500) // Slack requires to respond within 3000 milliseconds
	defer cancel()

	httpRequestID := fmt.Sprintf("%s-%v", r.Header.Get("x-slack-request-timestamp"), atomic.AddUint64(&s.counter, 1))
	log := log.WithField("slack_http_id", httpRequestID)

	sv, err := slack.NewSecretsVerifier(r.Header, s.secret)
	if err != nil {
		log.WithError(err).Error("Failed to initialize secrets verifier")
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}
	// tee body into verifier as it is read.
	r.Body = ioutil.NopCloser(io.TeeReader(r.Body, &sv))
	payload := []byte(r.FormValue("payload"))

	// the FormValue method exhausts the reader, so signature
	// verification can now proceed.
	if err := sv.Ensure(); err != nil {
		log.WithError(err).Error("Secret verification failed")
		http.Error(rw, "", http.StatusUnauthorized)
		return
	}

	var cb slack.InteractionCallback
	if err := json.Unmarshal(payload, &cb); err != nil {
		log.WithError(err).Error("Failed to parse json body")
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	if err := s.onCallback(ctx, Callback{httpRequestID, cb}); err != nil {
		log.WithError(err).Error("Failed to process callback")
		log.Debugf("%v", trace.DebugReport(err))
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
