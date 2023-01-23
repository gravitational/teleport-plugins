package oauth

import (
	"context"

	storage "github.com/gravitational/teleport-plugins/access/common/auth/storage"
)

// Authorizer is the composite interface of Exchanger and Refresher.
type Authorizer interface {
	Exchanger
	Refresher
}

// Exchanger implements the OAuth2 authorization code exchange operation.
type Exchanger interface {
	Exchange(ctx context.Context, authorizationCode string, redirectURI string) (*storage.Credentials, error)
}

// Refresher implements the OAuth2 bearer token refresh operation.
type Refresher interface {
	Refresh(ctx context.Context, refreshToken string) (*storage.Credentials, error)
}
