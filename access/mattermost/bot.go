package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	mm "github.com/mattermost/mattermost-server/v5/model"

	"github.com/gravitational/trace"
	// log "github.com/sirupsen/logrus"
)

const (
	mmMaxConns    = 100
	mmHTTPTimeout = 10 * time.Second
)

var postTextTemplate *template.Template

func init() {
	var err error
	postTextTemplate, err = template.New("description").Parse(
		`User:        {{.User}}
Roles:       {{range $index, $element := .Roles}}{{if $index}}, {{end}}{{ . }}{{end}}
Request ID:  {{.ID}}
Status:      {{.StatusEmoji}} {{.Status}}
`,
	)
	if err != nil {
		panic(err)
	}
}

// Bot is a wrapper around jira.Client that works with access.Request
type Bot struct {
	client      *mm.Client4
	server      *ActionServer
	auth        *ActionAuth
	team        string
	channel     string
	clusterName string
}

func NewBot(conf MattermostConfig, server *ActionServer, auth *ActionAuth) *Bot {
	client := mm.NewAPIv4Client(conf.URL)
	client.SetToken(conf.Token)
	client.HttpClient = &http.Client{
		Timeout: mmHTTPTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     mmMaxConns,
			MaxIdleConnsPerHost: mmMaxConns,
		},
	}
	return &Bot{
		client:  client,
		server:  server,
		auth:    auth,
		team:    conf.Team,
		channel: conf.Channel,
	}
}

func (b *Bot) HealthCheck() error {
	_, resp := b.client.GetTeamByName(b.team, "")
	if resp.Error != nil {
		return trace.Wrap(resp.Error)
	}
	return nil
}

// Post posts request info to Mattermost with action buttons.
func (b *Bot) CreatePost(ctx context.Context, reqID string, reqData RequestData) (data MattermostData, err error) {
	team, resp := b.client.GetTeamByName(b.team, "")
	if resp.Error != nil {
		err = trace.Wrap(resp.Error)
		return
	}
	channel, resp := b.client.GetChannelByName(b.channel, team.Id, "")
	if resp.Error != nil {
		err = trace.Wrap(resp.Error)
		return
	}

	actionsAttachment, err := b.NewActionsAttachment(reqID, reqData, "PENDING")
	if err != nil {
		return
	}

	post, resp := b.client.CreatePost(&mm.Post{
		ChannelId: channel.Id,
		Props: mm.StringInterface{
			"attachments": []*mm.SlackAttachment{actionsAttachment},
		},
	})
	if resp.Error != nil {
		err = trace.Wrap(resp.Error)
		return
	}
	data.PostID = post.Id
	data.ChannelID = post.ChannelId
	return
}

func (b *Bot) ExpirePost(ctx context.Context, reqID string, reqData RequestData, mmData MattermostData) error {
	actionsAttachment, err := b.NewActionsAttachment(reqID, reqData, "EXPIRED")
	if err != nil {
		return trace.Wrap(err)
	}

	_, resp := b.client.UpdatePost(mmData.PostID, &mm.Post{
		Id: mmData.PostID,
		Props: mm.StringInterface{
			"attachments": []*mm.SlackAttachment{actionsAttachment},
		},
	})
	if resp.Error != nil {
		return trace.Wrap(resp.Error)
	}
	return nil
}

func (b *Bot) GetUser(ctx context.Context, userID string) (*mm.User, error) {
	user, resp := b.client.GetUser(userID, "")
	if resp.Error != nil {
		return &mm.User{}, trace.Wrap(resp.Error)
	}

	return user, nil
}

func (b *Bot) NewPostAction(actionID, actionName, reqID string) (*mm.PostAction, error) {
	signature, err := b.auth.Sign(actionID, reqID)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	actionURL := b.server.ActionURL()

	return &mm.PostAction{
		Name: actionName,
		Integration: &mm.PostActionIntegration{
			URL: actionURL,
			Context: mm.StringInterface{
				"action":    actionID,
				"req_id":    reqID,
				"signature": base64.StdEncoding.EncodeToString(signature),
			},
		},
	}, nil
}

func (b *Bot) NewActionsAttachment(reqID string, reqData RequestData, status string) (*mm.SlackAttachment, error) {
	var actions []*mm.PostAction
	if status == "PENDING" {
		approveAction, err := b.NewPostAction("approve", "Approve", reqID)
		if err != nil {
			return nil, err
		}
		denyAction, err := b.NewPostAction("deny", "Deny", reqID)
		if err != nil {
			return nil, err
		}
		actions = append(actions, approveAction, denyAction)
	}

	text, err := b.buildPostText(reqID, reqData, status)
	if err != nil {
		return nil, err
	}

	return &mm.SlackAttachment{
		Text:    text,
		Actions: actions,
	}, nil
}

func (b *Bot) NewActionResponse(postID string, reqID string, reqData RequestData, status string) (*ActionResponse, error) {
	actionsAttachment, err := b.NewActionsAttachment(reqID, reqData, status)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &ActionResponse{
		Update: &mm.Post{
			Id: postID,
			Props: mm.StringInterface{
				"attachments": []*mm.SlackAttachment{actionsAttachment},
			},
		},
		EphemeralText: fmt.Sprintf("You have **%s** the request %s", strings.ToLower(status), reqID),
	}, nil
}

func (b *Bot) buildPostText(reqID string, reqData RequestData, status string) (string, error) {
	var statusEmoji string

	switch status {
	case "PENDING":
		statusEmoji = "⏳"
	case "APPROVED":
		statusEmoji = "✅"
	case "DENIED":
		statusEmoji = "❌"
	case "EXPIRED":
		statusEmoji = "⌛"
	}

	var (
		builder strings.Builder
		err     error
	)

	_, err = builder.WriteString("```\n")
	if err != nil {
		return "", trace.Wrap(err)
	}

	err = postTextTemplate.Execute(&builder, struct {
		ID          string
		Status      string
		StatusEmoji string
		RequestData
	}{
		reqID,
		status,
		statusEmoji,
		reqData,
	})
	if err != nil {
		return "", trace.Wrap(err)
	}

	_, err = builder.WriteString("\n```")
	if err != nil {
		return "", trace.Wrap(err)
	}

	return builder.String(), nil
}
