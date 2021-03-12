package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/trace"

	"github.com/go-resty/resty/v2"
	"github.com/nlopes/slack"
)

const slackMaxConns = 100
const slackHTTPTimeout = 10 * time.Second

// Bot is a wrapper around slack.Client that works with access.Request.
// It's responsible for formatting and posting a message on Slack when an
// action occurs with an access request: a new request popped up, or a
// request is processed/updated.
type Bot struct {
	client      *slack.Client
	respClient  *resty.Client
	clusterName string
	webProxyURL *url.URL
}

// NewBot initializes the new Slack message generator (Bot)
// takes SlackConfig as an argument.
func NewBot(conf Config, clusterName, webProxyAddr string) (*Bot, error) {
	httpClient := &http.Client{
		Timeout: slackHTTPTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     slackMaxConns,
			MaxIdleConnsPerHost: slackMaxConns,
		},
	}

	slackOptions := []slack.Option{
		slack.OptionHTTPClient(httpClient),
	}

	// APIURL parameter is set only in tests
	if conf.Slack.APIURL != "" {
		slackOptions = append(slackOptions, slack.OptionAPIURL(conf.Slack.APIURL))
	}

	respClient := resty.NewWithClient(httpClient)

	var webProxyURL *url.URL
	if webProxyAddr != "" {
		var err error
		if webProxyURL, err = url.Parse(webProxyAddr); err != nil {
			return nil, err
		}
	}

	return &Bot{
		client:      slack.New(conf.Slack.Token, slackOptions...),
		respClient:  respClient,
		clusterName: clusterName,
		webProxyURL: webProxyURL,
	}, nil
}

// Broadcast posts request info to Slack with action buttons.
func (b *Bot) Broadcast(ctx context.Context, channels []string, reqID string, reqData RequestData) (SlackData, []error) {
	var data SlackData
	var errors []error

	blocks := b.msgSections(reqID, reqData, "PENDING")

	for _, channel := range channels {
		channelID, timestamp, err := b.client.PostMessageContext(
			ctx,
			channel,
			slack.MsgOptionBlocks(blocks...),
		)
		if err != nil {
			errors = append(errors, trace.Wrap(err))
			continue
		}
		data = append(data, SlackDataMessage{ChannelID: channelID, Timestamp: timestamp})
	}

	return data, errors
}

// LookupDirectChannelByEmail fetches user's id by email.
func (b *Bot) LookupDirectChannelByEmail(ctx context.Context, email string) (string, error) {
	user, err := b.client.GetUserByEmailContext(ctx, email)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return user.ID, nil
}

// Expire updates request's Slack post with EXPIRED status and removes action buttons.
func (b *Bot) UpdateMessages(ctx context.Context, reqID string, reqData RequestData, slackData SlackData, status string) error {
	var errors []error
	for _, msg := range slackData {
		_, _, _, err := b.client.UpdateMessageContext(
			ctx,
			msg.ChannelID,
			msg.Timestamp,
			slack.MsgOptionBlocks(b.msgSections(reqID, reqData, status)...),
		)
		if err != nil {
			errors = append(errors, trace.Wrap(err))
		}
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// GetUserEmail takes a Slack User ID as input, and returns their
// email address.
// It might return an error if the Slack client can't fetch the user
// email for any reason.
func (b *Bot) GetUserEmail(ctx context.Context, id string) (string, error) {
	user, err := b.client.GetUserInfoContext(ctx, id)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return user.Profile.Email, nil
}

// Respond is used to send an updated message to Slack by "response_url" from interaction callback.
func (b *Bot) Respond(ctx context.Context, reqID string, reqData RequestData, status string, responseURL string) error {
	var message slack.Message
	message.Blocks.BlockSet = b.msgSections(reqID, reqData, status)
	message.ReplaceOriginal = true

	var result struct {
		Ok bool `json:"ok"`
	}

	resp, err := b.respClient.NewRequest().
		SetContext(ctx).
		SetBody(&message).
		SetResult(&result).
		Post(responseURL)
	if err != nil {
		return err
	}

	if !resp.IsSuccess() {
		return trace.Errorf("unexpected http status %q", resp.Status())
	}

	if !result.Ok {
		return trace.Errorf("operation status is not OK")
	}

	return nil
}

// msgSection builds a slack message section (obeys markdown).
func (b *Bot) msgSections(reqID string, reqData RequestData, status string) []slack.Block {
	var builder strings.Builder
	builder.Grow(128)

	msgFieldToBuilder(&builder, "ID", reqID)
	msgFieldToBuilder(&builder, "Cluster", b.clusterName)

	if len(reqData.User) > 0 {
		msgFieldToBuilder(&builder, "User", reqData.User)
	}
	if reqData.Roles != nil {
		msgFieldToBuilder(&builder, "Role(s)", strings.Join(reqData.Roles, ","))
	}
	if reqData.RequestReason != "" {
		msgFieldToBuilder(&builder, "Reason", reqData.RequestReason)
	}
	if b.webProxyURL != nil {
		reqURL := *b.webProxyURL
		reqURL.Path = lib.BuildURLPath("web", "requests", reqID)
		msgFieldToBuilder(&builder, "Link", reqURL.String())
	} else {
		if status != "EXPIRED" {
			msgFieldToBuilder(&builder, "Approve", fmt.Sprintf("tctl requests approve %s", reqID))
			msgFieldToBuilder(&builder, "Deny", fmt.Sprintf("tctl requests deny %s", reqID))
		}
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

	sections := []slack.Block{
		&slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: "You have a new Role Request:",
			},
		},
		&slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: builder.String(),
			},
		},
		&slack.ContextBlock{
			Type: slack.MBTContext,
			ContextElements: slack.ContextElements{
				Elements: []slack.MixedElement{
					&slack.TextBlockObject{
						Type: slack.MarkdownType,
						Text: fmt.Sprintf("*Status:* %s %s", statusEmoji, status),
					},
				},
			},
		},
	}

	return sections
}

func msgFieldToBuilder(b *strings.Builder, field, value string) {
	b.WriteString("*")
	b.WriteString(field)
	b.WriteString("*: ")
	b.WriteString(value)
	b.WriteString("\n")
}
