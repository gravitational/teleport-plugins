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

	"github.com/go-resty/resty"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	gitlabMaxConns    = 100
	gitlabHTTPTimeout = 10 * time.Second
)

type Bot struct {
	client        *resty.Client
	server        *WebhookServer
	projectID     string
	webhookSecret string
	baseURL       *url.URL
	apiToken      string

	clusterName string
	labels      map[string]string
}

var descriptionTemplate *template.Template
var nextLinkHeaderRegex *regexp.Regexp

func init() {
	var err error
	descriptionTemplate, err = template.New("description").Parse(
		`{{.User}} requested permissions for roles {{range $index, $element := .Roles}}{{if $index}}, {{end}}{{ . }}{{end}} on Teleport at {{.Created.Format .TimeFormat}}. To approve or deny the request, please assign a corresponding label and close the issue.

Request ID is {{.ID}}.
`)
	if err != nil {
		panic(err)
	}

	nextLinkHeaderRegex, err = regexp.Compile(`<([^>]+)>;\s+rel="next"`)
	if err != nil {
		panic(err)
	}
}

func NewBot(conf *Config, onAction WebhookFunc) (*Bot, error) {
	var err error

	client := resty.NewWithClient(&http.Client{
		Timeout: gitlabHTTPTimeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     gitlabMaxConns,
			MaxIdleConnsPerHost: gitlabMaxConns,
		},
	})
	webhookSecret := conf.Gitlab.WebhookSecret
	server, err := NewWebhookServer(conf.HTTP, webhookSecret, onAction)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var baseURL *url.URL
	if urlStr := conf.Gitlab.URL; urlStr != "" {
		baseURL, err = url.Parse(urlStr)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	} else {
		baseURL = &url.URL{
			Scheme: "https",
			Host:   "gitlab.com",
		}
	}
	return &Bot{
		client:        client,
		server:        server,
		baseURL:       baseURL,
		projectID:     conf.Gitlab.ProjectID,
		apiToken:      conf.Gitlab.Token,
		webhookSecret: webhookSecret,
		labels:        map[string]string{},
	}, nil
}

func (b *Bot) RunServer(ctx context.Context) error {
	return b.server.Run(ctx)
}

func (b *Bot) ShutdownServer(ctx context.Context) error {
	return b.server.Shutdown(ctx)
}

func (b *Bot) NewRequest(ctx context.Context) *resty.Request {
	return b.client.R().
		SetContext(ctx).
		SetError(&ErrorResult{}).
		SetHeader("Accept", "application/json").
		SetHeader("PRIVATE-TOKEN", b.apiToken)
}

func (b *Bot) APIV4URL(args ...interface{}) string {
	args = append([]interface{}{"api", "v4"}, args...)
	url := *b.baseURL
	url.Path = utils.BuildURLPath(args...)
	return url.String()
}

