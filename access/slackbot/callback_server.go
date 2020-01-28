package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/utils"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
)

type Callback = slack.InteractionCallback
type CallbackFunc func(ctx context.Context, callback Callback) error

// CallbackServer is a wrapper around http.Server that processes Slack interaction events.
// It verifies incoming requests and calls onCallback for valid ones
type CallbackServer struct {
	http       *utils.HTTP
	secret     string
	onCallback CallbackFunc
}

func NewCallbackServer(conf *Config, onCallback CallbackFunc) *CallbackServer {
	return &CallbackServer{
		utils.NewHTTP(conf.HTTP),
		conf.Slack.Secret,
		onCallback,
	}
}

func (s *CallbackServer) Run(ctx context.Context) error {
	if err := s.http.EnsureCert(DefaultDir + "/server"); err != nil {
		return err
	}
	s.http.Handle(ctx, "/", s.processCallback)
	return s.http.ListenAndServe(ctx)
}

func (s *CallbackServer) Shutdown(ctx context.Context) error {
	// 5 seconds should be enough since every callback is limited to execute within 2500 milliseconds
	return s.http.ShutdownWithTimeout(ctx, time.Second*5)
}

func (s *CallbackServer) processCallback(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*2500) // Slack requires to respond within 3000 milliseconds
	defer cancel()

	if r.Method != "POST" {
		http.Error(rw, "", 400)
		return
	}

	sv, err := slack.NewSecretsVerifier(r.Header, s.secret)
	if err != nil {
		log.WithError(err).Error("Failed to initialize secrets verifier")
		http.Error(rw, "verification failed", 500)
		return
	}
	// tee body into verifier as it is read.
	r.Body = ioutil.NopCloser(io.TeeReader(r.Body, &sv))
	payload := []byte(r.FormValue("payload"))

	// the FormValue method exhausts the reader, so signature
	// verification can now proceed.
	if err := sv.Ensure(); err != nil {
		log.WithError(err).Error("Secret verification failed")
		http.Error(rw, "verification failed", 500)
		return
	}

	var cb slack.InteractionCallback
	if err := json.Unmarshal(payload, &cb); err != nil {
		log.WithError(err).Error("Failed to parse json response")
		http.Error(rw, "failed to parse response", 500)
		return
	}

	if err := s.onCallback(ctx, cb); err != nil {
		log.WithError(err).Error("Failed to process callback")
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
