package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/teleport-plugins/access/config"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
)

func TestRecipients(t *testing.T) {
	testCases := []struct {
		desc             string
		in               string
		expectErr        require.ErrorAssertionFunc
		expectRecipients config.RecipientsMap
	}{
		{
			desc: "test delivery recipients",
			in: `
            [mailgun]
            domain = "x"
            private_key = "y"
            [delivery]
            sender = "email@example.org"
			recipients = ["email1@example.org","email2@example.org"]
			`,
			expectRecipients: config.RecipientsMap{
				types.Wildcard: []string{"email1@example.org", "email2@example.org"},
			},
		},
		{
			desc: "test role_to_recipients",
			in: `
            [mailgun]
            domain = "x"
            private_key = "y"
            [delivery]
            sender = "email@example.org"

			[role_to_recipients]
			"dev" = ["dev@example.org","sre@example.org"]
			"*" = "admin@example.org"
			`,
			expectRecipients: config.RecipientsMap{
				"dev":          []string{"dev@example.org", "sre@example.org"},
				types.Wildcard: []string{"admin@example.org"},
			},
		},
		{
			desc: "test role_to_recipients but no wildcard",
			in: `
            [mailgun]
            domain = "x"
            private_key = "y"
            [delivery]
            sender = "email@example.org"

			[role_to_recipients]
			"dev" = ["dev@example.org","sre@example.org"]
			`,
			expectErr: func(tt require.TestingT, e error, i ...interface{}) {
				require.Error(t, e)
				require.True(t, trace.IsBadParameter(e))
			},
		},
		{
			desc: "test role_to_recipients with wildcard but empty list of recipients",
			in: `
            [mailgun]
            domain = "x"
            private_key = "y"
            [delivery]
            sender = "email@example.org"

			[role_to_recipients]
            "dev" = "email@example.org"
			"*" = []
			`,
			expectErr: func(tt require.TestingT, e error, i ...interface{}) {
				require.Error(t, e)
				require.True(t, trace.IsBadParameter(e))
			},
		},
		{
			desc: "test no recipients or role_to_recipients",
			in: `
            [mailgun]
            domain = "x"
            private_key = "y"
            [delivery]
            sender = "email@example.org"
			`,
			expectErr: func(tt require.TestingT, e error, i ...interface{}) {
				require.Error(t, e)
				require.True(t, trace.IsBadParameter(e))
			},
		},
		{
			desc: "test recipients and role_to_recipients",
			in: `
			[slack]
			token = "token"
			recipients = ["dev@example.org","admin@example.org"]

			[role_to_recipients]
			"dev" = ["dev@example.org","admin@example.org"]
			"*" = "admin@example.org"
			`,
			expectErr: func(tt require.TestingT, e error, i ...interface{}) {
				require.Error(t, e)
				require.True(t, trace.IsBadParameter(e))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			filePath := filepath.Join(t.TempDir(), "config_test.toml")
			err := os.WriteFile(filePath, []byte(tc.in), 0777)
			require.NoError(t, err)

			c, err := LoadConfig(filePath)
			if tc.expectErr != nil {
				tc.expectErr(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectRecipients, c.RoleToRecipients)
		})
	}
}
