package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

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
func (c *Bot) Post(reqID string, reqData requestData) (data slackData, err error) {
	data.channelID, data.timestamp, err = c.client.PostMessage(
		c.channel,
		slack.MsgOptionBlocks(
			msgSection(msgText(reqID, reqData, "PENDING")),
			actionBlock(reqID),
		),
	)

	return
}

// Expire updates request's Slack post with EXPIRED status and removes action buttons.
func (c *Bot) Expire(reqID string, reqData requestData, slackData slackData) error {
	if len(slackData.channelID) == 0 || len(slackData.timestamp) == 0 {
		log.Warningf("can't expire slack message without channel ID or timestamp")
		return nil
	}

	_, _, _, err := c.client.UpdateMessage(
		slackData.channelID,
		slackData.timestamp,
		slack.MsgOptionBlocks(
			msgSection(msgText(reqID, reqData, "EXPIRED")),
		),
	)

	return err
}

// RespondSlack updates Slack post with the new request info and the new status, and removes action buttons
func RespondSlack(reqID string, reqData requestData, status string, responseURL string) error {
	var message slack.Message
	message.Blocks.BlockSet = []slack.Block{msgSection(msgText(reqID, reqData, status))}
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
		return trace.Errorf("Failed to update msg for %v", reqID)
	}

	return nil
}

// msgText builds the message text payload (contains markdown).
func msgText(reqID string, reqData requestData, status string) string {
	b := new(strings.Builder)
	b.Grow(128)

	fmt.Fprintln(b, "```")
	msgFieldToBuilder(b, "Request ", reqID)

	if len(reqData.user) > 0 {
		msgFieldToBuilder(b, "User    ", reqData.user)
	}
	if reqData.roles != nil {
		msgFieldToBuilder(b, "Role(s) ", strings.Join(reqData.roles, ","))
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
