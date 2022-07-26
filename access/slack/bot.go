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
	pd "github.com/gravitational/teleport-plugins/lib/plugindata"
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

// Slack has a 4000 character limit for message texts and 3000 character limit
// for message section texts so we truncate all reasons to a generous but
// conservative limit
const (
	requestReasonLimit = 500
	resolutionReasonLimit
	reviewReasonLimit
)

// Bot is a slack/discord client that works with AccessRequest.
// It's responsible for formatting and posting a message on Slack when an
// action occurs with an access request: a new request popped up, or a
// request is processed/updated.
type Bot struct {
	client      *resty.Client
	respClient  *resty.Client
	clusterName string
	webProxyURL *url.URL
	isDiscord   bool
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

	token := "Bearer " + conf.Slack.Token
	if conf.Slack.IsDiscord {
		token = "Bot " + conf.Slack.Token
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
		if conf.Slack.IsDiscord {
			client.SetHostURL("https://discord.com/api/")
			client.OnAfterResponse(onAfterResponseDiscord)
		} else {
			client.SetHostURL("https://slack.com/api/")
			client.OnAfterResponse(onAfterResponseSlack)
		}
	}

	// Error response handling

	respClient := resty.NewWithClient(httpClient)

	return Bot{
		client:      client,
		respClient:  respClient,
		clusterName: clusterName,
		webProxyURL: webProxyURL,
		isDiscord:   conf.Slack.IsDiscord,
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

// onAfterResponseDiscord resty error function for Discord
func onAfterResponseDiscord(_ *resty.Client, resp *resty.Response) error {
	if resp.IsSuccess() {
		return nil
	}

	var result DiscordResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return trace.Wrap(err)
	}

	if result.Message != "" {
		return trace.Errorf("%s (code: %v, status: %d)", result.Message, result.Code, resp.StatusCode())
	}

	return trace.Errorf("Discord API returned error: %s (status: %d)", string(resp.Body()), resp.StatusCode())
}

func (b Bot) HealthCheck(ctx context.Context) error {
	if b.isDiscord {
		_, err := b.client.NewRequest().
			SetContext(ctx).
			Get("/users/@me")
		if err != nil {
			return trace.Wrap(err, "health check failed, probably invalid token")
		}
	} else {
		_, err := b.client.NewRequest().
			SetContext(ctx).
			Post("auth.test")
		if err != nil {
			if err.Error() == "invalid_auth" {
				return trace.Wrap(err, "authentication failed, probably invalid token")
			}
			return trace.Wrap(err)
		}
	}
	return nil
}

// Broadcast posts request info to Slack with action buttons.
func (b Bot) Broadcast(ctx context.Context, channels []string, reqID string, reqData pd.AccessRequestData) (SlackData, error) {
	var data SlackData
	var errors []error

	for _, channel := range channels {
		var result ChatMsgResponse
		if b.isDiscord {
			_, err := b.client.NewRequest().
				SetContext(ctx).
				SetBody(DiscordMsg{Msg: Msg{Channel: channel}, Text: b.discordMsgText(reqID, reqData, nil)}).
				SetResult(&result).
				Post("/channels/" + channel + "/messages")
			if err != nil {
				errors = append(errors, trace.Wrap(err))
				continue
			}
			data = append(data, SlackDataMessage{ChannelID: channel, TimestampOrDiscordID: result.DiscordID})
		} else {
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
	}

	return data, trace.NewAggregate(errors...)
}

func (b Bot) msgReview(review types.AccessReview) (string, error) {
	if review.Reason != "" {
		review.Reason = lib.MarkdownEscape(review.Reason, reviewReasonLimit)
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
		return "", trace.Wrap(err)
	}
	return builder.String(), nil
}

func (b Bot) PostReviewReply(ctx context.Context, channelID, timestamp string, review types.AccessReview) error {
	text, err := b.msgReview(review)
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
func (b Bot) LookupDirectChannelByEmail(ctx context.Context, email string) (string, error) {
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
func (b Bot) UpdateMessages(ctx context.Context, reqID string, reqData pd.AccessRequestData, slackData SlackData, reviews []types.AccessReview) error {
	var errors []error
	for _, msg := range slackData {
		if b.isDiscord {
			_, err := b.client.NewRequest().
				SetContext(ctx).
				SetBody(DiscordMsg{Msg: Msg{Channel: msg.ChannelID}, Text: b.discordMsgText(reqID, reqData, reviews)}).
				Patch("/channels/" + msg.ChannelID + "/messages/" + msg.TimestampOrDiscordID)
			if err != nil {
				errors = append(errors, trace.Wrap(err))
			}
		} else {
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
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// Respond is used to send an updated message to Slack by "response_url" from interaction callback.
func (b Bot) Respond(ctx context.Context, reqID string, reqData pd.AccessRequestData, responseURL string) error {
	var message RespondMsg
	message.BlockItems = b.slackMsgSections(reqID, reqData)
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

func (b Bot) msgFields(reqID string, reqData pd.AccessRequestData) string {
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
		msgFieldToBuilder(&builder, "Reason", lib.MarkdownEscape(reqData.RequestReason, requestReasonLimit))
	}
	if b.webProxyURL != nil {
		reqURL := *b.webProxyURL
		reqURL.Path = lib.BuildURLPath("web", "requests", reqID)
		msgFieldToBuilder(&builder, "Link", reqURL.String())
	} else {
		if reqData.ResolutionTag == pd.Unresolved {
			msgFieldToBuilder(&builder, "Approve", fmt.Sprintf("`tsh request review --approve %s`", reqID))
			msgFieldToBuilder(&builder, "Deny", fmt.Sprintf("`tsh request review --deny %s`", reqID))
		}
	}

	return builder.String()
}

func (b Bot) msgStatusText(tag pd.ResolutionTag, reason string) string {
	var statusEmoji string
	status := string(tag)
	switch tag {
	case pd.Unresolved:
		status = "PENDING"
		statusEmoji = "⏳"
	case pd.ResolvedApproved:
		statusEmoji = "✅"
	case pd.ResolvedDenied:
		statusEmoji = "❌"
	case pd.ResolvedExpired:
		statusEmoji = "⌛"
	}

	statusText := fmt.Sprintf("*Status:* %s %s", statusEmoji, status)
	if reason != "" {
		statusText += fmt.Sprintf("\n*Resolution reason*: %s", lib.MarkdownEscape(reason, resolutionReasonLimit))
	}

	return statusText
}

// msgSection builds a slack message section (obeys markdown).
func (b Bot) slackMsgSections(reqID string, reqData pd.AccessRequestData) []BlockItem {
	fields := b.msgFields(reqID, reqData)
	statusText := b.msgStatusText(reqData.ResolutionTag, reqData.ResolutionReason)

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

func (b Bot) discordMsgText(reqID string, reqData pd.AccessRequestData, reviews []types.AccessReview) string {
	return "You have a new Role Request:\n" +
		b.msgFields(reqID, reqData) +
		b.msgDiscordReviews(reviews) +
		b.msgStatusText(reqData.ResolutionTag, reqData.ResolutionReason)
}

func (b Bot) msgDiscordReviews(reviews []types.AccessReview) string {
	var result = ""

	// TODO: Update error propagation
	for _, review := range reviews {
		text, err := b.msgReview(review)
		if err != nil {
			return ""
		}

		result += text + "\n"
	}

	return "\n" + result
}

func msgFieldToBuilder(b *strings.Builder, field, value string) {
	b.WriteString("*")
	b.WriteString(field)
	b.WriteString("*: ")
	b.WriteString(value)
	b.WriteString("\n")
}