func (b *Bot) HealthCheck(ctx context.Context) (IntID, error) {
	var project Project
	resp, err := b.NewRequest(ctx).
		SetResult(&project).
		Get(b.APIV4URL("projects", b.projectID))
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if resp.IsError() {
		if contentType := resp.Header().Get("Content-Type"); contentType != "application/json" {
			return 0, trace.Errorf("wrong content_type=%q", contentType)
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

func (b *Bot) listPages(ctx context.Context, url string, result interface{}, fn func(interface{}) bool) error {
	req := b.NewRequest(ctx)
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
			req = b.NewRequest(ctx)
			url = submatches[1]
		} else {
			break
		}
	}
	return nil
}

func (b *Bot) SetupProjectHook(ctx context.Context, existingID IntID) (IntID, error) {
	var err error
	url := b.server.WebhookURL()
	if existingID == 0 {
		existingID, err = b.findProjectHook(ctx, url)
		if err != nil {
			return 0, trace.Wrap(err)
		}
		if existingID != 0 {
			return existingID, nil
		}
		return b.createProjectHook(ctx, url)
	}
	resp, err := b.NewRequest(ctx).
		SetBody(&HookParams{
			URL:               url,
			Token:             b.webhookSecret,
			EnableIssueEvents: true,
		}).
		Put(b.APIV4URL("projects", b.projectID, "hooks", existingID))
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if resp.IsError() {
		if resp.StatusCode() == http.StatusNotFound {
			return b.createProjectHook(ctx, url)
		}
		return 0, responseError(resp)
	}
	return existingID, nil
}

func (b *Bot) findProjectHook(ctx context.Context, webhookURL string) (IntID, error) {
	var result IntID
	err := b.listPages(ctx, b.APIV4URL("projects", b.projectID, "hooks"), []ProjectHook(nil), func(page interface{}) bool {
		for _, hook := range *page.(*[]ProjectHook) {
			log.Debugf("HOOK_URL: %s", hook.URL)
			if hook.URL == webhookURL {
				result = hook.ID
				return false
			}
		}
		return true
	})
	return result, trace.Wrap(err)
}

func (b *Bot) createProjectHook(ctx context.Context, url string) (IntID, error) {
	var result struct {
		ID IntID `json:"id"`
	}
	resp, err := b.NewRequest(ctx).
		SetBody(&HookParams{
			URL:               url,
			Token:             b.webhookSecret,
			EnableIssueEvents: true,
		}).
		SetResult(&result).
		Post(b.APIV4URL("projects", b.projectID, "hooks"))
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if resp.IsError() {
		return 0, trace.Wrap(responseError(resp))
	}
	return result.ID, nil
}

func (b *Bot) SetupLabels(ctx context.Context, existingLabels map[string]string) error {
	existingKeys := make(map[string]string)
	for key, name := range existingLabels {
		if name != "" {
			existingKeys[name] = key
		}
	}
	err := b.listPages(ctx, b.APIV4URL("projects", b.projectID, "labels"), []Label(nil), func(page interface{}) bool {
		for _, label := range *page.(*[]Label) {
			if key := existingKeys[label.Name]; key != "" {
				b.labels[key] = label.Name
			} else if key := LabelName(label.Name).Reduced(); key != "" && b.labels[key] == "" {
				b.labels[key] = label.Name
			}
		}
		return true
	})
	if err != nil {
		return trace.Wrap(err)
	}
	for key := range existingLabels {
		if name := b.labels[key]; name == "" {
			name, err := b.createLabel(ctx, key)
			if err != nil {
				return trace.Wrap(err)
			}
			b.labels[key] = name
		} else {
			b.labels[key] = name
		}
	}
	return nil
}

func (b *Bot) createLabel(ctx context.Context, key string) (string, error) {
	name := fmt.Sprintf("Teleport: %s", strings.Title(key))
	log.Debugf("Trying to create a label %q", name)
	var label Label
	resp, err := b.NewRequest(ctx).
		SetBody(&LabelParams{
			Name:  name,
			Color: defaultLabelColor(key),
		}).
		SetResult(&label).
		Post(b.APIV4URL("projects", b.projectID, "labels"))
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

func (b *Bot) CreateIssue(ctx context.Context, reqID string, reqData RequestData) (GitlabData, error) {
	description, err := b.BuildIssueDescription(reqID, reqData)
	if err != nil {
		return GitlabData{}, trace.Wrap(err)
	}
	var result struct {
		ID        IntID `json:"id"`
		IID       IntID `json:"iid"`
		ProjectID IntID `json:"project_id"`
	}
	resp, err := b.NewRequest(ctx).
		SetBody(&IssueParams{
			Title:       fmt.Sprintf("Access request from %s", reqData.User),
			Description: description,
			Labels:      b.labels["pending"],
		}).
		SetResult(&result).
		Post(b.APIV4URL("projects", b.projectID, "issues"))
	if err != nil {
		return GitlabData{}, trace.Wrap(err)
	}
	if resp.IsError() {
		return GitlabData{}, trace.Wrap(responseError(resp))
	}
	return GitlabData{
		ID:        result.ID,
		IID:       result.IID,
		ProjectID: result.ProjectID,
	}, nil
}

func (b *Bot) BuildIssueDescription(reqID string, reqData RequestData) (string, error) {
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

func (b *Bot) GetIssue(ctx context.Context, issueID IntID) (Issue, error) {
	var issue Issue
	resp, err := b.NewRequest(ctx).
		SetResult(&issue).
		Get(b.APIV4URL("issues", issueID))
	if err != nil {
		return issue, trace.Wrap(err)
	}
	if resp.IsError() {
		return issue, trace.Wrap(responseError(resp))
	}
	return issue, nil
}

func (b *Bot) GetIssueLabelNames(ctx context.Context, issueIID IntID) ([]string, error) {
	var issue Issue
	resp, err := b.NewRequest(ctx).
		SetResult(&issue).
		Get(b.APIV4URL("projects", b.projectID, "issues", issueIID))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if resp.IsError() {
		return nil, trace.Wrap(responseError(resp))
	}
	var labelNames []string
	for _, label := range issue.Labels {
		labelNames = append(labelNames, label.Title)
	}
	return labelNames, nil
}

func (b *Bot) CloseIssue(ctx context.Context, issueIID IntID, setLabel string) error {
	params := IssueParams{StateEvent: "close"}
	if setLabelName := b.labels[setLabel]; setLabelName != "" {
		existingNames, err := b.GetIssueLabelNames(ctx, issueIID)
		if err != nil {
			return trace.Wrap(err)
		}
		var names []string
		// Filter out all plugin labels.
		for _, name := range existingNames {
			if LabelName(name).Reduced() == "" {
				names = append(names, name)
			}
		}
		params.Labels = strings.Join(append(names, setLabelName), ",")
	}
	resp, err := b.NewRequest(ctx).
		SetBody(&params).
		Put(b.APIV4URL("projects", b.projectID, "issues", issueIID))
	if err != nil {
		return trace.Wrap(err)
	}
	if resp.IsError() {
		return trace.Wrap(responseError(resp))
	}
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
