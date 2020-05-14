package main

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
	mm "github.com/mattermost/mattermost-server/model"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

const ActionURL = "/mattermost_action"

type BotAction struct {
	HttpRequestID string

	UserID    string
	PostID    string
	ChannelID string
	Action    string
	ReqID     string
}

type BotActionResponse struct {
	Status  string
	ReqData RequestData
}

type BotActionFunc func(ctx context.Context, action BotAction) (BotActionResponse, error)

type BotServer struct {
	bot      *Bot
	http     *utils.HTTP
	onAction BotActionFunc
	counter  uint64
}

func NewBotServer(bot *Bot, onAction BotActionFunc, config utils.HTTPConfig) (*BotServer, error) {
	httpSrv, err := utils.NewHTTP(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	server := &BotServer{
		bot:      bot,
		http:     httpSrv,
		onAction: onAction,
	}
	httpSrv.POST(ActionURL, server.OnAction)
	return server, nil
}

func (s *BotServer) ActionURL() string {
	return s.http.NewURL(ActionURL, nil).String()
}

func (s *BotServer) Run(ctx context.Context) error {
	if err := s.http.EnsureCert(DefaultDir + "/server"); err != nil {
		return err
	}
	return s.http.ListenAndServe(ctx)
}

func (s *BotServer) Shutdown(ctx context.Context) error {
	return s.http.ShutdownWithTimeout(ctx, time.Second*5)
}

func (s *BotServer) OnAction(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Millisecond*2500)
	defer cancel()

	httpRequestID := fmt.Sprintf("%v-%v", time.Now().Unix(), atomic.AddUint64(&s.counter, 1))
	log := log.WithField("mm_http_id", httpRequestID)

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
	payloadSignature, err := base64.StdEncoding.DecodeString(encodedSignature)
	if err != nil {
		log.WithError(err).Error(`Failed to decode "signature" value`)
		http.Error(rw, "", http.StatusBadRequest)
		return
	}

	signature, err := s.bot.HMAC(action, reqID)
	if err != nil {
		log.WithError(err).Errorf(`Failed to calculate HMAC value for %q/%q`, action, reqID)
		http.Error(rw, "", http.StatusInternalServerError)
		return
	}

	if !hmac.Equal(payloadSignature, signature) {
		log.Error(`Failed to validate "signature" value`)
		http.Error(rw, "", http.StatusUnauthorized)
		return
	}

	actionData := BotAction{
		HttpRequestID: httpRequestID,
		UserID:        payload.UserId,
		PostID:        payload.PostId,
		ChannelID:     payload.ChannelId,
		Action:        action,
		ReqID:         reqID,
	}

	if actionResponse, err := s.onAction(ctx, actionData); err != nil {
		log.WithError(err).Error("Failed to process mattermost action")
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
		actionsAttachment, err := s.bot.NewActionsAttachment(reqID, actionResponse.ReqData, actionResponse.Status)
		if err != nil {
			log.WithError(err).Error("Failed to build action response")
			http.Error(rw, "", http.StatusInternalServerError)
			return
		}
		response := &mm.PostActionIntegrationResponse{
			Update: &mm.Post{
				Id: payload.PostId,
				Props: mm.StringInterface{
					"attachments": []*mm.SlackAttachment{actionsAttachment},
				},
			},
			EphemeralText: fmt.Sprintf("You have **%s** the request %s", strings.ToLower(actionResponse.Status), reqID),
		}

		respBody, err := json.Marshal(response)
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
}
