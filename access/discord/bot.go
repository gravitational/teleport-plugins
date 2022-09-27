/*
Copyright 2022 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"github.com/gravitational/teleport-plugins/access/common"
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
// takes DiscordConfig as an argument.
func NewDiscordBot(conf DiscordConfig, clusterName, webProxyAddr string) (common.MessagingBot, error) {
	var (
		webProxyURL *url.URL
		err         error
	)
	if webProxyAddr != "" {
		if webProxyURL, err = lib.AddrToURL(webProxyAddr); err != nil {
			return DiscordBot{}, trace.Wrap(err)
		}
	}

	token := "Bot " + conf.Discord.Token

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
	if endpoint := conf.Discord.APIURL; endpoint != "" {
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

func (b DiscordBot) CheckHealth(ctx context.Context) error {
	_, err := b.client.NewRequest().
		SetContext(ctx).
		Get("/users/@me")
	if err != nil {
		return trace.Wrap(err, "health check failed, probably invalid token")
	}

	return nil
}

// Broadcast posts request info to Discord.
func (b DiscordBot) Broadcast(ctx context.Context, recipients []common.Recipient, reqID string, reqData pd.AccessRequestData) (common.SentMessages, error) {
	var data common.SentMessages
	var errors []error

	for _, recipient := range recipients {
		var result ChatMsgResponse
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(DiscordMsg{Msg: Msg{Channel: recipient.ID}, Text: b.discordMsgText(reqID, reqData, nil)}).
			SetResult(&result).
			Post("/channels/" + recipient.ID + "/messages")
		if err != nil {
			errors = append(errors, trace.Wrap(err))
			continue
		}
		data = append(data, common.MessageData{ChannelID: recipient.ID, MessageID: result.DiscordID})

	}

	return data, trace.NewAggregate(errors...)
}

// PostReviewReply does nothing as Discord does not have threaded replies
func (b DiscordBot) PostReviewReply(ctx context.Context, channelID, timestamp string, review types.AccessReview) error {
	return nil
}

// Expire updates request's Slack post with EXPIRED status and removes action buttons.
func (b DiscordBot) UpdateMessages(ctx context.Context, reqID string, reqData pd.AccessRequestData, messagingData common.SentMessages, reviews []types.AccessReview) error {
	var errors []error
	for _, msg := range messagingData {
		_, err := b.client.NewRequest().
			SetContext(ctx).
			SetBody(DiscordMsg{Msg: Msg{Channel: msg.ChannelID}, Text: b.discordMsgText(reqID, reqData, reviews)}).
			Patch("/channels/" + msg.ChannelID + "/messages/" + msg.MessageID)
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
		common.MsgFields(reqID, reqData, b.clusterName, b.webProxyURL) +
		b.msgDiscordReviews(reviews) +
		common.MsgStatusText(reqData.ResolutionTag, reqData.ResolutionReason)
}

func (b DiscordBot) msgDiscordReviews(reviews []types.AccessReview) string {
	var result = ""

	// TODO: Update error propagation
	for _, review := range reviews {
		text, err := common.MsgReview(review)
		if err != nil {
			return ""
		}

		result += text + "\n"
	}

	return "\n" + result
}

func (b DiscordBot) FetchRecipient(ctx context.Context, recipient string) (*common.Recipient, error) {
	// Discord does not support resolving email address, we only return the channel name
	// TODO: check if channel exists ?
	return &common.Recipient{
		Name: recipient,
		ID:   recipient,
		Kind: "Channel",
		Data: nil,
	}, nil
}
