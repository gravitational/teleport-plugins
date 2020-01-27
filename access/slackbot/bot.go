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

var emojiByStatus = map[string]string{
	"PENDING":  ":hourglass_flowing_sand: ",
	"APPROVED": ":white_check_mark: ",
	"DENIED":   ":x: ",
	"EXPIRED":  ":hourglass: ",
}

// Bot is a wrapper around slack.Client that works with access.Request
type Bot struct {
	client      *slack.Client
	channel     string
	clusterName string
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
func (b *Bot) Post(reqID string, reqData requestData) (data slackData, err error) {
	data.channelID, data.timestamp, err = b.client.PostMessage(
		b.channel,
		slack.MsgOptionBlocks(b.msgSections(reqID, reqData, "PENDING", true)...),
	)

	return
}

// Expire updates request's Slack post with EXPIRED status and removes action buttons.
func (b *Bot) Expire(reqID string, reqData requestData, slackData slackData) error {
	if len(slackData.channelID) == 0 || len(slackData.timestamp) == 0 {
		log.Warningf("can't expire slack message without channel ID or timestamp")
		return nil
	}

	_, _, _, err := b.client.UpdateMessage(
		slackData.channelID,
		slackData.timestamp,
		slack.MsgOptionBlocks(b.msgSections(reqID, reqData, "EXPIRED", false)...),
	)

	return err
}

func (b *Bot) Respond(reqID string, reqData requestData, status string, responseURL string) error {
	var message slack.Message
	message.Blocks.BlockSet = b.msgSections(reqID, reqData, status, false)
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

// msgSection builds a slack message section (obeys markdown).
func (b *Bot) msgSections(reqID string, reqData requestData, status string, actions bool) []slack.Block {
	builder := new(strings.Builder)
	builder.Grow(128)

	msgFieldToBuilder(builder, "ID", reqID)
	msgFieldToBuilder(builder, "Cluster", b.clusterName)

	if len(reqData.user) > 0 {
		msgFieldToBuilder(builder, "User", reqData.user)
	}
	if reqData.roles != nil {
		msgFieldToBuilder(builder, "Role(s)", strings.Join(reqData.roles, ","))
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
						Text: fmt.Sprintf("*Status:* %s%s", emojiByStatus[status], status),
					},
				},
			},
		},
	}

	if actions {
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
