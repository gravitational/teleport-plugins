package main

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	cards "github.com/DanielTitkov/go-adaptive-cards"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
)

// BuildCard builds the MS Teams message from a request data
func BuildCard(id string, webProxyURL *url.URL, clusterName string, data plugindata.AccessRequestData, reviews []types.AccessReview) (string, error) {
	var statusEmoji string
	status := string(data.ResolutionTag)
	statusColor := ""
	statusEmoji = resolutionIcon(data.ResolutionTag)

	switch data.ResolutionTag {
	case plugindata.Unresolved:
		status = "PENDING"
		statusColor = "Accent"
	case plugindata.ResolvedApproved:
		statusColor = "Good"
	case plugindata.ResolvedDenied:
		statusColor = "Attention"
	case plugindata.ResolvedExpired:
		statusColor = "Accent"
	}

	var actions []cards.Node

	log.Default().Printf("Cluster : %s", clusterName)
	log.Default().Printf("User : %s", data.User)
	log.Default().Printf("Roles : %s", strings.Join(data.Roles, ", "))

	facts := []*cards.Fact{
		{Title: "Cluster", Value: clusterName},
		{Title: "User", Value: data.User},
		{Title: "Role(s)", Value: strings.Join(data.Roles, ", ")},
	}

	if data.RequestReason != "" {
		log.Default().Printf("Reason : %s", data.RequestReason)
		facts = append(facts, &cards.Fact{Title: "Reason", Value: data.RequestReason})
	}

	if data.ResolutionReason != "" {
		log.Default().Printf("Resolution Reason : %s", data.ResolutionReason)
		facts = append(facts, &cards.Fact{Title: "Resolution reason", Value: data.ResolutionReason})
	}

	if webProxyURL != nil {
		reqURL := *webProxyURL
		reqURL.Path = lib.BuildURLPath("web", "requests", id)
		actions = []cards.Node{
			&cards.ActionOpenURL{
				URL:   reqURL.String(),
				Title: "Open",
			},
		}
	} else {
		if data.ResolutionTag == plugindata.Unresolved {
			facts = append(
				facts,
				&cards.Fact{Title: "Approve", Value: fmt.Sprintf("tsh request review --approve %s", id)},
				&cards.Fact{Title: "Deny", Value: fmt.Sprintf("tsh request review --deny %s", id)},
			)
		}
	}

	body := []cards.Node{
		&cards.TextBlock{
			Text: fmt.Sprintf("Access Request %v", id),
			Size: "small",
		},
		&cards.ColumnSet{
			Columns: []*cards.Column{
				{
					Width: "stretch",
					Items: []cards.Node{
						&cards.TextBlock{
							Text: statusEmoji,
							Size: "large",
						},
					},
				},
				{
					Width: "auto",
					Items: []cards.Node{
						&cards.TextBlock{
							Text:   status,
							Size:   "large",
							Weight: "bolder",
							Color:  statusColor,
						},
					},
				},
			},
		},
		&cards.FactSet{
			Facts: facts,
		},
	}

	if len(reviews) > 0 {
		body = append(
			body,
			&cards.TextBlock{
				Text:      "Reviews",
				Weight:    "bolder",
				Color:     "accent",
				Separator: cards.TruePtr(),
			},
		)

		nodes := make([]cards.Node, 0)

		for i, r := range reviews {
			log.Default().Printf("Review %d - Proposed state : %s", i, r.ProposedState.String())
			log.Default().Printf("Review %d - Status : %s", i, resolutionIcon(plugindata.ResolutionTag(r.ProposedState.String())))
			log.Default().Printf("Review %d - Author : %s", i, r.Author)
			log.Default().Printf("Review %d - Created at : %s", i, r.Created.Format(time.RFC822))
			facts := []*cards.Fact{
				{
					Title: "Status",
					Value: resolutionIcon(plugindata.ResolutionTag(r.ProposedState.String())),
				},
				{
					Title: "Author",
					Value: r.Author,
				},
				{
					Title: "Created at",
					Value: r.Created.Format(time.RFC822),
				},
			}

			if r.Reason != "" {
				log.Default().Printf("Review %d - Reason : %s", i, r.Reason)
				facts = append(facts, &cards.Fact{
					Title: "Reason",
					Value: r.Reason,
				})
			}

			nodes = append(nodes, &cards.FactSet{Facts: facts})
		}

		body = append(body, nodes...)
	}

	card := cards.New(body, actions).
		WithSchema(cards.DefaultSchema).
		WithVersion(cards.Version12)

	return card.StringIndent("", "    ")
}

func resolutionIcon(tag plugindata.ResolutionTag) string {
	switch tag {
	case plugindata.Unresolved:
		return "⏳"
	case plugindata.ResolvedApproved:
		return "✅"
	case plugindata.ResolvedDenied:
		return "❌"
	case plugindata.ResolvedExpired:
		return "⌛"
	}

	return ""
}
