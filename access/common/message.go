package common

import (
	"fmt"
	"github.com/gravitational/teleport-plugins/lib"
	pd "github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"net/url"
	"strings"
	"text/template"
	"time"
)

// Slack has a 4000 character limit for message texts and 3000 character limit
// for message section texts, so we truncate all reasons to a generous but
// conservative limit
const (
	requestReasonLimit = 500
	resolutionReasonLimit
	reviewReasonLimit
)

var reviewReplyTemplate = template.Must(template.New("review reply").Parse(
	`{{.Author}} reviewed the request at {{.Created.Format .TimeFormat}}.
Resolution: {{.ProposedStateEmoji}} {{.ProposedState}}.
{{if .Reason}}Reason: {{.Reason}}.{{end}}`,
))

func MsgStatusText(tag pd.ResolutionTag, reason string) string {
	var statusEmoji string
	status := string(tag)
	switch tag {
	case pd.Unresolved:
		status = "PENDING"
		statusEmoji = "⏳"
	case pd.ResolvedApproved:
		statusEmoji = "✅"
	case pd.ResolvedDenied:
		statusEmoji = "❌"
	case pd.ResolvedExpired:
		statusEmoji = "⌛"
	}

	statusText := fmt.Sprintf("*Status:* %s %s", statusEmoji, status)
	if reason != "" {
		statusText += fmt.Sprintf("\n*Resolution reason*: %s", lib.MarkdownEscape(reason, resolutionReasonLimit))
	}

	return statusText
}

func MsgFields(reqID string, reqData pd.AccessRequestData, clusterName string, webProxyURL *url.URL) string {
	var builder strings.Builder
	builder.Grow(128)

	msgFieldToBuilder(&builder, "ID", reqID)
	msgFieldToBuilder(&builder, "Cluster", clusterName)

	if len(reqData.User) > 0 {
		msgFieldToBuilder(&builder, "User", reqData.User)
	}
	if reqData.Roles != nil {
		msgFieldToBuilder(&builder, "Role(s)", strings.Join(reqData.Roles, ","))
	}
	if reqData.RequestReason != "" {
		msgFieldToBuilder(&builder, "Reason", lib.MarkdownEscape(reqData.RequestReason, requestReasonLimit))
	}
	if webProxyURL != nil {
		reqURL := *webProxyURL
		reqURL.Path = lib.BuildURLPath("web", "requests", reqID)
		msgFieldToBuilder(&builder, "Link", reqURL.String())
	} else {
		if reqData.ResolutionTag == pd.Unresolved {
			msgFieldToBuilder(&builder, "Approve", fmt.Sprintf("`tsh request review --approve %s`", reqID))
			msgFieldToBuilder(&builder, "Deny", fmt.Sprintf("`tsh request review --deny %s`", reqID))
		}
	}

	return builder.String()
}

func MsgReview(review types.AccessReview) (string, error) {
	if review.Reason != "" {
		review.Reason = lib.MarkdownEscape(review.Reason, reviewReasonLimit)
	}

	var proposedStateEmoji string
	switch review.ProposedState {
	case types.RequestState_APPROVED:
		proposedStateEmoji = "✅"
	case types.RequestState_DENIED:
		proposedStateEmoji = "❌"
	}

	var builder strings.Builder
	err := reviewReplyTemplate.Execute(&builder, struct {
		types.AccessReview
		ProposedState      string
		ProposedStateEmoji string
		TimeFormat         string
	}{
		review,
		review.ProposedState.String(),
		proposedStateEmoji,
		time.RFC822,
	})
	if err != nil {
		return "", trace.Wrap(err)
	}
	return builder.String(), nil
}

func msgFieldToBuilder(b *strings.Builder, field, value string) {
	b.WriteString("*")
	b.WriteString(field)
	b.WriteString("*: ")
	b.WriteString(value)
	b.WriteString("\n")
}
