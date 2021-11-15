/*
Copyright 2020-2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"text/template"
	"time"

	"gopkg.in/resty.v1"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

const (
	gitlabMaxConns    = 100
	gitlabHTTPTimeout = 10 * time.Second
)

type Gitlab struct {
	client        *resty.Client
	server        *WebhookServer
	webhookSecret string
	baseURL       *url.URL
	apiToken      string

	clusterName string
	webProxyURL *url.URL
	labels      map[string]string
}

var nextLinkHeaderRegex = regexp.MustCompile(`<([^>]+)>;\s+rel="next"`)

var descriptionTemplate = template.Must(template.New("description").Parse(
	`{{.User}} requested permissions for roles {{range $index, $element := .Roles}}{{if $index}}, {{end}}**{{ . }}**{{end}} on Teleport at **{{.Created.Format .TimeFormat}}**. To approve or deny the request, please assign a corresponding label and close the issue{{if .RequestLink}} or proceed to {{.RequestLink}}{{end}}.

{{if .RequestReason}}Reason: **{{.RequestReason}}**.{{end}}

Request ID is ` + "`{{.ID}}`.",
))
var reviewCommentTemplate = template.Must(template.New("review comment").Parse(
	`**{{.Author}}** reviewed the request at **{{.Created.Format .TimeFormat}}**.

Resolution: **{{.ProposedState}}**.

{{if .Reason}}Reason: {{.Reason}}.{{end}}`,
))
var resolutionCommentTemplate = template.Must(template.New("resolution comment").Parse(
	`Access request has been {{.Resolution}}

{{if .ResolveReason}}Reason: {{.ResolveReason}}{{end}}`,
))

// NewGitlabClient builds a new GitLab client.
func NewGitlabClient(conf GitlabConfig, clusterName, webProxyAddr string, server *WebhookServer) (Gitlab, error) {
	var (
		webProxyURL *url.URL
		err         error
	)
	if webProxyAddr != "" {
		if webProxyURL, err = lib.AddrToURL(webProxyAddr); err != nil {
			return Gitlab{}, trace.Wrap(err)
		}
	}

	client := resty.NewWithClient(&http.Client{
		Timeout: gitlabHTTPTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     gitlabMaxConns,
			MaxIdleConnsPerHost: gitlabMaxConns,
		},
	})

	var baseURL *url.URL
	if urlStr := conf.URL; urlStr != "" {
		baseURL, err = url.Parse(urlStr)
		if err != nil {
			return Gitlab{}, trace.Wrap(err)
		}
	} else {
		baseURL = &url.URL{
			Scheme: "https",
			Host:   "gitlab.com",
		}
	}
	return Gitlab{
		client:        client,
		server:        server,
		baseURL:       baseURL,
		clusterName:   clusterName,
		webProxyURL:   webProxyURL,
		apiToken:      conf.Token,
		webhookSecret: conf.WebhookSecret,
		labels:        map[string]string{},
	}, nil
}

func (g Gitlab) NewRequest(ctx context.Context) *resty.Request {
	return g.client.R().
		SetContext(ctx).
		SetError(&ErrorResult{}).
		SetHeader("Accept", "application/json").
		SetHeader("PRIVATE-TOKEN", g.apiToken)
}

func (g Gitlab) APIV4URL(args ...interface{}) string {
	args = append([]interface{}{"api", "v4"}, args...)
	url := *g.baseURL
	url.Path = lib.BuildURLPath(args...)
	return url.String()
}

// HealthCheck checks that project is accessible by API.
func (g Gitlab) HealthCheck(ctx context.Context, projectIDOrPath string) (IntID, error) {
	var project Project
	resp, err := g.NewRequest(ctx).
		SetResult(&project).
		Get(g.APIV4URL("projects", projectIDOrPath))
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if resp.IsError() {
		if contentType := resp.Header().Get("Content-Type"); contentType != "application/json" {
			return 0, trace.Errorf("wrong content type %s", contentType)
		}
		if code := resp.StatusCode(); code == http.StatusUnauthorized {
			return 0, trace.Errorf("got %v from API endpoint, perhaps GitLab credentials are not configured well", code)
		}
		return 0, responseError(resp)
	}
	if project.ID == 0 {
		return 0, trace.Errorf("bad response from GitLab API")
	}
	return project.ID, nil
}

func (g Gitlab) listPages(ctx context.Context, url string, result interface{}, fn func(interface{}) bool) error {
	req := g.NewRequest(ctx)
	req.SetQueryParams(map[string]string{
		"order_by": "id",
		"sort":     "asc",
	})

	for {
		req.SetResult(result)
		resp, err := req.Get(url)
		if err != nil {
			return trace.Wrap(err)
		}
		if resp.IsError() {
			return trace.Wrap(responseError(resp))
		}

		if !fn(resp.Result()) {
			break
		}

		submatches := nextLinkHeaderRegex.FindStringSubmatch(resp.Header().Get("link"))
		if len(submatches) > 1 {
			req = g.NewRequest(ctx)
			url = submatches[1]
		} else {
			break
		}
	}
	return nil
}

func (g Gitlab) SetupProjectHook(ctx context.Context, projectID, existingHookID IntID) (IntID, error) {
	var err error
	url := g.server.WebhookURL()
	if existingHookID == 0 {
		existingHookID, err = g.findProjectHook(ctx, projectID, url)
		if err != nil {
			return 0, trace.Wrap(err)
		}
		if existingHookID != 0 {
			return existingHookID, nil
		}
		return g.createProjectHook(ctx, projectID, url)
	}
	resp, err := g.NewRequest(ctx).
		SetBody(HookParams{
			URL:               url,
			Token:             g.webhookSecret,
			EnableIssueEvents: true,
		}).
		Put(g.APIV4URL("projects", projectID, "hooks", existingHookID))
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if resp.IsError() {
		if resp.StatusCode() == http.StatusNotFound {
			return g.createProjectHook(ctx, projectID, url)
		}
		return 0, responseError(resp)
	}
	return existingHookID, nil
}

func (g Gitlab) findProjectHook(ctx context.Context, projectID IntID, webhookURL string) (IntID, error) {
	var result IntID
	err := g.listPages(ctx, g.APIV4URL("projects", projectID, "hooks"), []ProjectHook(nil), func(page interface{}) bool {
		for _, hook := range *page.(*[]ProjectHook) {
			if hook.URL == webhookURL {
				result = hook.ID
				return false
			}
		}
		return true
	})
	return result, trace.Wrap(err)
}

func (g Gitlab) createProjectHook(ctx context.Context, projectID IntID, url string) (IntID, error) {
	var result struct {
		ID IntID `json:"id"`
	}
	resp, err := g.NewRequest(ctx).
		SetBody(HookParams{
			URL:               url,
			Token:             g.webhookSecret,
			EnableIssueEvents: true,
		}).
		SetResult(&result).
		Post(g.APIV4URL("projects", projectID, "hooks"))
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if resp.IsError() {
		return 0, trace.Wrap(responseError(resp))
	}
	return result.ID, nil
}

func (g *Gitlab) SetupLabels(ctx context.Context, projectID IntID, existingLabels map[string]string) error {
	existingKeys := make(map[string]string)
	for key, name := range existingLabels {
		if name != "" {
			existingKeys[name] = key
		}
	}
	err := g.listPages(ctx, g.APIV4URL("projects", projectID, "labels"), []Label(nil), func(page interface{}) bool {
		for _, label := range *page.(*[]Label) {
			if key := existingKeys[label.Name]; key != "" {
				g.labels[key] = label.Name
			} else if key := LabelName(label.Name).Reduced(); key != "" && g.labels[key] == "" {
				g.labels[key] = label.Name
			}
		}
		return true
	})
	if err != nil {
		return trace.Wrap(err)
	}
	for key := range existingLabels {
		if name := g.labels[key]; name == "" {
			name, err := g.createLabel(ctx, projectID, key)
			if err != nil {
				return trace.Wrap(err)
			}
			g.labels[key] = name
		} else {
			g.labels[key] = name
		}
	}
	return nil
}

func (g Gitlab) createLabel(ctx context.Context, projectID IntID, key string) (string, error) {
	log := logger.Get(ctx)
	name := fmt.Sprintf("Teleport: %s", strings.Title(key))
	log.Debugf("Trying to create a label %q", name)
	var label Label
	resp, err := g.NewRequest(ctx).
		SetBody(LabelParams{
			Name:  name,
			Color: defaultLabelColor(key),
		}).
		SetResult(&label).
		Post(g.APIV4URL("projects", projectID, "labels"))
	if err != nil {
		return "", trace.Wrap(err)
	}
	if resp.IsError() {
		if resp.Error().(*ErrorResult).Message == "Label already exists" {
			log.Debugf("Label %q already exists", name)
			// Race condition here though normally it must not happen.
			return name, nil

		}
		return "", trace.Wrap(responseError(resp))
	}
	return label.Name, nil
}

func defaultLabelColor(key string) string {
	switch key {
	case "pending":
		return "#FFECDB"
	case "approved":
		return "#428BCA"
	case "denied":
		return "#D9534F"
	case "expired":
		return "#7F8C8D"
	default:
		return ""
	}
}

func (g Gitlab) CreateIssue(ctx context.Context, projectID IntID, reqID string, reqData RequestData) (GitlabData, error) {
	description, err := g.buildIssueDescription(reqID, reqData)
	if err != nil {
		return GitlabData{}, trace.Wrap(err)
	}
	var result struct {
		ID        IntID `json:"id"`
		IID       IntID `json:"iid"`
		ProjectID IntID `json:"project_id"`
	}
	resp, err := g.NewRequest(ctx).
		SetBody(IssueParams{
			Title:       fmt.Sprintf("Access request from %s", reqData.User),
			Description: description,
			Labels:      g.labels["pending"],
		}).
		SetResult(&result).
		Post(g.APIV4URL("projects", projectID, "issues"))
	if err != nil {
		return GitlabData{}, trace.Wrap(err)
	}
	if resp.IsError() {
		return GitlabData{}, trace.Wrap(responseError(resp))
	}
	return GitlabData{
		IssueID:   result.ID,
		IssueIID:  result.IID,
		ProjectID: result.ProjectID,
	}, nil
}

func (g Gitlab) buildIssueDescription(reqID string, reqData RequestData) (string, error) {
	var requestLink string
	if g.webProxyURL != nil {
		reqURL := *g.webProxyURL
		reqURL.Path = lib.BuildURLPath("web", "requests", reqID)
		requestLink = reqURL.String()
	}

	var builder strings.Builder
	err := descriptionTemplate.Execute(&builder, struct {
		ID          string
		TimeFormat  string
		RequestLink string
		RequestData
	}{
		reqID,
		time.RFC822,
		requestLink,
		reqData,
	})
	if err != nil {
		return "", trace.Wrap(err)
	}
	return builder.String(), nil
}

// GetIssue loads issue info.
func (g Gitlab) GetIssue(ctx context.Context, projectID, issueIID IntID) (Issue, error) {
	var issue Issue
	resp, err := g.NewRequest(ctx).
		SetResult(&issue).
		Get(g.APIV4URL("projects", projectID, "issues", issueIID))
	if err != nil {
		return Issue{}, trace.Wrap(err)
	}
	if resp.IsError() {
		return Issue{}, trace.Wrap(responseError(resp))
	}
	return issue, nil
}

// ResolveIssue adds a resolution comment to the issue and closes it.
func (g Gitlab) ResolveIssue(ctx context.Context, projectID, issueIID IntID, resolution Resolution) error {
	// Try to add a comment.
	err1 := trace.Wrap(g.PostResolutionComment(ctx, projectID, issueIID, resolution))

	// Try to close the issue.
	err2 := trace.Wrap(g.CloseIssue(ctx, projectID, issueIID, resolution))

	return trace.NewAggregate(err1, err2)
}

// CloseIssue sets an issue e.g. "approved", "denied" or "expired" and closes it.
func (g Gitlab) CloseIssue(ctx context.Context, projectID, issueIID IntID, resolution Resolution) error {
	params := IssueParams{
		StateEvent:   "close",
		RemoveLabels: g.labels["pending"],
		AddLabels:    g.labels[string(resolution.Tag)],
	}
	resp, err := g.NewRequest(ctx).
		SetBody(params).
		Put(g.APIV4URL("projects", projectID, "issues", issueIID))
	if err != nil {
		return trace.Wrap(err)
	}
	if resp.IsError() {
		return trace.Wrap(responseError(resp))
	}

	logger.Get(ctx).Debug("Successfully closed the issue")
	return nil
}

// PostReviewComment posts an issue comment about access review added to a request.
func (g Gitlab) PostReviewComment(ctx context.Context, projectID, issueIID IntID, review types.AccessReview) error {
	var builder strings.Builder
	err := reviewCommentTemplate.Execute(&builder, struct {
		types.AccessReview
		ProposedState string
		TimeFormat    string
	}{
		review,
		review.ProposedState.String(),
		time.RFC822,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	resp, err := g.NewRequest(ctx).
		SetBody(NoteParams{Body: builder.String()}).
		Post(g.APIV4URL("projects", projectID, "issues", issueIID, "notes"))
	if err != nil {
		return trace.Wrap(err)
	}
	if resp.IsError() {
		return trace.Wrap(responseError(resp))
	}

	logger.Get(ctx).Debug("Successfully posted a review comment to the issue")
	return nil
}

// PostResolutionComment posts an issue comment about access review added to a request.
func (g Gitlab) PostResolutionComment(ctx context.Context, projectID, issueIID IntID, resolution Resolution) error {
	var builder strings.Builder
	err := resolutionCommentTemplate.Execute(&builder, struct {
		Resolution    string
		ResolveReason string
	}{
		string(resolution.Tag),
		resolution.Reason,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	resp, err := g.NewRequest(ctx).
		SetBody(NoteParams{Body: builder.String()}).
		Post(g.APIV4URL("projects", projectID, "issues", issueIID, "notes"))
	if err != nil {
		return trace.Wrap(err)
	}
	if resp.IsError() {
		return trace.Wrap(responseError(resp))
	}

	logger.Get(ctx).Debug("Successfully posted a resolution comment to the issue")
	return nil
}

func responseError(resp *resty.Response) error {
	result := resp.Error().(*ErrorResult)
	err := fmt.Sprintf("http error code=%v", resp.StatusCode())
	if result.Error != "" {
		err += fmt.Sprintf(", error=%q", result.Error)
	}
	if result.Message != nil {
		err += fmt.Sprintf(", message=%v", result.Message)
	}
	return trace.Errorf(err)
}
