package plugindata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var sampleAccessRequestData = AccessRequestData{
	User:             "user-foo",
	Roles:            []string{"role-foo", "role-bar"},
	RequestReason:    "foo reason",
	ReviewsCount:     3,
	ResolutionTag:    ResolvedApproved,
	ResolutionReason: "foo ok",
}

func TestEncodeAccessRequestData(t *testing.T) {
	dataMap := EncodeAccessRequestData(sampleAccessRequestData)
	assert.Len(t, dataMap, 6)
	assert.Equal(t, "user-foo", dataMap["user"])
	assert.Equal(t, "role-foo,role-bar", dataMap["roles"])
	assert.Equal(t, "foo reason", dataMap["request_reason"])
	assert.Equal(t, "3", dataMap["reviews_count"])
	assert.Equal(t, "APPROVED", dataMap["resolution"])
	assert.Equal(t, "foo ok", dataMap["resolve_reason"])
}

func TestDecodeAccessRequestData(t *testing.T) {
	pluginData := DecodeAccessRequestData(map[string]string{
		"user":           "user-foo",
		"roles":          "role-foo,role-bar",
		"request_reason": "foo reason",
		"reviews_count":  "3",
		"resolution":     "APPROVED",
		"resolve_reason": "foo ok",
	})
	assert.Equal(t, sampleAccessRequestData, pluginData)
}

func TestEncodeEmptyAccessRequestData(t *testing.T) {
	dataMap := EncodeAccessRequestData(AccessRequestData{})
	assert.Len(t, dataMap, 6)
	for key, value := range dataMap {
		assert.Emptyf(t, value, "value at key %q must be empty", key)
	}
}

func TestDecodeEmptyAccessRequestData(t *testing.T) {
	assert.Empty(t, DecodeAccessRequestData(nil))
	assert.Empty(t, DecodeAccessRequestData(make(map[string]string)))
}
