package teleport

import (
	"context"

	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
)

type Client interface {
	plugindata.Client
	types.Events
	Ping(context.Context) (proto.PingResponse, error)
}
