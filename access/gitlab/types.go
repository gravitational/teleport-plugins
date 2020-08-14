package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gravitational/teleport-plugins/access"

	log "github.com/sirupsen/logrus"
)

const (
	NoAction ActionID = iota
	ApproveAction
	DenyAction
)

type RequestData struct {
	User    string
	Roles   []string
	Created time.Time
}

type GitlabData struct {
	ID        IntID
	IID       IntID
	ProjectID IntID
}

type PluginData struct {
	RequestData
	GitlabData
}

type logFields = log.Fields

type ActionID int

type IntID uint64

type LabelName string

type LabelParams struct {
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

type Label struct {
	ID IntID `json:"id"`

	// Title and Name really are the same things but both used in different contexts :(
	Title string `json:"title"`
	Name  string `json:"name"`
}

type LabelsChange struct {
	Previous []Label `json:"previous"`
	Current  []Label `json:"current"`
}

type SortableLabels []Label

type Project struct {
	ID IntID `json:"id"`
}

type HookParams struct {
	URL               string `json:"url,omitempty"`
	EnableIssueEvents bool   `json:"issues_events,omitempty"`
	Token             string `json:"token,omitempty"`
}

type ProjectHook struct {
	ID  IntID  `json:"id"`
	URL string `json:"url,omitempty"`
}

type IssueParams struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Labels      string `json:"labels,omitempty"`
	StateEvent  string `json:"state_event,omitempty"`
}

type Issue struct {
	ID          IntID   `json:"id,omitempty"`
	IID         IntID   `json:"iid,omitempty"`
	ProjectID   IntID   `json:"project_id,omitempty"`
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	State       string  `json:"state,omitempty"`
	Labels      []Label `json:"labels,omitempty"`
}

type IssueObjectAttributes struct {
	Action string `json:"action,omitempty"`
	Issue
}

type IssueChanges struct {
	Labels *LabelsChange `json:"labels,omitempty"`
}

type IssueEvent struct {
	User struct {
		Name     string `json:"name,omitempty"`
		Username string `json:"username,omitempty"`
		Email    string `json:"email,omitempty"`
	} `json:"user"`
	Project          Project               `json:"project"`
	ObjectAttributes IssueObjectAttributes `json:"object_attributes"`
	Changes          IssueChanges          `json:"changes"`
}

type Webhook struct {
	HTTPID string
	Event  interface{}
}

type WebhookFunc func(ctx context.Context, hook Webhook) error

type ErrorResult struct {
	Error   string      `json:"error,omitempty"`
	Message interface{} `json:"message,omitempty"`
}

var issueDescriptionRegex = regexp.MustCompile(`(?i)request\s+id\s+is\s+([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)

func DecodePluginData(dataMap access.PluginDataMap) (data PluginData) {
	var created int64
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	fmt.Sscanf(dataMap["created"], "%d", &created)
	data.Created = time.Unix(created, 0)
	fmt.Sscanf(dataMap["issue_id"], "%d", &data.ID)
	fmt.Sscanf(dataMap["issue_iid"], "%d", &data.IID)
	fmt.Sscanf(dataMap["project_id"], "%d", &data.ProjectID)
	return
}

func EncodePluginData(data PluginData) access.PluginDataMap {
	return access.PluginDataMap{
		"issue_id":   fmt.Sprintf("%d", data.ID),
		"issue_iid":  fmt.Sprintf("%d", data.IID),
		"project_id": fmt.Sprintf("%d", data.ProjectID),
		"user":       data.User,
		"roles":      strings.Join(data.Roles, ","),
		"created":    fmt.Sprintf("%d", data.Created.Unix()),
	}
}

func (id IntID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

func IntIDToBytes(id IntID) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(id))
	return data
}

func BytesToIntID(data []byte) IntID {
	if data != nil {
		return IntID(binary.BigEndian.Uint64(data))
	}
	return 0
}

// ToAction maps label name like "Teleport: Approved", "teleport : denied", etc to a specific action.
func (name LabelName) ToAction() ActionID {
	switch name.Reduced() {
	case "approved":
		return ApproveAction
	case "denied":
		return DenyAction
	default:
		return NoAction
	}
}

// Reduced maps label name like "Teleport: Approved", "teleport : denied" to "approved", "denied", etc.
func (name LabelName) Reduced() string {
	substrs := strings.SplitN(strings.ToLower(string(name)), ":", 2)
	if len(substrs) != 2 || strings.TrimFunc(substrs[0], unicode.IsSpace) != "teleport" {
		return ""
	}
	return strings.TrimFunc(substrs[1], unicode.IsSpace)
}

// NewSortableLabels is a Label slice wrapper that implements sort.Interface sorting by label ID.
func NewSortableLabels(slice []Label) SortableLabels {
	if slice == nil {
		return SortableLabels(nil)
	}
	labels := make([]Label, len(slice))
	copy(labels, slice)
	return SortableLabels(labels)
}

func (labels SortableLabels) Len() int           { return len(labels) }
func (labels SortableLabels) Swap(i, j int)      { labels[i], labels[j] = labels[j], labels[i] }
func (labels SortableLabels) Less(i, j int) bool { return labels[i].ID < labels[j].ID }

// Diff subtracts previous labels from current.
func (labels *LabelsChange) Diff() []Label {
	currentLabels := NewSortableLabels(labels.Current)
	previousLabels := NewSortableLabels(labels.Previous)
	sort.Sort(currentLabels)
	sort.Sort(previousLabels)

	var diff []Label
	for i, j := 0, 0; i < currentLabels.Len(); {
		if j < previousLabels.Len() {
			currentID, previousID := currentLabels[i].ID, previousLabels[j].ID
			switch {
			case currentID == previousID:
				i++
				j++
			case currentID < previousID:
				diff = append(diff, currentLabels[i])
				i++
			case currentID > previousID:
				j++
			}
		} else {
			diff = append(diff, currentLabels[i])
			i++

		}
	}
	return diff
}

// ParseDescriptionRequestID is a fallback for searching request id in the issue description
// if it's missing in the database.
func (issue *Issue) ParseDescriptionRequestID() string {
	submatches := issueDescriptionRegex.FindStringSubmatch(issue.Description)
	if len(submatches) > 1 {
		return submatches[1]
	}
	return ""
}
