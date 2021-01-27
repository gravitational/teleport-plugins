package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/gravitational/trace"
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
	respClient  *http.Client
	channel     string
	clusterName string
	notifyOnly  bool
}

// NewBot initializes the new Slack message generator (Bot)
// takes SlackConfig as an argument.
func NewBot(conf SlackConfig) *Bot {
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
	if conf.APIURL != "" {
		slackOptions = append(slackOptions, slack.OptionAPIURL(conf.APIURL))
	}

	return &Bot{
		client:     slack.New(conf.Token, slackOptions...),
		channel:    conf.Channel,
		respClient: httpClient,
		notifyOnly: conf.NotifyOnly,
	}
}

// Post posts request info to Slack with action buttons.
func (b *Bot) Post(ctx context.Context, reqID string, reqData RequestData) (data SlackData, err error) {
	data.ChannelID, data.Timestamp, err = b.client.PostMessageContext(
		ctx,
		b.channel,
		slack.MsgOptionBlocks(b.msgSections(reqID, reqData, "PENDING")...),
	)
	err = trace.Wrap(err)

	return
}

// Expire updates request's Slack post with EXPIRED status and removes action buttons.
func (b *Bot) Expire(ctx context.Context, reqID string, reqData RequestData, slackData SlackData) error {
	_, _, _, err := b.client.UpdateMessageContext(
		ctx,
		slackData.ChannelID,
		slackData.Timestamp,
		slack.MsgOptionBlocks(b.msgSections(reqID, reqData, "EXPIRED")...),
	)

	return trace.Wrap(err)
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

	body, err := json.Marshal(message)
	if err != nil {
		return trace.Wrap(err, "failed to serialize msg block: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, responseURL, bytes.NewReader(body))
	if err != nil {
		return trace.Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rsp, err := b.respClient.Do(req)
	if err != nil {
		return trace.Wrap(err, "failed to send update: %v", err)
	}
	defer rsp.Body.Close()

	rbody, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return trace.Wrap(err, "failed to read update response: %v", err)
	}

	var ursp struct {
		Ok bool `json:"ok"`
	}
	if err := json.Unmarshal(rbody, &ursp); err != nil {
		return trace.Wrap(err, "failed to parse response body: %v", err)
	}

	if !ursp.Ok {
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

	var statusEmoji string
	switch status {
	case "PENDING":
		statusEmoji = ":hourglass_flowing_sand:"
	case "APPROVED":
		statusEmoji = ":white_check_mark:"
	case "DENIED":
		statusEmoji = ":x:"
	case "EXPIRED":
		statusEmoji = ":hourglass:"
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

	// Only show buttons for pending requests, and if the plugin is
	// working in interactive mode (i.e. notify-only)
	if status == "PENDING" && !b.notifyOnly {
		sections = append(sections, slack.NewActionBlock(
			"approve_or_deny",
			&slack.ButtonBlockElement{
				Type:     slack.METButton,
				ActionID: ActionApprove,
				Text:     slack.NewTextBlockObject("plain_text", "Approve", true, false),
				Value:    reqID,
				Style:    slack.StylePrimary,
			},
			&slack.ButtonBlockElement{
				Type:     slack.METButton,
				ActionID: ActionDeny,
				Text:     slack.NewTextBlockObject("plain_text", "Deny", true, false),
				Value:    reqID,
				Style:    slack.StyleDanger,
			},
		))
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
