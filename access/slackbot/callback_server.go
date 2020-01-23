package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
)

const (
	// ActionApprove uniquely identifies the approve button in events.
	ActionApprove = "approve_request"
	// ActionDeny uniquely identifies the deny button in events.
	ActionDeny = "deny_request"
)

type CallbackFunc func(ctx context.Context, reqID, actionID, responseURL string) error

// CallbackServer is a wrapper around http.Server that processes Slack interaction events.
// It verifies incoming requests and calls onCallback for valid ones
type CallbackServer struct {
	secret     string
	onCallback CallbackFunc
	httpServer *http.Server
}

func NewCallbackServer(ctx context.Context, conf *Config, onCallback CallbackFunc) *CallbackServer {
	s := CallbackServer{
		secret:     conf.Slack.Secret,
		onCallback: onCallback,
	}

	s.httpServer = &http.Server{
		Addr: conf.Slack.Listen,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			s.processCallback(ctx, rw, r)
		}),
	}

	go func() {
		<-ctx.Done()
		s.httpServer.Close()
	}()

	return &s
}

func (s *CallbackServer) ListenAndServe() error {
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *CallbackServer) Shutdown(ctx context.Context) error {
	sctx, scancel := context.WithTimeout(ctx, time.Second*20)
	defer scancel()

	return s.httpServer.Shutdown(sctx)
}

func (s *CallbackServer) processCallback(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*2500) // Slack requires to respond within 3000 milliseconds
	defer cancel()

	sv, err := slack.NewSecretsVerifier(r.Header, s.secret)
	if err != nil {
		log.Errorf("Failed to initialize secrets verifier: %s", err)
		http.Error(rw, "verification failed", 500)
		return
	}
	// tee body into verifier as it is read.
	r.Body = ioutil.NopCloser(io.TeeReader(r.Body, &sv))
	payload := []byte(r.FormValue("payload"))

	// the FormValue method exhausts the reader, so signature
	// verification can now proceed.
	if err := sv.Ensure(); err != nil {
		log.Errorf("Secret verification failed: %s", err)
		http.Error(rw, "verification failed", 500)
		return
	}

	var cb slack.InteractionCallback
	if err := json.Unmarshal(payload, &cb); err != nil {
		log.Errorf("Failed to parse json response: %s", err)
		http.Error(rw, "failed to parse response", 500)
		return
	}

	if len(cb.ActionCallback.BlockActions) != 1 {
		log.Warnf("Received more than one Slack action: %+v", cb.ActionCallback.BlockActions)
		http.Error(rw, "expected exactly one block action", 500)
		return
	}

	action := cb.ActionCallback.BlockActions[0]

	if err := s.onCallback(ctx, action.Value, action.ActionID, cb.ResponseURL); err != nil {
		log.Errorf("Failed to process callback: %s", err)
		var code int
		switch {
		case access.IsCanceled(err) || access.IsDeadline(err):
			code = 503
		default:
			code = 500
		}
		http.Error(rw, "failed to process callback", code)
	} else {
		rw.WriteHeader(http.StatusOK)
	}
}
