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
	"encoding/binary"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	NoAction ActionID = iota
	ApproveAction
	DenyAction
)

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
	ID        IntID  `json:"id"`
	ProjectID IntID  `json:"project_id"`
	URL       string `json:"url,omitempty"`
}

type IssueParams struct {
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	Labels       string `json:"labels,omitempty"`
	StateEvent   string `json:"state_event,omitempty"`
	RemoveLabels string `json:"remove_labels,omitempty"`
	AddLabels    string `json:"add_labels,omitempty"`
}

type Issue struct {
	ID          IntID    `json:"id,omitempty"`
	IID         IntID    `json:"iid,omitempty"`
	ProjectID   IntID    `json:"project_id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	State       string   `json:"state,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

type IssueObjectAttributes struct {
	Action      string `json:"action,omitempty"`
	ID          IntID  `json:"id,omitempty"`
	IID         IntID  `json:"iid,omitempty"`
	ProjectID   IntID  `json:"project_id,omitempty"`
	Description string `json:"description,omitempty"`
}

type IssueChanges struct {
	Labels *LabelsChange `json:"labels,omitempty"`
}

type User struct {
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
}

type Note struct {
	ID           IntID  `json:"id"`
	NoteableType string `json:"noteable_type"` //nolint:misspell
	NoteableID   IntID  `json:"noteable_id"`   //nolint:misspell
	Body         string `json:"body"`
	Confidential bool   `json:"confidential"`
}

type NoteParams struct {
	Body         string `json:"body"`
	Confidential bool   `json:"confidential,omitempty"`
}

type IssueEvent struct {
	User             User                  `json:"user"`
	Project          Project               `json:"project"`
	ObjectAttributes IssueObjectAttributes `json:"object_attributes"`
	Changes          IssueChanges          `json:"changes"`
}

type Webhook struct {
	Event interface{}
}

type WebhookFunc func(ctx context.Context, hook Webhook) error

type ErrorResult struct {
	Error   string      `json:"error,omitempty"`
	Message interface{} `json:"message,omitempty"`
}

var issueDescriptionRegex = regexp.MustCompile(`(?i)request\s+id\s+is.+([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)

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
func (issue IssueObjectAttributes) ParseDescriptionRequestID() string {
	submatches := issueDescriptionRegex.FindStringSubmatch(issue.Description)
	if len(submatches) > 1 {
		return submatches[1]
	}
	return ""
}
