package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
	mm "github.com/mattermost/mattermost-server/v5/model"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

const ActionURL = "/mattermost_action"

type ActionData struct {
	UserID    string
	PostID    string
	ChannelID string
	Action    string
	ReqID     string
}

type ActionResponse mm.PostActionIntegrationResponse

type ActionFunc func(ctx context.Context, action ActionData) (*ActionResponse, error)

type ActionServer struct {
	auth     *ActionAuth
	http     *lib.HTTP
	onAction ActionFunc
	counter  uint64
}

func NewActionServer(config lib.HTTPConfig, auth *ActionAuth, onAction ActionFunc) (*ActionServer, error) {
	httpSrv, err := lib.NewHTTP(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	server := &ActionServer{
		http:     httpSrv,
		onAction: onAction,
		auth:     auth,
	}
	httpSrv.POST(ActionURL, server.ServeAction)
	return server, nil
}

func (s *ActionServer) ServiceJob() lib.ServiceJob {
	return s.http.ServiceJob()
}

func (s *ActionServer) ActionURL() string {
	return s.http.NewURL(ActionURL, nil).String()
}

func (s *ActionServer) BaseURL() *url.URL {
	return s.http.BaseURL()
}

func (s *ActionServer) EnsureCert() error {
	return s.http.EnsureCert(DefaultDir + "/server")
}

func (s *ActionServer) Run(ctx context.Context) error {
	if err := s.http.EnsureCert(DefaultDir + "/server"); err != nil {
		return err
	}
	return s.http.ListenAndServe(ctx)
}

func (s *ActionServer) Shutdown(ctx context.Context) error {
	return s.http.ShutdownWithTimeout(ctx, time.Second*5)
}

func (s *ActionServer) ServeAction(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond*2500)
	defer cancel()

	httpRequestID := fmt.Sprintf("%v-%v", time.Now().Unix(), atomic.AddUint64(&s.counter, 1))
	ctx, log := logger.WithField(ctx, "mm_http_id", httpRequestID)

	var payload mm.PostActionIntegrationRequest

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("Failed to read action payload")
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		log.WithError(err).Error("Failed to parse action payload")
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	action, ok := payload.Context["action"].(string)
	if !ok {
		log.Errorf(`Invalid type for "action" value, got: %v`, payload.Context["action"])
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	reqID, ok := payload.Context["req_id"].(string)
	if !ok {
		log.Errorf(`Invalid type for "req_id" value, got: %v`, payload.Context["req_id"])
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	encodedSignature, ok := payload.Context["signature"].(string)
	if !ok {
		log.Errorf(`Invalid type for "signature" value, got: %v`, payload.Context["signature"])
		http.Error(rw, "", http.StatusBadRequest)
		return
	}
	signature, err := base64.StdEncoding.DecodeString(encodedSignature)
	if err != nil {
		log.WithError(err).Error(`Failed to decode "signature" value`)
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	signatureOk, err := s.auth.Verify(action, reqID, signature)
	if err != nil {
		log.WithError(err).Errorf(`Failed to calculate HMAC value for %q/%q`, action, reqID)
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}

	if !signatureOk {
		log.Error(`Failed to validate "signature" value`)
		http.Error(rw, "", http.StatusUnauthorized)
		return
	}

	actionData := ActionData{
		UserID:    payload.UserId,
		PostID:    payload.PostId,
		ChannelID: payload.ChannelId,
		Action:    action,
		ReqID:     reqID,
	}

	actionResponse, err := s.onAction(ctx, actionData)
	if err != nil {
		log.WithError(err).Error("Failed to process mattermost action")
		log.Debugf("%v", trace.DebugReport(err))
		var code int
		switch {
		case lib.IsCanceled(err) || lib.IsDeadline(err):
			code = http.StatusServiceUnavailable
		default:
			code = http.StatusInternalServerError
		}
		http.Error(rw, "", code)
		return
	}

	respBody, err := json.Marshal(actionResponse)
	if err != nil {
		log.WithError(err).Error("Failed to serialize action response")
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, err = rw.Write(respBody)
	if err != nil {
		log.WithError(err).Error("Failed to send action response")
	}
}
