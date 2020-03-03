package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	jira "gopkg.in/andygrunwald/go-jira.v1"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	RequestIdPropertyKey = "teleportAccessRequestId"

	jiraMaxConns    = 100
	jiraHttpTimeout = 10 * time.Second
)

// Bot is a wrapper around jira.Client that works with access.Request
type Bot struct {
	client      JiraClient
	project     string
	clusterName string
}

type BotIssue Issue

type BotIssueUpdate struct {
	Status string
	Author jira.User
}

func (issue *BotIssue) GetRequestID() (string, error) {
	reqID, ok := issue.Properties[RequestIdPropertyKey].(string)
	if !ok {
		return "", trace.Errorf("got non-string '%s' field", RequestIdPropertyKey)
	}
	return reqID, nil
}

func (issue *BotIssue) GetLastUpdate(status string) (BotIssueUpdate, error) {
	changelog := issue.Changelog
	if changelog == nil {
		return BotIssueUpdate{}, trace.Errorf("changelog is missing in API response")
	}

	var update *BotIssueUpdate
	for _, entry := range changelog.Histories {
		for _, item := range entry.Items {
			if item.FieldType == "jira" && item.Field == "status" && strings.ToLower(item.ToString) == status {
				update = &BotIssueUpdate{
					Status: status,
					Author: entry.Author,
				}
				break
			}
		}
		if update != nil {
			break
		}
	}
	if update == nil {
		return BotIssueUpdate{}, trace.Errorf("cannot find a %q status update in changelog", status)
	}
	return *update, nil
}

func (issue *BotIssue) GetTransition(status string) (jira.Transition, error) {
	for _, transition := range issue.Transitions {
		if strings.ToLower(transition.To.Name) == status {
			return transition, nil
		}
	}
	return jira.Transition{}, trace.Errorf("cannot find a %q status among possible transitions", status)
}

func NewBot(conf *Config) (*Bot, error) {
	transport := jira.BasicAuthTransport{
		Username: conf.JIRA.Username,
		Password: conf.JIRA.APIToken,
		Transport: &http.Transport{
			MaxConnsPerHost:     jiraMaxConns,
			MaxIdleConnsPerHost: jiraMaxConns,
		},
	}
	httpClient := transport.Client()
	httpClient.Timeout = jiraHttpTimeout

	client, err := jira.NewClient(httpClient, conf.JIRA.URL)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Bot{
		client:  JiraClient{client},
		project: conf.JIRA.Project,
	}, nil
}

func (b *Bot) HealthCheck(ctx context.Context) error {
	log.Info("Starting JIRA API health check...")
	req, err := b.client.NewRequest(ctx, "GET", "rest/api/2/myself", nil)
	if err != nil {
		return trace.Wrap(err)
	}
	resp, err := b.client.Do(req, nil)
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			if resp.StatusCode == 404 {
				return trace.AccessDenied("got %s from API endpoint, perhaps JIRA URL is not configured well", resp.Status)
			}
			if resp.StatusCode == 403 || resp.StatusCode == 401 {
				return trace.AccessDenied("got %s from API endpoint, perhaps JIRA credentials are not configured well", resp.Status)
			}
		}
		return trace.Wrap(err)
	}
	if resp != nil {
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			return trace.AccessDenied("got non-json response from API endpoint, perhaps JIRA URL is not configured well")
		}
	}

	log.Info("Checking out JIRA project...")
	project, err := b.client.GetProject(ctx, b.project)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Found project %q: %q", project.Key, project.Name)

	log.Info("Checking out JIRA project permissions...")
	permissions, err := b.client.GetMyPermissions(ctx, &GetMyPermissionsQueryOptions{
		ProjectKey:  b.project,
		Permissions: []string{"BROWSE_PROJECTS", "CREATE_ISSUES"},
	})
	if err != nil {
		return trace.Wrap(err)
	}
	if !permissions.Permissions["BROWSE_PROJECTS"].HavePermission {
		return trace.Errorf("bot user does not have BROWSE_PROJECTS permission")
	}
	if !permissions.Permissions["CREATE_ISSUES"].HavePermission {
		return trace.Errorf("bot user does not have CREATE_ISSUES permission")
	}

	log.Info("JIRA API health check finished ok")
	return nil
}

// CreateIssue creates an issue with "Pending" status
func (b *Bot) CreateIssue(ctx context.Context, reqID string, reqData requestData) (data jiraData, err error) {
	issue, err := b.client.CreateIssue(ctx, &IssueInput{
		Properties: []jira.EntityProperty{
			jira.EntityProperty{
				Key:   RequestIdPropertyKey,
				Value: reqID,
			},
		},
		Fields: &jira.IssueFields{
			Type:    jira.IssueType{Name: "Task"},
			Project: jira.Project{Key: b.project},
			Summary: fmt.Sprintf("Incoming request %s", reqID),
		},
	})
	if err = trace.Wrap(err); err != nil {
		return
	}

	data.ID = issue.ID
	data.Key = issue.Key
	return
}

// GetIssue loads the issue with all necessary nested data.
func (b *Bot) GetIssue(ctx context.Context, id string) (*BotIssue, error) {
	jiraIssue, err := b.client.GetIssue(ctx, id, &jira.GetQueryOptions{
		Expand:     "changelog,transitions",
		Properties: RequestIdPropertyKey,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	issue := BotIssue(*jiraIssue)
	return &issue, nil
}

// ExpireIssue sets "Expired" status to an issue.
func (b *Bot) ExpireIssue(ctx context.Context, reqID string, reqData requestData, jiraData jiraData) error {
	issue, err := b.GetIssue(ctx, jiraData.ID)
	if err != nil {
		return trace.Wrap(err)
	}

	transition, err := issue.GetTransition("expired")
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(b.client.TransitionIssue(ctx, issue.ID, transition.ID))
}
