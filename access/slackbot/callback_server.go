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

type Callback = slack.InteractionCallback
type CallbackFunc func(ctx context.Context, callback Callback) error

// CallbackServer is a wrapper around http.Server that processes Slack interaction events.
// It verifies incoming requests and calls onCallback for valid ones
type CallbackServer struct {
	secret     string
	onCallback CallbackFunc
	httpServer *http.Server
	keyFile    string
	certFile   string
}

func NewCallbackServer(ctx context.Context, conf *Config, onCallback CallbackFunc) *CallbackServer {
	s := CallbackServer{
		secret:     conf.Slack.Secret,
		onCallback: onCallback,
		keyFile:    conf.HTTP.KeyFile,
		certFile:   conf.HTTP.CertFile,
	}

	s.httpServer = &http.Server{
		Addr: conf.HTTP.Listen,
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
	var err error
	if s.certFile != "" {
		log.Infof("Starting secure HTTPS server on %s", s.httpServer.Addr)
		err = s.httpServer.ListenAndServeTLS(s.certFile, s.keyFile)
	} else {
		log.Infof("Starting insecure HTTP server on %s", s.httpServer.Addr)
		err = s.httpServer.ListenAndServe()
	}
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

	if r.Method != "POST" {
		http.Error(rw, "", 400)
		return
	}

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

	if err := s.onCallback(ctx, cb); err != nil {
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
