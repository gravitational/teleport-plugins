package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/andygrunwald/go-jira"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	RequestIDPropertyKey = "teleportAccessRequestId"

	jiraMaxConns    = 100
	jiraHTTPTimeout = 10 * time.Second
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

var descriptionTemplate *template.Template

func init() {
	var err error
	descriptionTemplate, err = template.New("description").Parse(`User *{{.User}}* requested an access on *{{.Created.Format .TimeFormat}}* with the following roles:
{{range .Roles}}
* {{ . }}
{{end}}
{{if .RequestReason}}
Request Reason: *{{.RequestReason}}*
{{else}}
No Request Reason provided.
{{end}}
Request ID: *{{.ID}}*
`)
	if err != nil {
		panic(err)
	}
}

func (issue *BotIssue) GetRequestID() (string, error) {
	reqID, ok := issue.Properties[RequestIDPropertyKey].(string)
	if !ok {
		return "", trace.Errorf("got non-string %q field", RequestIDPropertyKey)
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

func NewBot(conf JIRAConfig) (*Bot, error) {
	transport := jira.BasicAuthTransport{
		Username: conf.Username,
		Password: conf.APIToken,
		Transport: &http.Transport{
			MaxConnsPerHost:     jiraMaxConns,
			MaxIdleConnsPerHost: jiraMaxConns,
		},
	}
	httpClient := transport.Client()
	httpClient.Timeout = jiraHTTPTimeout

	client, err := jira.NewClient(httpClient, conf.URL)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Bot{
		client:  JiraClient{client},
		project: conf.Project,
	}, nil
}

func (b *Bot) HealthCheck(ctx context.Context) error {
	req, err := b.client.NewRequest(ctx, http.MethodGet, "rest/api/2/myself", nil)
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

	log.Debug("Checking out JIRA project...")
	project, err := b.client.GetProject(ctx, b.project)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Debugf("Found project %q: %q", project.Key, project.Name)

	log.Debug("Checking out JIRA project permissions...")
	permissions, err := b.client.GetMyPermissions(ctx, &GetMyPermissionsQueryOptions{
		ProjectKey:  b.project,
		Permissions: []string{"BROWSE_PROJECTS", "CREATE_ISSUES"},
	})
	if err != nil {
		return trace.Wrap(err)
	}
	if !permissions.Permissions["BROWSE_PROJECTS"].HavePermission {
		return trace.AccessDenied("bot user does not have BROWSE_PROJECTS permission")
	}
	if !permissions.Permissions["CREATE_ISSUES"].HavePermission {
		return trace.AccessDenied("bot user does not have CREATE_ISSUES permission")
	}

	return nil
}

// CreateIssue creates an issue with "Pending" status
func (b *Bot) CreateIssue(ctx context.Context, reqID string, reqData RequestData) (JiraData, error) {
	description, err := b.buildIssueDescription(reqID, reqData)
	if err != nil {
		return JiraData{}, trace.Wrap(err)
	}

	issue, err := b.client.CreateIssue(ctx, &IssueInput{
		Properties: []jira.EntityProperty{
			{
				Key:   RequestIDPropertyKey,
				Value: reqID,
			},
		},
		Fields: &jira.IssueFields{
			Type:        jira.IssueType{Name: "Task"},
			Project:     jira.Project{Key: b.project},
			Summary:     fmt.Sprintf("%s requested %s", reqData.User, strings.Join(reqData.Roles, ", ")),
			Description: description,
		},
	})
	if err != nil {
		return JiraData{}, trace.Wrap(err)
	}

	return JiraData{
		ID:  issue.ID,
		Key: issue.Key,
	}, nil
}

func (b *Bot) buildIssueDescription(reqID string, reqData RequestData) (string, error) {
	var builder strings.Builder
	err := descriptionTemplate.Execute(&builder, struct {
		ID         string
		TimeFormat string
		RequestData
	}{
		reqID,
		time.RFC822,
		reqData,
	})
	if err != nil {
		return "", trace.Wrap(err)
	}
	return builder.String(), nil
}

// GetIssue loads the issue with all necessary nested data.
func (b *Bot) GetIssue(ctx context.Context, id string) (*BotIssue, error) {
	jiraIssue, err := b.client.GetIssue(ctx, id, &jira.GetQueryOptions{
		Expand:     "changelog,transitions",
		Properties: RequestIDPropertyKey,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	issue := BotIssue(*jiraIssue)
	return &issue, nil
}

// ExpireIssue sets "Expired" status to an issue.
func (b *Bot) ExpireIssue(ctx context.Context, reqID string, reqData RequestData, jiraData JiraData) error {
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
