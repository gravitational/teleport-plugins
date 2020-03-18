package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/julienschmidt/httprouter"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
)

type Callback struct {
	RequestId string
	slack.InteractionCallback
}
type CallbackFunc func(ctx context.Context, callback Callback) error

// CallbackServer is a wrapper around http.Server that processes Slack interaction events.
// It verifies incoming requests and calls onCallback for valid ones
type CallbackServer struct {
	http       *utils.HTTP
	secret     string
	onCallback CallbackFunc
	counter    uint64
}

func NewCallbackServer(conf *Config, onCallback CallbackFunc) *CallbackServer {
	srv := &CallbackServer{
		http:       utils.NewHTTP(conf.HTTP),
		secret:     conf.Slack.Secret,
		onCallback: onCallback,
	}
	srv.http.POST("/", srv.processCallback)
	return srv
}

func (s *CallbackServer) Run(ctx context.Context) error {
	if err := s.http.EnsureCert(DefaultDir + "/server"); err != nil {
		return err
	}
	return s.http.ListenAndServe(ctx)
}

func (s *CallbackServer) Shutdown(ctx context.Context) error {
	// 5 seconds should be enough since every callback is limited to execute within 2500 milliseconds
	return s.http.ShutdownWithTimeout(ctx, time.Second*5)
}

func (s *CallbackServer) processCallback(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond*2500) // Slack requires to respond within 3000 milliseconds
	defer cancel()

	requestId := fmt.Sprintf("%s-%v", r.Header.Get("x-slack-request-timestamp"), atomic.AddUint64(&s.counter, 1))
	log := log.WithField("slack_http_id", requestId)

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
		log.WithError(err).Error("Failed to parse json response")
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	if err := s.onCallback(ctx, Callback{requestId, cb}); err != nil {
		log.WithError(err).Error("Failed to process callback")
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
