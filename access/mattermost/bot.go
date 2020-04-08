package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"
	"time"

	mm "github.com/mattermost/mattermost-server/model"

	"github.com/gravitational/trace"
	// log "github.com/sirupsen/logrus"
)

const (
	mmMaxConns    = 100
	mmHttpTimeout = 10 * time.Second
)

const DescriptionTemplate = `User:        {{.User}}
Roles:       {{range $index, $element := .Roles}}{{if $index}}, {{end}}{{ . }}{{end}}
Request ID:  {{.ID}}
Status:      {{.StatusEmoji}} {{.Status}}
`

// Bot is a wrapper around jira.Client that works with access.Request
type Bot struct {
	client      *mm.Client4
	server      *BotServer
	secret      string
	team        string
	channel     string
	clusterName string
}

func NewBot(conf *Config, onAction BotActionFunc) (*Bot, error) {
	client := mm.NewAPIv4Client(conf.Mattermost.URL)
	client.SetToken(conf.Mattermost.Token)
	bot := &Bot{
		client:  client,
		secret:  conf.Mattermost.Secret,
		team:    conf.Mattermost.Team,
		channel: conf.Mattermost.Channel,
	}
	bot.server = NewBotServer(bot, onAction, conf.HTTP)
	return bot, nil
}

func (b *Bot) RunServer(ctx context.Context) error {
	return b.server.Run(ctx)
}

func (b *Bot) ShutdownServer(ctx context.Context) error {
	return b.server.Shutdown(ctx)
}

func (b *Bot) HealthCheck() error {
	_, resp := b.client.GetTeamByName(b.team, "")
	if resp.Error != nil {
		return trace.Wrap(resp.Error)
	}
	return nil
}

func (b *Bot) HMAC(action, reqID string) ([]byte, error) {
	data := fmt.Sprintf("%s/%s", action, reqID)
	mac := hmac.New(sha256.New, []byte(b.secret))
	_, err := mac.Write([]byte(data))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return mac.Sum(nil), nil
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

func (b *Bot) NewPostAction(actionId, actionName, reqID string) (*mm.PostAction, error) {
	signature, err := b.HMAC(actionId, reqID)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	actionURL, err := b.server.ActionURL()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &mm.PostAction{
		Name: actionName,
		Integration: &mm.PostActionIntegration{
			URL: actionURL,
			Context: mm.StringInterface{
				"action":    actionId,
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

	text, err := b.GetPostText(reqID, reqData, status)
	if err != nil {
		return nil, err
	}

	return &mm.SlackAttachment{
		Text:    text,
		Actions: actions,
	}, nil

}

func (b *Bot) GetPostText(reqID string, reqData RequestData, status string) (string, error) {
	tmpl, err := template.New("description").Parse(DescriptionTemplate)
	if err != nil {
		return "", trace.Wrap(err)
	}

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

	var builder strings.Builder

	_, err = builder.WriteString("```\n")
	if err != nil {
		return "", trace.Wrap(err)
	}

	err = tmpl.Execute(&builder, struct {
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
