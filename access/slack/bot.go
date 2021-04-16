package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/trace"

	"github.com/go-resty/resty/v2"
)

const slackMaxConns = 100
const slackHTTPTimeout = 10 * time.Second

// Bot is a slack client that works with access.Request.
// It's responsible for formatting and posting a message on Slack when an
// action occurs with an access request: a new request popped up, or a
// request is processed/updated.
type Bot struct {
	client      *resty.Client
	respClient  *resty.Client
	clusterName string
	webProxyURL *url.URL
}

// NewBot initializes the new Slack message generator (Bot)
// takes SlackConfig as an argument.
func NewBot(conf Config, clusterName, webProxyAddr string) (Bot, error) {
	var (
		webProxyURL *url.URL
		err         error
	)
	if webProxyAddr != "" {
		if webProxyURL, err = lib.AddrToURL(webProxyAddr); err != nil {
			return Bot{}, trace.Wrap(err)
		}
	}

	httpClient := &http.Client{
		Timeout: slackHTTPTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     slackMaxConns,
			MaxIdleConnsPerHost: slackMaxConns,
		},
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
		SetHeader("Authorization", "Bearer "+conf.Slack.Token)
	// APIURL parameter is set only in tests
	if endpoint := conf.Slack.APIURL; endpoint != "" {
		client.SetHostURL(endpoint)
	} else {
		client.SetHostURL("https://slack.com/api/")
	}

	// Error response handling
	client.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
		if !resp.IsSuccess() {
			return trace.Errorf("slack api returned unexpected code %v", resp.StatusCode())
		}
		var result Response
		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return trace.Wrap(err)
		}

		if !result.Ok {
			return trace.Errorf("%s", result.Error)
		}

		return nil
	})

	respClient := resty.NewWithClient(httpClient)

	return Bot{
		client:      client,
		respClient:  respClient,
		clusterName: clusterName,
		webProxyURL: webProxyURL,
	}, nil
}

func (b Bot) HealthCheck(ctx context.Context) error {
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
func (b Bot) Broadcast(ctx context.Context, channels []string, reqID string, reqData RequestData) (SlackData, error) {
	var data SlackData
	var errors []error

	blockItems := b.msgSections(reqID, reqData, "PENDING")

	for _, channel := range channels {
		var result ChatMsgResponse
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(Msg{Channel: channel, BlockItems: blockItems}).
			SetResult(&result).
			Post("chat.postMessage")
		if err != nil {
			errors = append(errors, trace.Wrap(err))
			continue
		}
		data = append(data, SlackDataMessage{ChannelID: result.Channel, Timestamp: result.Timestamp})
	}

	return data, trace.NewAggregate(errors...)
}

// LookupDirectChannelByEmail fetches user's id by email.
func (b Bot) LookupDirectChannelByEmail(ctx context.Context, email string) (string, error) {
	var result struct {
		Response
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
func (b Bot) UpdateMessages(ctx context.Context, reqID string, reqData RequestData, slackData SlackData, status string) error {
	var errors []error
	for _, msg := range slackData {
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(Msg{
				Channel:    msg.ChannelID,
				Timestamp:  msg.Timestamp,
				BlockItems: b.msgSections(reqID, reqData, status),
			}).
			Post("chat.update")
		if err != nil {
			switch err.Error() {
			case "message_not_found":
				err = trace.Wrap(err, "cannot find message with timestamp %q in channel %q", msg.Timestamp, msg.ChannelID)
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

// Respond is used to send an updated message to Slack by "response_url" from interaction callback.
func (b Bot) Respond(ctx context.Context, reqID string, reqData RequestData, status string, responseURL string) error {
	var message RespondMsg
	message.BlockItems = b.msgSections(reqID, reqData, status)
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
func (b Bot) msgSections(reqID string, reqData RequestData, status string) []BlockItem {
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
		if status == "PENDING" {
			msgFieldToBuilder(&builder, "Approve", fmt.Sprintf("`tsh request review --aprove %s`", reqID))
			msgFieldToBuilder(&builder, "Deny", fmt.Sprintf("`tsh request review --deny %s`", reqID))
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

	sections := []BlockItem{
		NewBlockItem(SectionBlock{
			Text: NewTextObjectItem(MarkdownObject{Text: "You have a new Role Request:"}),
		}),
		NewBlockItem(SectionBlock{
			Text: NewTextObjectItem(MarkdownObject{Text: builder.String()}),
		}),
		NewBlockItem(ContextBlock{
			ElementItems: []ContextElementItem{
				NewContextElementItem(MarkdownObject{Text: fmt.Sprintf("*Status:* %s %s", statusEmoji, status)}),
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
