package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
)

func TestRecipients(t *testing.T) {
	testCases := []struct {
		desc             string
		in               string
		expectErr        require.ErrorAssertionFunc
		expectRecipients RecipientsMap
	}{
		{
			desc: "test recipients",
			in: `
			[slack]
			token = "token"
			recipients = ["dev-channel","admin-channel"]
			`,
			expectRecipients: RecipientsMap{
				types.Wildcard: []string{"dev-channel", "admin-channel"},
			},
		},
		{
			desc: "test recipients_map",
			in: `
			[slack]
			token = "token"

			[recipients_map]
			"dev" = ["dev-channel","admin-channel"]
			"*" = "admin-channel"
			`,
			expectRecipients: RecipientsMap{
				"dev":          []string{"dev-channel", "admin-channel"},
				types.Wildcard: []string{"admin-channel"},
			},
		},
		{
			desc: "test no recipients or recipients_map",
			in: `
			[slack]
			token = "token"
			`,
			expectErr: func(tt require.TestingT, e error, i ...interface{}) {
				require.Error(t, e)
				require.True(t, trace.IsBadParameter(e))
			},
		},
		{
			desc: "test recipients and recipients_map",
			in: `
			[slack]
			token = "token"
			recipients = ["dev-channel","admin-channel"]

			[recipients_map]
			"dev" = ["dev-channel","admin-channel"]
			"*" = "admin-channel"
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
			require.Equal(t, tc.expectRecipients, c.RecipientsMap)
		})
	}
}
