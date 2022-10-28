package main

import (
	"testing"

	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/stretchr/testify/assert"
)

var samplePluginData = PluginData{
	AccessRequestData: plugindata.AccessRequestData{
		User:             "user-foo",
		Roles:            []string{"role-foo", "role-bar"},
		RequestReason:    "foo reason",
		ReviewsCount:     3,
		ResolutionTag:    plugindata.ResolvedApproved,
		ResolutionReason: "foo ok",
	},
	TeamsData: []TeamsMessage{
		{ID: "CHANNEL1", Timestamp: "0000001", RecipientID: "foo@example.com"},
		{ID: "CHANNEL2", Timestamp: "0000002", RecipientID: "2ca235ec-37d0-44b0-964d-ca359e770603"},
	},
}

func TestEncodePluginData(t *testing.T) {
	dataMap := EncodePluginData(samplePluginData)
	assert.Len(t, dataMap, 7)
	assert.Equal(t, "user-foo", dataMap["user"])
	assert.Equal(t, "role-foo,role-bar", dataMap["roles"])
	assert.Equal(t, "foo reason", dataMap["request_reason"])
	assert.Equal(t, "3", dataMap["reviews_count"])
	assert.Equal(t, "APPROVED", dataMap["resolution"])
	assert.Equal(t, "foo ok", dataMap["resolve_reason"])
	assert.Equal(t, "CHANNEL1/0000001/foo@example.com,CHANNEL2/0000002/2ca235ec-37d0-44b0-964d-ca359e770603", dataMap["messages"])
}

func TestDecodePluginData(t *testing.T) {
	pluginData := DecodePluginData(map[string]string{
		"user":           "user-foo",
		"roles":          "role-foo,role-bar",
		"request_reason": "foo reason",
		"reviews_count":  "3",
		"resolution":     "APPROVED",
		"resolve_reason": "foo ok",
		"messages":       "CHANNEL1/0000001/foo@example.com,CHANNEL2/0000002/2ca235ec-37d0-44b0-964d-ca359e770603",
	})
	assert.Equal(t, samplePluginData, pluginData)
}

func TestEncodeEmptyPluginData(t *testing.T) {
	dataMap := EncodePluginData(PluginData{})
	assert.Len(t, dataMap, 7)
	for key, value := range dataMap {
		assert.Emptyf(t, value, "value at key %q must be empty", key)
	}
}

func TestDecodeEmptyPluginData(t *testing.T) {
	assert.Empty(t, DecodePluginData(nil))
	assert.Empty(t, DecodePluginData(make(map[string]string)))
}
