package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

// Callback struct represents a full callback request with it's payload
// separated into a smaller wrapper struct.
type Callback struct {
	Payload CallbackPayload `json:"payload"`
}

// CallbackPayload is the relevant part of the callback request —
// with info about which access request to update, and what status to set.
type CallbackPayload struct {
	ReqID     string `json:"ReqID"`
	State     string `json:"State"`
	Delegator string `json:"Delegator"`
}

// CallbackFunc type represents a callback handler that takes a context and
// a callback in, handles it, and optionally returns an error.
type CallbackFunc func(ctx context.Context, callback Callback) error

// CallbackServer is a wrapper around http.Server that processes Slack interaction events.
// It verifies incoming requests and calls onCallback for valid ones
type CallbackServer struct {
	http       *utils.HTTP
	onCallback CallbackFunc
	counter    uint64
}

// NewCallbackServer initializes and returns an HTTP server that handles Slack callback (webhook) requests.
func NewCallbackServer(conf utils.HTTPConfig, onCallback CallbackFunc) (*CallbackServer, error) {
	httpSrv, err := utils.NewHTTP(conf)
	if err != nil {
		return nil, err
	}
	srv := &CallbackServer{
		http:       httpSrv,
		onCallback: onCallback,
	}
	httpSrv.POST("/", srv.handleCallback)
	return srv, nil
}

// ServiceJob returns a service job object from the Callback HTTP server.
func (s *CallbackServer) ServiceJob() utils.ServiceJob {
	return s.http.ServiceJob()
}

// BaseURL returns Webhook (callback) HTTP server base URL.
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

func (s *CallbackServer) handleCallback(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond*2500)
	defer cancel()

	decoder := json.NewDecoder(r.Body)

	var callback Callback
	err := decoder.Decode(&callback)

	if err != nil {
		message := fmt.Sprintf("Failed to parse json payload field: %v", r.FormValue("payload"))
		log.WithError(err).Error(message)
		http.Error(rw, message, http.StatusBadRequest)
		return
	}

	// Invoke app-level callback handler.
	// If it errored out, send HTTP error and log it.
	// If it succeeded, send HTTP OK.
	if err := s.onCallback(ctx, callback); err != nil {
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

// func (s *CallbackServer) validateSignature(reqSignature, payload, secret string) error {
// 	h := hmac.New(sha256.New, []byte(secret))
// 	h.Write([]byte(payload))
// 	sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

// 	if sign != reqSignature {
// 		return trace.Errorf("Request signature verification failed")
// 	}
// 	return nil
// }
