package main

import (
	"fmt"
	"io/ioutil"

	jira "gopkg.in/andygrunwald/go-jira.v1"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	RequestIdFieldName = "TeleportAccessRequestId"
)

// Bot is a wrapper around jira.Client that works with access.Request
type Bot struct {
	client *jira.Client
	project string
}

func NewBot(conf *Config) (*Bot, error) {
	transport := jira.BasicAuthTransport{
		Username: conf.JIRA.Username,
		Password: conf.JIRA.APIToken,
	}

	client, err := jira.NewClient(transport.Client(), conf.JIRA.URL)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Bot{
		client,
		conf.JIRA.Project,
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

func (c *Bot) GetIssue(issueID string) (issue *jira.Issue, reqID string, err error) {
	requestIdField, err := c.GetRequestIdField()
	if err != nil {
		err = trace.Wrap(err)
		return nil, "", err
	}

	issue, res, err := c.client.Issue.Get(issueID, nil)
	if err != nil {
		body, err := parseErrorResponse(res, err)
		log.Error(body)
		return nil, "", err
	}

	reqID, ok := issue.Fields.Unknowns[requestIdField.Key].(string)
	if !ok {
		return nil, "", trace.Errorf("Got non-string '%s' field", RequestIdFieldName)
	}
	return
}

// ExpireIssue sets "Expired" status to an issue
func (c *Bot) ExpireIssue(reqID string, reqData requestData, jiraData jiraData) error {
	// TODO: implement issue transition
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
	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", trace.NewAggregate(origErr, err)
	}
	return string(bodyBytes), origErr
}
