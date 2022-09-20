package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	pd "github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	"github.com/go-resty/resty/v2"
)

const slackMaxConns = 100
const slackHTTPTimeout = 10 * time.Second

// SlackBot is a slack client that works with AccessRequest.
// It's responsible for formatting and posting a message on Slack when an
// action occurs with an access request: a new request popped up, or a
// request is processed/updated.
type SlackBot struct {
	client      *resty.Client
	clusterName string
	webProxyURL *url.URL
}

// NewSlackBot initializes the new Slack message generator (SlackBot)
// takes SlackConfig as an argument.
func NewSlackBot(conf Config, clusterName, webProxyAddr string) (SlackBot, error) {
	var (
		webProxyURL *url.URL
		err         error
	)
	if webProxyAddr != "" {
		if webProxyURL, err = lib.AddrToURL(webProxyAddr); err != nil {
			return SlackBot{}, trace.Wrap(err)
		}
	}

	token := "Bearer " + conf.Slack.Token
	if conf.Slack.IsDiscord {
		token = "SlackBot " + conf.Slack.Token
	}

	client := resty.
		NewWithClient(&http.Client{
			Timeout: slackHTTPTimeout,
			Transport: &http.Transport{
				MaxConnsPerHost:     slackMaxConns,
				MaxIdleConnsPerHost: slackMaxConns,
			},
		}).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", token)

	// APIURL parameter is set only in tests
	if endpoint := conf.Slack.APIURL; endpoint != "" {
		client.SetHostURL(endpoint)
	} else {
		client.SetHostURL("https://slack.com/api/")
		client.OnAfterResponse(onAfterResponseSlack)
	}

	return SlackBot{
		client:      client,
		clusterName: clusterName,
		webProxyURL: webProxyURL,
	}, nil
}

// onAfterResponseSlack resty error function for Slack
func onAfterResponseSlack(_ *resty.Client, resp *resty.Response) error {
	if !resp.IsSuccess() {
		return trace.Errorf("slack api returned unexpected code %v", resp.StatusCode())
	}

	var result SlackResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return trace.Wrap(err)
	}

	if !result.Ok {
		return trace.Errorf("%s", result.Error)
	}

	return nil
}

func (b SlackBot) HealthCheck(ctx context.Context) error {
	_, err := b.client.NewRequest().
		SetContext(ctx).
		Post("auth.test")
	if err != nil {
		if err.Error() == "invalid_auth" {
			return trace.Wrap(err, "authentication failed, probably invalid token")
		}
		return trace.Wrap(err)
	}
	return nil
}

// Broadcast posts request info to Slack with action buttons.
func (b SlackBot) Broadcast(ctx context.Context, channels []string, reqID string, reqData pd.AccessRequestData) (SlackData, error) {
	var data SlackData
	var errors []error

	for _, channel := range channels {
		var result ChatMsgResponse
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(SlackMsg{Msg: Msg{Channel: channel}, BlockItems: b.slackMsgSections(reqID, reqData)}).
			SetResult(&result).
			Post("chat.postMessage")
		if err != nil {
			errors = append(errors, trace.Wrap(err))
			continue
		}
		data = append(data, SlackDataMessage{ChannelID: result.Channel, TimestampOrDiscordID: result.Timestamp})
	}

	return data, trace.NewAggregate(errors...)
}

func (b SlackBot) PostReviewReply(ctx context.Context, channelID, timestamp string, review types.AccessReview) error {
	text, err := msgReview(review)
	if err != nil {
		return trace.Wrap(err)
	}

	_, err = b.client.NewRequest().
		SetContext(ctx).
		SetBody(SlackMsg{Msg: Msg{Channel: channelID, ThreadTs: timestamp}, Text: text}).
		Post("chat.postMessage")
	return trace.Wrap(err)
}

// LookupDirectChannelByEmail fetches user's id by email.
func (b SlackBot) LookupDirectChannelByEmail(ctx context.Context, email string) (string, error) {
	var result struct {
		SlackResponse
		User User `json:"user"`
	}
	_, err := b.client.NewRequest().
		SetContext(ctx).
		SetQueryParam("email", email).
		SetResult(&result).
		Get("users.lookupByEmail")
	if err != nil {
		return "", trace.Wrap(err)
	}

	return result.User.ID, nil
}

// Expire updates request's Slack post with EXPIRED status and removes action buttons.
func (b SlackBot) UpdateMessages(ctx context.Context, reqID string, reqData pd.AccessRequestData, slackData SlackData, reviews []types.AccessReview) error {
	var errors []error
	for _, msg := range slackData {
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(SlackMsg{Msg: Msg{
				Channel:   msg.ChannelID,
				Timestamp: msg.TimestampOrDiscordID,
			}, BlockItems: b.slackMsgSections(reqID, reqData)}).
			Post("chat.update")
		if err != nil {
			switch err.Error() {
			case "message_not_found":
				err = trace.Wrap(err, "cannot find message with timestamp %s in channel %s", msg.TimestampOrDiscordID, msg.ChannelID)
			default:
				err = trace.Wrap(err)
			}
			errors = append(errors, trace.Wrap(err))
		}
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// msgSection builds a slack message section (obeys markdown).
func (b SlackBot) slackMsgSections(reqID string, reqData pd.AccessRequestData) []BlockItem {
	fields := msgFields(reqID, reqData, b.webProxyURL)
	statusText := msgStatusText(reqData.ResolutionTag, reqData.ResolutionReason)

	sections := []BlockItem{
		NewBlockItem(SectionBlock{
			Text: NewTextObjectItem(MarkdownObject{Text: "You have a new Role Request:"}),
		}),
		NewBlockItem(SectionBlock{
			Text: NewTextObjectItem(MarkdownObject{Text: fields}),
		}),
		NewBlockItem(ContextBlock{
			ElementItems: []ContextElementItem{
				NewContextElementItem(MarkdownObject{Text: statusText}),
			},
		}),
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
