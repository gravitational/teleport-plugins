package lib

import (
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/trace"

	"github.com/hashicorp/go-version"
)

// AssertServerVersion returns an error if server version in ping response is
// less than minimum required version.
func AssertServerVersion(pong proto.PingResponse, minVersion string) error {
	actual, err := version.NewVersion(pong.ServerVersion)
	if err != nil {
		return trace.Wrap(err)
	}
	required, err := version.NewVersion(minVersion)
	if err != nil {
		return trace.Wrap(err)
	}
	if actual.LessThan(required) {
		return trace.Errorf("server version %s is less than %s", pong.ServerVersion, minVersion)
	}
	return nil
}
