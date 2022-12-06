package oauth

import (
	"context"

	"github.com/gravitational/teleport-plugins/access/common/auth/state"
)

type Authorizer interface {
	Exchanger
	Refresher
}

type Exchanger interface {
	Exchange(ctx context.Context, authorizationCode string, redirectURI string) (*state.Credentials, error)
}

type Refresher interface {
	Refresh(ctx context.Context, refreshToken string) (*state.Credentials, error)
}
