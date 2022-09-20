package main

import (
	pd "github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
	"golang.org/x/net/context"
)

type MessagingBot interface {
	// HealthCheck Checks if the bot can connect to its messaging service
	HealthCheck(ctx context.Context) error
	// Broadcast sends an access request message to a list of channels
	Broadcast(ctx context.Context, channels []string, reqID string, reqData pd.AccessRequestData) (data SlackData, err error)
	// PostReviewReply posts in thread an access request review. This does nothing if the messageing service
	// does not support threaded replies.
	PostReviewReply(ctx context.Context, channelID string, threadID string, review types.AccessReview) error
	// LookupDirectChannelByEmail takes a user email and returns the associated direct channel
	LookupDirectChannelByEmail(ctx context.Context, email string) (string, error)
	// UpdateMessages updates access request messages that were previously sent via Broadcast
	// This is used to change the access reaquest status and number of required approval remaining
	UpdateMessages(ctx context.Context, reqID string, data pd.AccessRequestData, slackData SlackData, reviews []types.AccessReview) error
	// TODO: add LookupChannel method
}
