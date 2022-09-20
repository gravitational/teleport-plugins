package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	pd "github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	"github.com/go-resty/resty/v2"
)

const discordMaxConns = 100
const discordHTTPTimeout = 10 * time.Second

// DiscordBot is a discord client that works with AccessRequest.
// It's responsible for formatting and posting a message on Discord when an
// action occurs with an access request: a new request popped up, or a
// request is processed/updated.
type DiscordBot struct {
	client      *resty.Client
	clusterName string
	webProxyURL *url.URL
}

// NewDiscordBot initializes the new Discord message generator (DiscordBot)
// takes Config as an argument.
func NewDiscordBot(conf Config, clusterName, webProxyAddr string) (DiscordBot, error) {
	var (
		webProxyURL *url.URL
		err         error
	)
	if webProxyAddr != "" {
		if webProxyURL, err = lib.AddrToURL(webProxyAddr); err != nil {
			return DiscordBot{}, trace.Wrap(err)
		}
	}

	token := "Bearer " + conf.Slack.Token
	if conf.Slack.IsDiscord {
		token = "SlackBot " + conf.Slack.Token
	}

	client := resty.
		NewWithClient(&http.Client{
			Timeout: discordHTTPTimeout,
			Transport: &http.Transport{
				MaxConnsPerHost:     discordMaxConns,
				MaxIdleConnsPerHost: discordMaxConns,
			},
		}).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", token)

	// APIURL parameter is set only in tests
	if endpoint := conf.Slack.APIURL; endpoint != "" {
		client.SetHostURL(endpoint)
	} else {
		client.SetHostURL("https://discord.com/api/")
		client.OnAfterResponse(onAfterResponseDiscord)
	}

	return DiscordBot{
		client:      client,
		clusterName: clusterName,
		webProxyURL: webProxyURL,
	}, nil
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

func (b DiscordBot) HealthCheck(ctx context.Context) error {
	_, err := b.client.NewRequest().
		SetContext(ctx).
		Get("/users/@me")
	if err != nil {
		return trace.Wrap(err, "health check failed, probably invalid token")
	}

	return nil
}

// Broadcast posts request info to Slack with action buttons.
func (b DiscordBot) Broadcast(ctx context.Context, channels []string, reqID string, reqData pd.AccessRequestData) (SlackData, error) {
	var data SlackData
	var errors []error

	for _, channel := range channels {
		var result ChatMsgResponse
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

	}

	return data, trace.NewAggregate(errors...)
}

// PostReviewReply does nothing as Discord does not have threaded replies
func (b DiscordBot) PostReviewReply(ctx context.Context, channelID, timestamp string, review types.AccessReview) error {
	return nil
}

// LookupDirectChannelByEmail fetches user's id by email.
func (b DiscordBot) LookupDirectChannelByEmail(ctx context.Context, email string) (string, error) {
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
func (b DiscordBot) UpdateMessages(ctx context.Context, reqID string, reqData pd.AccessRequestData, messagingData SlackData, reviews []types.AccessReview) error {
	var errors []error
	for _, msg := range messagingData {
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(DiscordMsg{Msg: Msg{Channel: msg.ChannelID}, Text: b.discordMsgText(reqID, reqData, reviews)}).
			Patch("/channels/" + msg.ChannelID + "/messages/" + msg.TimestampOrDiscordID)
		if err != nil {
			errors = append(errors, trace.Wrap(err))
		}
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

func (b DiscordBot) discordMsgText(reqID string, reqData pd.AccessRequestData, reviews []types.AccessReview) string {
	return "You have a new Role Request:\n" +
		msgFields(reqID, reqData, b.webProxyURL) +
		b.msgDiscordReviews(reviews) +
		msgStatusText(reqData.ResolutionTag, reqData.ResolutionReason)
}

func (b DiscordBot) msgDiscordReviews(reviews []types.AccessReview) string {
	var result = ""

	// TODO: Update error propagation
	for _, review := range reviews {
		text, err := msgReview(review)
		if err != nil {
			return ""
		}

		result += text + "\n"
	}

	return "\n" + result
}
