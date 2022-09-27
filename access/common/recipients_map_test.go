package common

import (
	"testing"

	"github.com/gravitational/teleport/api/types"
	"github.com/pelletier/go-toml"
	"github.com/stretchr/testify/require"
)

type wrapRecipientsMap struct {
	RecipientsMap RecipientsMap `toml:"role_to_recipients"`
}

func TestRecipientsMap(t *testing.T) {
	testCases := []struct {
		desc             string
		in               string
		expectRecipients RecipientsMap
	}{
		{
			desc: "test role_to_recipients multiple format",
			in: `
            [role_to_recipients]
            "dev" = ["dev-channel", "admin-channel"]
            "*" = "admin-channel"
            `,
			expectRecipients: RecipientsMap{
				"dev":          []string{"dev-channel", "admin-channel"},
				types.Wildcard: []string{"admin-channel"},
			},
		},
		{
			desc: "test role_to_recipients role to list of recipients",
			in: `
            [role_to_recipients]
            "dev" = ["dev-channel", "admin-channel"]
            "prod" = ["sre-channel", "oncall-channel"]
            `,
			expectRecipients: RecipientsMap{
				"dev":  []string{"dev-channel", "admin-channel"},
				"prod": []string{"sre-channel", "oncall-channel"},
			},
		},
		{
			desc: "test role_to_recipients role to string recipient",
			in: `
            [role_to_recipients]
            "single" = "admin-channel"
            `,
			expectRecipients: RecipientsMap{
				"single": []string{"admin-channel"},
			},
		},
		{
			desc: "test role_to_recipients multiple format",
			in: `
            [role_to_recipients]
            "dev" = ["dev-channel", "admin-channel"]
            "*" = "admin-channel"
            `,
			expectRecipients: RecipientsMap{
				"dev":          []string{"dev-channel", "admin-channel"},
				types.Wildcard: []string{"admin-channel"},
			},
		},
		{
			desc: "test role_to_recipients no mapping",
			in: `
            [role_to_recipients]
            `,
			expectRecipients: RecipientsMap{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			w := wrapRecipientsMap{}
			err := toml.Unmarshal([]byte(tc.in), &w)
			require.NoError(t, err)

			require.Equal(t, tc.expectRecipients, w.RecipientsMap)
		})
	}
}

func TestRecipientsMapGetRecipients(t *testing.T) {
	testCases := []struct {
		desc               string
		m                  RecipientsMap
		roles              []string
		suggestedReviewers []string
		output             []string
	}{
		{
			desc: "test match exact role",
			m: RecipientsMap{
				"dev": []string{"chanDev"},
				"*":   []string{"chanA", "chanB"},
			},
			roles:              []string{"dev"},
			suggestedReviewers: []string{},
			output:             []string{"chanDev"},
		},
		{
			desc: "test only default recipient",
			m: RecipientsMap{
				"*": []string{"chanA", "chanB"},
			},
			roles:              []string{"dev"},
			suggestedReviewers: []string{},
			output:             []string{"chanA", "chanB"},
		},
		{
			desc: "test deduplicate recipients",
			m: RecipientsMap{
				"dev": []string{"chanA", "chanB"},
				"*":   []string{"chanC"},
			},
			roles:              []string{"dev"},
			suggestedReviewers: []string{"chanA", "chanB"},
			output:             []string{"chanA", "chanB"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			recipients := tc.m.GetRecipientsFor(tc.roles, tc.suggestedReviewers)
			require.ElementsMatch(t, recipients, tc.output)
		})
	}
}
