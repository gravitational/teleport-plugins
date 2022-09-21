package common

import (
	pd "github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
	"golang.org/x/net/context"
)

type MessagingBot interface {
	// HealthCheck Checks if the bot can connect to its messaging service
	HealthCheck(ctx context.Context) error
	// Broadcast sends an access request message to a list of channels
	Broadcast(ctx context.Context, channels []string, reqID string, reqData pd.AccessRequestData) (data SentMessages, err error)
	// PostReviewReply posts in thread an access request review. This does nothing if the messageing service
	// does not support threaded replies.
	PostReviewReply(ctx context.Context, channelID string, threadID string, review types.AccessReview) error
	// UpdateMessages updates access request messages that were previously sent via Broadcast
	// This is used to change the access reaquest status and number of required approval remaining
	UpdateMessages(ctx context.Context, reqID string, data pd.AccessRequestData, slackData SentMessages, reviews []types.AccessReview) error
	// FetchRecipient fetches recipient data from the messaging service API. It can also be used to
	FetchRecipient(ctx context.Context, recipient string) (*Recipient, error)
}

type Recipient struct {
	// Name is the original string that was passed to create the recipient. This can be an id, email, channel name
	// URL, ... This is the user input (through suggested reviewers or plugin configuration)
	Name string
	// ID is the Id the Name was resolved to. It represents the recipient from the messaging service point of view.
	// e.g. if Name is a Slack user email address, ID will be the Slack user id.
	ID string
	// Kind is the recipient kind inferred from the Recipient Name. This is a messaging service concept, most common
	// values are "User" or "Channel".
	Kind string
	// Data allows MessagingBot to store required data for the recipient
	Data interface{}
}

type BotFactory[T PluginConfiguration] func(T, string, string) (MessagingBot, error)
