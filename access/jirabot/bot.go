package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	jira "gopkg.in/andygrunwald/go-jira.v1"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	RequestIdFieldName = "TeleportAccessRequestId"

	jiraMaxConns    = 100
	jiraHttpTimeout = 10 * time.Second
)

// Bot is a wrapper around jira.Client that works with access.Request
type Bot struct {
	client      *jira.Client
	project     string
	clusterName string
}

type Issue struct {
	*jira.Issue
	requestIdField *jira.Field
}

type IssueUpdate struct {
	Status string
	Author jira.User
}

func (issue *Issue) GetRequestID() (string, error) {
	reqID, ok := issue.Fields.Unknowns[issue.requestIdField.Key].(string)
	if !ok {
		return "", trace.Errorf("got non-string '%s' field", RequestIdFieldName)
	}
	return reqID, nil
}

func (issue *Issue) GetLastUpdateBy(status string) (IssueUpdate, error) {
	changelog := issue.Changelog
	if changelog == nil {
		return IssueUpdate{}, trace.Errorf("changelog is missing in API response")
	}

	var update *IssueUpdate
	for _, entry := range changelog.Histories {
		for _, item := range entry.Items {
			if item.FieldType == "jira" && item.Field == "status" && strings.ToLower(item.ToString) == status {
				update = &IssueUpdate{
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
		return IssueUpdate{}, trace.Errorf("cannot find a %q status update in changelog", status)
	}
	return *update, nil
}

func (issue *Issue) GetTransition(status string) (jira.Transition, error) {
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
		client:  client,
		project: conf.JIRA.Project,
	}, nil
}

// CreateIssue creates an issue with "Pending" status
func (c *Bot) CreateIssue(reqID string, reqData requestData) (data jiraData, err error) {
	requestIdField, err := c.GetRequestIdField()
	if err != nil {
		return data, trace.Wrap(err)
	}

	issue, res, err := c.client.Issue.Create(&jira.Issue{
		Fields: &jira.IssueFields{
			Type:    jira.IssueType{Name: "Task"},
			Project: jira.Project{Key: c.project},
			Summary: fmt.Sprintf("Incoming request %s", reqID),
			Unknowns: map[string]interface{}{
				requestIdField.Key: reqID,
			},
		},
	})
	if err != nil {
		body, err := parseErrorResponse(res, err)
		log.Error(body)
		return data, err
	}

	data.ID = issue.ID
	data.Key = issue.Key
	return
}

func (c *Bot) GetIssue(issueID string) (*Issue, error) {
	jiraIssue, res, err := c.client.Issue.Get(issueID, &jira.GetQueryOptions{
		Expand: "changelog,transitions",
	})
	if err != nil {
		body, err := parseErrorResponse(res, err)
		log.Error(body)
		return nil, err
	}
	requestIdField, err := c.GetRequestIdField()
	if err != nil {
		err = trace.Wrap(err)
		return nil, err
	}

	return &Issue{jiraIssue, requestIdField}, nil
}

// ExpireIssue sets "Expired" status to an issue
func (c *Bot) ExpireIssue(reqID string, reqData requestData, jiraData jiraData) error {
	issue, err := c.GetIssue(jiraData.ID)
	if err != nil {
		return trace.Wrap(err)
	}

	transition, err := issue.GetTransition("expired")
	if err != nil {
		return trace.Wrap(err)
	}

	res, err := c.client.Issue.DoTransition(issue.ID, transition.ID)
	if err != nil {
		body, err := parseErrorResponse(res, err)
		log.Error(body)
		return trace.Wrap(err)
	}

	return nil
}

func (c *Bot) GetRequestIdField() (field *jira.Field, err error) {
	fields, res, err := c.client.Field.GetList()
	if err != nil {
		body, err := parseErrorResponse(res, err)
		log.Error(body)
		return nil, err
	}

	for _, f := range fields {
		if f.Custom && f.Name == RequestIdFieldName {
			field = &f
			break
		}
	}
	if field == nil {
		err = trace.Errorf("Cannot find custom field '%s'", RequestIdFieldName)
	}
	return
}

func parseErrorResponse(response *jira.Response, origErr error) (string, error) {
	if response == nil {
		return "", origErr
	}
	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", trace.NewAggregate(origErr, err)
	}
	return string(bodyBytes), origErr
}
