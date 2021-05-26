package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddrToURL(t *testing.T) {
	url, err := AddrToURL("foo")
	assert.NoError(t, err)
	assert.Equal(t, "https://foo", url.String())

	url, err = AddrToURL("foo:443")
	assert.NoError(t, err)
	assert.Equal(t, "https://foo", url.String())

	url, err = AddrToURL("foo:3080")
	assert.NoError(t, err)
	assert.Equal(t, "https://foo:3080", url.String())
}
