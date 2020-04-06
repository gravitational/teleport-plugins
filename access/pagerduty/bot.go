package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"

	"github.com/gravitational/trace"
	// log "github.com/sirupsen/logrus"
)

const (
	pdMaxConns    = 100
	pdHttpTimeout = 10 * time.Second
	pdListLimit   = uint(60)

	pdIncidentKeyPrefix  = "teleport-access-request"
	pdApproveAction      = "approve"
	pdApproveActionLabel = "Approve Request"
	pdDenyAction         = "deny"
	pdDenyActionLabel    = "Deny Request"
)

// Bot is a wrapper around pd.Client that works with access.Request
type Bot struct {
	httpClient  *http.Client
	apiEndpoint string
	apiKey      string
	server      *WebhookServer
	from        string
	serviceId   string

	clusterName string
}

type HTTPClientImpl func(*http.Request) (*http.Response, error)

func (h HTTPClientImpl) Do(req *http.Request) (*http.Response, error) {
	return h(req)
}

func NewBot(conf *Config, onAction WebhookFunc) (*Bot, error) {
	httpClient := &http.Client{
		Timeout: pdHttpTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     pdMaxConns,
			MaxIdleConnsPerHost: pdMaxConns,
		},
	}
	bot := &Bot{
		httpClient:  httpClient,
		apiEndpoint: conf.Pagerduty.APIEndpoint,
		apiKey:      conf.Pagerduty.APIKey,
		from:        conf.Pagerduty.UserEmail,
		serviceId:   conf.Pagerduty.ServiceId,
	}
	bot.server = NewWebhookServer(conf.HTTP, onAction)
	return bot, nil

}

func (b *Bot) NewClient(ctx context.Context) *pd.Client {
	clientOpts := []pd.ClientOptions{}
	// apiEndpoint parameter is set only in tests
	if b.apiEndpoint != "" {
		clientOpts = append(clientOpts, pd.WithAPIEndpoint(b.apiEndpoint))
	}
	client := pd.NewClient(b.apiKey, clientOpts...)
	client.HTTPClient = HTTPClientImpl(func(r *http.Request) (*http.Response, error) {
		return b.httpClient.Do(r.WithContext(ctx))
	})
	return client
}

func (b *Bot) RunServer(ctx context.Context) error {
	return b.server.Run(ctx)
}

func (b *Bot) ShutdownServer(ctx context.Context) error {
	return b.server.Shutdown(ctx)
}

func (b *Bot) Setup(ctx context.Context) error {
	client := b.NewClient(ctx)

	var more bool
	var offset uint

	var webhookSchemaID string
	for offset, more = 0, true; webhookSchemaID == "" && more; {
		schemaResp, err := client.ListExtensionSchemas(pd.ListExtensionSchemaOptions{
			APIListObject: pd.APIListObject{
				Offset: offset,
				Limit:  pdListLimit,
			},
		})
		if err != nil {
			return trace.Wrap(err)
		}

		for _, schema := range schemaResp.ExtensionSchemas {
			if schema.Key == "custom_webhook" {
				webhookSchemaID = schema.ID
			}
		}

		more = schemaResp.More
		offset += pdListLimit
	}
	if webhookSchemaID == "" {
		return trace.NotFound(`failed to find "Custom Incident Action" extension type`)
	}

	var approveExtID, denyExtID string
	for offset, more = 0, true; (approveExtID == "" || denyExtID == "") && more; {
		extResp, err := client.ListExtensions(pd.ListExtensionOptions{
			APIListObject: pd.APIListObject{
				Offset: offset,
				Limit:  pdListLimit,
			},
			ExtensionObjectID: b.serviceId,
			ExtensionSchemaID: webhookSchemaID,
		})
		if err != nil {
			return trace.Wrap(err)
		}

		for _, ext := range extResp.Extensions {
			if ext.Name == pdApproveActionLabel {
				approveExtID = ext.ID
			}
			if ext.Name == pdDenyActionLabel {
				denyExtID = ext.ID
			}
		}

		more = extResp.More
		offset += pdListLimit
	}

	if err := b.setupCustomAction(client, approveExtID, webhookSchemaID, pdApproveAction, pdApproveActionLabel); err != nil {
		return err
	}
	if err := b.setupCustomAction(client, denyExtID, webhookSchemaID, pdDenyAction, pdDenyActionLabel); err != nil {
		return err
	}

	return nil
}

func (b *Bot) setupCustomAction(client *pd.Client, extensionId, schemaId, actionName, actionLabel string) error {
	actionURL, err := b.server.ActionURL(actionName)
	if err != nil {
		return trace.Wrap(err)
	}

	ext := &pd.Extension{
		Name:        actionLabel,
		EndpointURL: actionURL,
		ExtensionSchema: pd.APIObject{
			Type: "extension_schema_reference",
			ID:   schemaId,
		},
		ExtensionObjects: []pd.APIObject{
			pd.APIObject{
				Type: "service_reference",
				ID:   b.serviceId,
			},
		},
	}
	if extensionId == "" {
		_, err := client.CreateExtension(ext)
		return trace.Wrap(err)
	} else {
		_, err := client.UpdateExtension(extensionId, ext)
		return trace.Wrap(err)
	}
}

func (b *Bot) CreateIncident(ctx context.Context, reqID string, reqData RequestData) (PagerdutyData, error) {
	client := b.NewClient(ctx)

	incident, err := client.CreateIncident(b.from, &pd.CreateIncidentOptions{
		Title:       fmt.Sprintf("Access request from %s", reqData.User),
		IncidentKey: fmt.Sprintf("%s/%s", pdIncidentKeyPrefix, reqID),
		Service: &pd.APIReference{
			Type: "service_reference",
			ID:   b.serviceId,
		},
		Body: &pd.APIDetails{
			Type:    "incident_body",
			Details: fmt.Sprintf("TODO %s", reqID),
		},
	})
	if err != nil {
		return PagerdutyData{}, trace.Wrap(err)
	}

	return PagerdutyData{
		ID: incident.Id, // Yes, due to strange implementation, it's called `Id` which overrides `APIObject.ID`.
	}, nil
}

func (b *Bot) ResolveIncident(ctx context.Context, reqID string, pdData PagerdutyData, status string) error {
	client := b.NewClient(ctx)

	err := client.CreateIncidentNote(pdData.ID, pd.IncidentNote{
		User: pd.APIObject{
			Summary: b.from,
		},
		Content: fmt.Sprintf("Access request has been %s", status),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = client.ManageIncidents(b.from, []pd.ManageIncidentsOptions{
		pd.ManageIncidentsOptions{
			ID:     pdData.ID,
			Type:   "incident_reference",
			Status: "resolved",
		},
	})
	return trace.Wrap(err)
}
