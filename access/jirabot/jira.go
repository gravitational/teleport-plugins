package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gravitational/trace"
	jira "gopkg.in/andygrunwald/go-jira.v1"
)

// JiraClient is a convenient wrapper around jira.Client.
type JiraClient struct {
	client *jira.Client
}

type GetMyPermissionsQueryOptions struct {
	ProjectKey  string   `url:"projectKey,omitempty"`
	Permissions []string `url:"permissions,comma,omitempty"`
}

type Permission struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	HavePermission bool   `json:"havePermission"`
}

type Permissions struct {
	Permissions map[string]Permission `json:"permissions"`
}

// IssueInput represents an issue input parameters for create/update operations.
type IssueInput struct {
	Fields     *jira.IssueFields     `json:"fields,omitempty"`
	Properties []jira.EntityProperty `json:"properties,omitempty"`
}

// CreatedIssue represents a response of issue creation.
type CreatedIssue struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

type Issue struct {
	Expand         string                    `json:"expand"`
	ID             string                    `json:"id"`
	Self           string                    `json:"self"`
	Key            string                    `json:"key"`
	Fields         *jira.IssueFields         `json:"fields"`
	RenderedFields *jira.IssueRenderedFields `json:"renderedFields"`
	Changelog      *jira.Changelog           `json:"changelog"`
	Properties     map[string]interface{}    `json:"properties"`
	Transitions    []jira.Transition         `json:"transitions"`
}

func (c *JiraClient) NewRequest(ctx context.Context, method, url string, body interface{}, options ...func(*http.Request) error) (*http.Request, error) {
	req, err := c.client.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	for _, option := range options {
		err = option(req)
		if err != nil {
			return nil, err
		}
	}

	return req, nil
}

func (c *JiraClient) Do(req *http.Request, value interface{}) (*jira.Response, error) {
	resp, err := c.client.Do(req, value)
	if err != nil {
		return resp, trace.Wrap(jira.NewJiraError(resp, err))
	}
	return resp, nil
}

func (c *JiraClient) GetMyPermissions(ctx context.Context, options *GetMyPermissionsQueryOptions) (*Permissions, error) {
	req, err := c.NewRequest(ctx, "GET", "rest/api/2/mypermissions", nil, jira.WithQueryOptions(options))
	if err != nil {
		return nil, err
	}

	permissions := &Permissions{}
	_, err = c.Do(req, permissions)
	if err != nil {
		return nil, err
	}
	return permissions, err
}

func (c *JiraClient) GetProject(ctx context.Context, projectID string) (*jira.Project, error) {
	url := fmt.Sprintf("rest/api/2/project/%s", projectID)
	req, err := c.NewRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	project := &jira.Project{}
	_, err = c.Do(req, project)
	if err != nil {
		return nil, err
	}
	return project, err
}

func (c *JiraClient) CreateIssue(ctx context.Context, input *IssueInput) (*CreatedIssue, error) {
	req, err := c.NewRequest(ctx, "POST", "rest/api/2/issue", input)
	if err != nil {
		return nil, err
	}

	createdIssue := &CreatedIssue{}
	_, err = c.Do(req, createdIssue)
	if err != nil {
		return nil, err
	}
	return createdIssue, err
}

func (c *JiraClient) GetIssue(ctx context.Context, issueID string, options *jira.GetQueryOptions) (*Issue, error) {
	url := fmt.Sprintf("rest/api/2/issue/%s", issueID)
	req, err := c.NewRequest(ctx, "GET", url, nil, jira.WithQueryOptions(options))
	if err != nil {
		return nil, err
	}

	issue := &Issue{}
	_, err = c.Do(req, issue)
	if err != nil {
		return nil, err
	}
	return issue, err
}

func (c *JiraClient) TransitionIssue(ctx context.Context, issueID, transitionID string) error {
	url := fmt.Sprintf("rest/api/2/issue/%s/transitions", issueID)
	payload := jira.CreateTransitionPayload{
		Transition: jira.TransitionPayload{
			ID: transitionID,
		},
	}
	req, err := c.NewRequest(ctx, "POST", url, payload)
	if err != nil {
		return err
	}
	_, err = c.Do(req, nil)
	return err
}
