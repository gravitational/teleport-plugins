package main

import (
	"testing"

	"github.com/gravitational/teleport-plugins/access/ms-teams/msapi"
	"github.com/stretchr/testify/require"
)

func TestBot_CheckChannelURL(t *testing.T) {
	b := Bot{}
	tests := []struct {
		name             string
		url              string
		expectedUserData *RecipientData
		validURL         bool
	}{
		{
			name: "Valid URL",
			url:  "https://teams.microsoft.com/l/channel/19%3ae06a7383ed98468f90217a35fa1980d7%40thread.tacv2/Approval%2520Channel%25202?groupId=f2b3c8ed-5502-4449-b76f-dc3acea81f1c&tenantId=ff882432-09b0-437b-bd22-ca13c0037ded",
			expectedUserData: &RecipientData{
				ID:  "ff882432-09b0-437b-bd22-ca13c0037ded/f2b3c8ed-5502-4449-b76f-dc3acea81f1c/Approval%20Channel%202",
				App: msapi.InstalledApp{},
				Chat: msapi.Chat{
					ID:       "19:e06a7383ed98468f90217a35fa1980d7@thread.tacv2",
					TenantID: "ff882432-09b0-437b-bd22-ca13c0037ded",
					WebURL:   "https://teams.microsoft.com/l/channel/19%3ae06a7383ed98468f90217a35fa1980d7%40thread.tacv2/Approval%2520Channel%25202?groupId=f2b3c8ed-5502-4449-b76f-dc3acea81f1c&tenantId=ff882432-09b0-437b-bd22-ca13c0037ded",
				},
			},
			validURL: true,
		},
		{
			name:             "Invalid URL (no tenant)",
			url:              "https://teams.microsoft.com/l/channel/19%3ae06a7383ed98468f90217a35fa1980d7%40thread.tacv2/Approval%2520Channel%25202?groupId=f2b3c8ed-5502-4449-b76f-dc3acea81f1c",
			expectedUserData: nil,
			validURL:         false,
		},
		{
			name:             "Invalid URL (wrong length)",
			url:              "https://teams.microsoft.com/channel/19%3ae06a7383ed98468f90217a35fa1980d7%40thread.tacv2/Approval%2520Channel%25202?groupId=f2b3c8ed-5502-4449-b76f-dc3acea81f1c&tenantId=ff882432-09b0-437b-bd22-ca13c0037ded",
			expectedUserData: nil,
			validURL:         false,
		},
		{
			name:             "Email",
			url:              "foo@example.com",
			expectedUserData: nil,
			validURL:         false,
		},
		{
			name:             "Not an URL",
			url:              "This is not an url 🙂",
			expectedUserData: nil,
			validURL:         false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, ok := b.checkChannelURL(tc.url)
			require.Equal(t, tc.validURL, ok)
			if tc.validURL {
				require.Equal(t, tc.expectedUserData, &data)
			}
		})
	}
}
