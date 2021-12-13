package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	"github.com/go-resty/resty/v2"
)

const slackMaxConns = 100
const slackHTTPTimeout = 10 * time.Second

var reviewReplyTemplate = template.Must(template.New("review reply").Parse(
	`{{.Author}} reviewed the request at {{.Created.Format .TimeFormat}}.
Resolution: {{.ProposedStateEmoji}} {{.ProposedState}}.
{{if .Reason}}Reason: {{.Reason}}.{{end}}`,
))

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

	blockItems := b.msgSections(reqID, reqData)

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

func (b Bot) PostReviewReply(ctx context.Context, channelID, timestamp string, review types.AccessReview) error {
	// see reasoning in msgSections
	if review.Reason != "" {
		review.Reason = lib.MarkdownEscape(review.Reason, 500)
	}

	var proposedStateEmoji string
	switch review.ProposedState {
	case types.RequestState_APPROVED:
		proposedStateEmoji = "✅"
	case types.RequestState_DENIED:
		proposedStateEmoji = "❌"
	}

	var builder strings.Builder
	err := reviewReplyTemplate.Execute(&builder, struct {
		types.AccessReview
		ProposedState      string
		ProposedStateEmoji string
		TimeFormat         string
	}{
		review,
		review.ProposedState.String(),
		proposedStateEmoji,
		time.RFC822,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	text := builder.String()

	_, err = b.client.NewRequest().
		SetContext(ctx).
		SetBody(Msg{Channel: channelID, ThreadTs: timestamp, Text: text}).
		Post("chat.postMessage")
	return trace.Wrap(err)
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
func (b Bot) UpdateMessages(ctx context.Context, reqID string, reqData RequestData, slackData SlackData) error {
	var errors []error
	for _, msg := range slackData {
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(Msg{
				Channel:    msg.ChannelID,
				Timestamp:  msg.Timestamp,
				BlockItems: b.msgSections(reqID, reqData),
			}).
			Post("chat.update")
		if err != nil {
			switch err.Error() {
			case "message_not_found":
				err = trace.Wrap(err, "cannot find message with timestamp %s in channel %s", msg.Timestamp, msg.ChannelID)
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
func (b Bot) Respond(ctx context.Context, reqID string, reqData RequestData, responseURL string) error {
	var message RespondMsg
	message.BlockItems = b.msgSections(reqID, reqData)
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
		return trace.Errorf("unexpected http status %s", resp.Status())
	}

	if !result.Ok {
		return trace.Errorf("operation status is not OK")
	}

	return nil
}

// msgSection builds a slack message section (obeys markdown).
func (b Bot) msgSections(reqID string, reqData RequestData) []BlockItem {
	var builder strings.Builder
	builder.Grow(128)

	resolution := reqData.Resolution

	msgFieldToBuilder(&builder, "ID", reqID)
	msgFieldToBuilder(&builder, "Cluster", b.clusterName)

	if len(reqData.User) > 0 {
		msgFieldToBuilder(&builder, "User", reqData.User)
	}
	if reqData.Roles != nil {
		msgFieldToBuilder(&builder, "Role(s)", strings.Join(reqData.Roles, ","))
	}
	if reqData.RequestReason != "" {
		// Slack has a 4000 character recommendation (with a more generous 40000
		// hard limit), Mattermost has either 4000 or 16k depending on the
		// version; let's just be very conservative and limit request reason and
		// resolution reason to 500 each
		msgFieldToBuilder(&builder, "Reason", lib.MarkdownEscape(reqData.RequestReason, 500))
	}
	if b.webProxyURL != nil {
		reqURL := *b.webProxyURL
		reqURL.Path = lib.BuildURLPath("web", "requests", reqID)
		msgFieldToBuilder(&builder, "Link", reqURL.String())
	} else {
		if resolution.Tag == Unresolved {
			msgFieldToBuilder(&builder, "Approve", fmt.Sprintf("`tsh request review --approve %s`", reqID))
			msgFieldToBuilder(&builder, "Deny", fmt.Sprintf("`tsh request review --deny %s`", reqID))
		}
	}

	var statusEmoji string
	status := string(resolution.Tag)
	switch resolution.Tag {
	case Unresolved:
		status = "PENDING"
		statusEmoji = "⏳"
	case ResolvedApproved:
		statusEmoji = "✅"
	case ResolvedDenied:
		statusEmoji = "❌"
	case ResolvedExpired:
		statusEmoji = "⌛"
	}

	statusText := fmt.Sprintf("*Status:* %s %s", statusEmoji, status)
	if resolution.Reason != "" {
		// see above for the limit reasoning
		statusText += fmt.Sprintf("\n*Resolution reason*: %s", lib.MarkdownEscape(resolution.Reason, 500))
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
