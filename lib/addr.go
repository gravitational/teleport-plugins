package lib

import (
	"net/url"
	"strings"

	"github.com/gravitational/trace"
)

func AddrToURL(addr string) (*url.URL, error) {
	var (
		result *url.URL
		err    error
	)
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "https://" + addr
	}
	if result, err = url.Parse(addr); err != nil {
		return nil, trace.Wrap(err)
	}
	if result.Scheme == "https" && result.Port() == "443" {
		// Cut off redundant :443
		result.Host = result.Hostname()
	}
	return result, nil
}
