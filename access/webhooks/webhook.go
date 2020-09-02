package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/trace"
)

const webhookMaxConnections = 100
const webhookHTTPTimeout = 10 * time.Second

// WebhookClient represents a Webhook sender component of teleport-webooks
// that is responsible for sending webhooks on access request events.
type WebhookClient struct {
	client      *http.Client
	url         string
	notifyOnly  bool
	clusterName string
}

// Payload is a webhook payload. The webhook assembles it and then serializes
// it into JSON and then sends it to the destination.
type Payload struct {
	ClusterName string
	RequestID   string
	User        string
	Roles       []string
	CreatedAt   time.Time
	StateNum    access.State
	StateName   string
}

// NewWebhookClient initializes and returns a new webhook client
func NewWebhookClient(conf WebhookConfig) *WebhookClient {
	return &WebhookClient{
		client: &http.Client{
			Timeout: webhookHTTPTimeout,
			Transport: &http.Transport{
				MaxConnsPerHost:     webhookMaxConnections,
				MaxIdleConnsPerHost: webhookMaxConnections,
			},
		},
		notifyOnly: conf.NotifyOnly,
		url:        conf.URL,
	}
}

// stateString converts an access.State to it's string representation
func stateString(state access.State) string {
	return [...]string{"", "Pending", "Approved", "Denied"}[state]
}

// makeRequestBody builds a Payload and then serializes it to JSON,
// then returns it as a request body for net.http to use.
func (c *WebhookClient) makeRequestBody(req access.Request) ([]byte, error) {

	payload := Payload{
		ClusterName: c.clusterName,
		RequestID:   req.ID,
		User:        req.User,
		Roles:       req.Roles,
		CreatedAt:   req.Created,
		StateNum:    req.State,
		StateName:   stateString(req.State),
	}

	return json.Marshal(payload)
}

func (c *WebhookClient) sendWebhook(ctx context.Context, req access.Request) error {
	body, err := c.makeRequestBody(req)
	if err != nil {
		return trace.Wrap(err, "failed to serialize request block: %v", err)
	}

	request, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return trace.Wrap(err, "failed to send the webhook request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return trace.Errorf("Webhook request returned non-200 status code: %v", response)
	}
	return nil
}
