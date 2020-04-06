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
	client    *pd.Client
	server    *WebhookServer
	from      string
	serviceId string

	clusterName string
}

func NewBot(conf *Config, onAction WebhookFunc) (*Bot, error) {
	client := pd.NewClient(conf.Pagerduty.APIKey)
	client.HTTPClient = &http.Client{
		Timeout: pdHttpTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     pdMaxConns,
			MaxIdleConnsPerHost: pdMaxConns,
		},
	}
	bot := &Bot{
		from:      conf.Pagerduty.UserEmail,
		client:    client,
		serviceId: conf.Pagerduty.ServiceId,
	}
	bot.server = NewWebhookServer(conf.HTTP, onAction)
	return bot, nil

}

func (b *Bot) RunServer(ctx context.Context) error {
	return b.server.Run(ctx)
}

func (b *Bot) ShutdownServer(ctx context.Context) error {
	return b.server.Shutdown(ctx)
}

func (b *Bot) Setup() error {
	var more bool
	var offset uint

	var webhookSchemaID string
	for offset, more = 0, true; webhookSchemaID == "" && more; {
		schemaResp, err := b.client.ListExtensionSchemas(pd.ListExtensionSchemaOptions{
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
		return trace.NotFound(`Failed to find "Custom Incident Action" extension type`)
	}

	var approveExtID, denyExtID string
	for offset, more = 0, true; (approveExtID == "" || denyExtID == "") && more; {
		extResp, err := b.client.ListExtensions(pd.ListExtensionOptions{
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

	if err := b.SetupCustomAction(approveExtID, webhookSchemaID, pdApproveAction, pdApproveActionLabel); err != nil {
		return err
	}
	if err := b.SetupCustomAction(denyExtID, webhookSchemaID, pdDenyAction, pdDenyActionLabel); err != nil {
		return err
	}

	return nil
}

func (b *Bot) SetupCustomAction(extensionId, schemaId, actionName, actionLabel string) error {
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
		_, err := b.client.CreateExtension(ext)
		return trace.Wrap(err)
	} else {
		_, err := b.client.UpdateExtension(extensionId, ext)
		return trace.Wrap(err)
	}
}

func (b *Bot) CreateIncident(reqID string, reqData RequestData) (PagerdutyData, error) {
	incident, err := b.client.CreateIncident(b.from, &pd.CreateIncidentOptions{
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

func (b *Bot) ResolveIncident(reqID string, pdData PagerdutyData, status string) error {
	err := b.client.CreateIncidentNote(pdData.ID, pd.IncidentNote{
		User: pd.APIObject{
			Summary: b.from,
		},
		Content: fmt.Sprintf("Access request has been %s", status),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = b.client.ManageIncidents(b.from, []pd.ManageIncidentsOptions{
		pd.ManageIncidentsOptions{
			ID:     pdData.ID,
			Type:   "incident_reference",
			Status: "resolved",
		},
	})
	return trace.Wrap(err)
}
