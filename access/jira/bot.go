package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/go-querystring/query"

	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

const (
	RequestIDPropertyKey = "teleportAccessRequestId"

	jiraMaxConns    = 100
	jiraHTTPTimeout = 10 * time.Second
)

// Bot is a wrapper around jira.Client that works with access.Request
type Bot struct {
	client      *resty.Client
	project     string
	issueType   string
	clusterName string
}

type BotIssue Issue

type BotIssueUpdate struct {
	Status string
	Author UserDetails
}

var descriptionTemplate *template.Template

func init() {
	var err error
	descriptionTemplate, err = template.New("description").Parse(`User *{{.User}}* requested an access on *{{.Created.Format .TimeFormat}}* with the following roles:
{{range .Roles}}
* {{ . }}
{{end}}
{{if .RequestReason}}
Reason: *{{.RequestReason}}*
{{end}}
Request ID: *{{.ID}}*
`)
	if err != nil {
		panic(err)
	}
}

func (issue BotIssue) GetRequestID() (string, error) {
	reqID, ok := issue.Properties[RequestIDPropertyKey].(string)
	if !ok {
		return "", trace.Errorf("got non-string %q field", RequestIDPropertyKey)
	}
	return reqID, nil
}

func (issue BotIssue) GetLastUpdate(status string) (BotIssueUpdate, error) {
	changelog := issue.Changelog
	if len(changelog.Histories) == 0 {
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

func (issue BotIssue) GetTransition(status string) (IssueTransition, error) {
	for _, transition := range issue.Transitions {
		if strings.ToLower(transition.To.Name) == status {
			return transition, nil
		}
	}
	return IssueTransition{}, trace.Errorf("cannot find a %q status among possible transitions", status)
}

func NewBot(conf JIRAConfig) *Bot {
	client := resty.NewWithClient(&http.Client{
		Timeout: jiraHTTPTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     jiraMaxConns,
			MaxIdleConnsPerHost: jiraMaxConns,
		},
	})
	client.SetHostURL(conf.URL)
	client.SetBasicAuth(conf.Username, conf.APIToken)
	client.SetHeader("Content-Type", "application/json")
	client.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		req.SetError(&ErrorResult{})
		return nil
	})
	client.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
		if resp.IsError() {
			switch result := resp.Error().(type) {
			case *ErrorResult:
				return trace.Errorf("http error code=%v, errors=[%v]", resp.StatusCode(), strings.Join(result.ErrorMessages, ", "))
			case nil:
				return nil
			default:
				return trace.Errorf("unknown error result %#v", result)
			}
		}
		return nil
	})
	return &Bot{client: client, project: conf.Project, issueType: conf.IssueType}
}

func (b *Bot) HealthCheck(ctx context.Context) error {
	log := logger.Get(ctx)
	var emptyError *ErrorResult
	resp, err := b.client.NewRequest().
		SetContext(ctx).
		SetError(emptyError).
		Get("rest/api/2/myself")
	if err != nil {
		return trace.Wrap(err)
	}
	if !strings.HasPrefix(resp.Header().Get("Content-Type"), "application/json") {
		return trace.AccessDenied("got non-json response from API endpoint, perhaps JIRA URL is not configured well")
	}
	if resp.IsError() {
		if resp.StatusCode() == 404 {
			return trace.AccessDenied("got %s from API endpoint, perhaps JIRA URL is not configured well", resp.Status())
		} else if resp.StatusCode() == 403 || resp.StatusCode() == 401 {
			return trace.AccessDenied("got %s from API endpoint, perhaps JIRA credentials are not configured well", resp.Status())
		} else {
			return trace.AccessDenied("got %s from API endpoint", resp.Status())
		}
	}

	log.Debug("Checking out JIRA project...")
	var project Project
	_, err = b.client.NewRequest().
		SetContext(ctx).
		SetPathParams(map[string]string{"projectID": b.project}).
		SetResult(&project).
		Get("rest/api/2/project/{projectID}")
	if err != nil {
		return trace.Wrap(err)
	}
	log.Debugf("Found project %q: %q", project.Key, project.Name)

	log.Debug("Checking out JIRA project permissions...")
	queryOptions, err := query.Values(GetMyPermissionsQueryOptions{
		ProjectKey:  b.project,
		Permissions: []string{"BROWSE_PROJECTS", "CREATE_ISSUES"},
	})
	if err != nil {
		return trace.Wrap(err)
	}
	var permissions Permissions
	_, err = b.client.NewRequest().
		SetContext(ctx).
		SetQueryParamsFromValues(queryOptions).
		SetResult(&permissions).
		Get("rest/api/2/mypermissions")
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

	input := IssueInput{
		Properties: []EntityProperty{
			{
				Key:   RequestIDPropertyKey,
				Value: reqID,
			},
		},
		Fields: IssueFieldsInput{
			Type:        &IssueType{Name: b.issueType},
			Project:     &Project{Key: b.project},
			Summary:     fmt.Sprintf("%s requested %s", reqData.User, strings.Join(reqData.Roles, ", ")),
			Description: description,
		},
	}
	var issue CreatedIssue
	_, err = b.client.NewRequest().
		SetContext(ctx).
		SetBody(&input).
		SetResult(&issue).
		Post("rest/api/2/issue")
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
func (b *Bot) GetIssue(ctx context.Context, id string) (BotIssue, error) {
	queryOptions, err := query.Values(GetIssueQueryOptions{
		Fields:     []string{"status", "comment"},
		Expand:     []string{"changelog", "transitions"},
		Properties: []string{RequestIDPropertyKey},
	})
	if err != nil {
		return BotIssue{}, trace.Wrap(err)
	}
	var jiraIssue Issue
	_, err = b.client.NewRequest().
		SetContext(ctx).
		SetPathParams(map[string]string{"issueID": id}).
		SetQueryParamsFromValues(queryOptions).
		SetResult(&jiraIssue).
		Get("rest/api/2/issue/{issueID}")
	if err != nil {
		return BotIssue{}, trace.Wrap(err)
	}

	return BotIssue(jiraIssue), nil
}

func (b *Bot) RangeIssueCommentsDescending(ctx context.Context, id string, fn func(PageOfComments) bool) error {
	startAt := 0
	for {
		queryOptions, err := query.Values(GetIssueCommentQueryOptions{
			StartAt: startAt,
			OrderBy: "-created",
		})
		if err != nil {
			return trace.Wrap(err)
		}

		var pageOfComments PageOfComments
		_, err = b.client.NewRequest().
			SetContext(ctx).
			SetPathParams(map[string]string{"issueID": id}).
			SetQueryParamsFromValues(queryOptions).
			SetResult(&pageOfComments).
			Get("rest/api/2/issue/{issueID}/comment")
		if err != nil {
			return trace.Wrap(err)
		}

		nComments := len(pageOfComments.Comments)

		if nComments == 0 {
			break
		}

		if !fn(pageOfComments) {
			break
		}

		if nComments < pageOfComments.MaxResults {
			break
		}

		startAt = startAt + nComments
	}

	return nil
}

func (b *Bot) TransitionIssue(ctx context.Context, issueID, transitionID string) error {
	payload := IssueTransitionInput{
		Transition: IssueTransition{
			ID: transitionID,
		},
	}
	_, err := b.client.NewRequest().
		SetContext(ctx).
		SetPathParams(map[string]string{"issueID": issueID}).
		SetBody(&payload).
		Post("rest/api/2/issue/{issueID}/transitions")
	return trace.Wrap(err)
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

	return trace.Wrap(b.TransitionIssue(ctx, issue.ID, transition.ID))
}
