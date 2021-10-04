package main

import (
	"testing"

	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/stretchr/testify/require"
)

var samplePluginData = PluginData{
	RequestData: RequestData{
		User:          "user-foo",
		Roles:         []string{"role-foo", "role-bar"},
		RequestReason: "foo reason",
		ReviewsCount:  3,
		Resolution:    Resolution{Tag: ResolvedApproved, Reason: "foo ok"},
	},
	SlackData: SlackData{
		{ChannelID: "CHANNEL1", Timestamp: "0000001"},
		{ChannelID: "CHANNEL2", Timestamp: "0000002"},
	},
}

var sampleStringMap = plugindata.StringMap{
	"user":           "user-foo",
	"roles":          "role-foo,role-bar",
	"request_reason": "foo reason",
	"reviews_count":  "3",
	"resolution":     "APPROVED",
	"resolve_reason": "foo ok",
	"messages":       "CHANNEL1/0000001,CHANNEL2/0000002",
}

func TestMarshalPluginData(t *testing.T) {
	require.Equal(t, sampleStringMap, samplePluginData.MarshalPluginData())
}

func TestUnmarshalPluginData(t *testing.T) {
	var data PluginData
	data.UnmarshalPluginData(sampleStringMap)
	require.Equal(t, samplePluginData, data)
}

func TestMarshalEmptyPluginData(t *testing.T) {
	data := &PluginData{}
	dataMap := data.MarshalPluginData()
	require.Len(t, dataMap, 7)
	for key, value := range dataMap {
		require.Zerof(t, value, "value at key %q must be a zero", key)
	}
}

func TestUnmarshalEmptyPluginData(t *testing.T) {
	var data PluginData

	data.UnmarshalPluginData(nil)
	require.Zero(t, data)

	data.UnmarshalPluginData(make(map[string]string))
	require.Zero(t, data)
}
