package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/trace"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
)

// Bot is a wrapper around slack.Client that works with access.Request
type Bot struct {
	client  *slack.Client
	channel string
}

func NewBot(conf *Config) *Bot {
	slackOptions := []slack.Option{}
	if conf.Slack.APIURL != "" {
		slackOptions = append(slackOptions, slack.OptionAPIURL(conf.Slack.APIURL))
	}

	return &Bot{
		client:  slack.New(conf.Slack.Token, slackOptions...),
		channel: conf.Slack.Channel,
	}
}

// Post posts request info to Slack with action buttons
func (c *Bot) Post(req access.Request) (channelID, timestamp string, err error) {
	channelID, timestamp, err = c.client.PostMessage(
		c.channel,
		slack.MsgOptionBlocks(
			msgSection(msgText(req, "PENDING")),
			actionBlock(req.ID),
		),
	)

	return
}

// Expire updates request's Slack post with EXPIRED status and removes action buttons.
// TODO: Use ext-data when it's integrated
func (c *Bot) Expire(req access.Request, channelID, timestamp string) error {
	if len(channelID) == 0 || len(timestamp) == 0 {
		log.Warningf("can't expire slack message without channel ID or timestamp")
		return nil
	}

	_, _, _, err := c.client.UpdateMessage(
		channelID,
		timestamp,
		slack.MsgOptionBlocks(
			msgSection(msgText(req, "EXPIRED")),
		),
	)

	return err
}

// RespondSlack updates Slack post with the new request info and the new status, and removes action buttons
func RespondSlack(req access.Request, status string, responseURL string) error {
	var message slack.Message
	message.Blocks.BlockSet = []slack.Block{msgSection(msgText(req, status))}
	message.ReplaceOriginal = true

	body, err := json.Marshal(message)
	if err != nil {
		return trace.Errorf("Failed to serialize msg block: %s", err)
	}

	rsp, err := http.Post(responseURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return trace.Errorf("Failed to send update: %s", err)
	}

	rbody, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return trace.Errorf("Failed to read update response: %s", err)
	}

	var ursp struct {
		Ok bool `json:"ok"`
	}
	if err := json.Unmarshal(rbody, &ursp); err != nil {
		return trace.Errorf("Failed to parse response body: %s", err)
	}

	if !ursp.Ok {
		return trace.Errorf("Failed to update msg for %+v", req)
	}

	return nil
}

// msgText builds the message text payload (contains markdown).
func msgText(req access.Request, status string) string {
	b := new(strings.Builder)
	b.Grow(128)

	fmt.Fprintln(b, "```")
	msgFieldToBuilder(b, "Request ", req.ID)
	msgFieldToBuilder(b, "Cluster ", req.ClusterName)

	if len(req.User) > 0 {
		msgFieldToBuilder(b, "User    ", req.User)
	}
	if req.Roles != nil {
		msgFieldToBuilder(b, "Role(s) ", strings.Join(req.Roles, ","))
	}

	msgFieldToBuilder(b, "Status  ", status)
	fmt.Fprintln(b, "```")

	return b.String()
}

func msgFieldToBuilder(b *strings.Builder, field, value string) {
	b.WriteString(field)
	b.WriteString(value)
	b.WriteString("\n")
}

// msgSection builds a slack message section (obeys markdown).
func msgSection(msg string) slack.SectionBlock {
	return slack.SectionBlock{
		Type: slack.MBTSection,
		Text: &slack.TextBlockObject{
			Type: slack.MarkdownType,
			Text: msg,
		},
	}
}

// actionBlock builds a slack action block for a pending request.
func actionBlock(reqID string) *slack.ActionBlock {
	return slack.NewActionBlock(
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
	)
}
