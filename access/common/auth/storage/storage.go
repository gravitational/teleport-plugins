package storage

import (
	"context"
	"time"
)

// Credentials represents the short-lived OAuth2 credentials.
type Credentials struct {
	// AccessToken is the Bearer token used to access the provider's API
	AccessToken string
	// RefreshToken is used to acquire a new access token.
	RefreshToken string
	// ExpiresAt marks the end of validity period for the access token.
	// The application must use the refresh token to acquire a new access token
	// before this time.
	ExpiresAt time.Time
}

// Store defines the interface for persisting the short-lived OAuth2 credentials.
type Store interface {
	GetCredentials(context.Context) (*Credentials, error)
	PutCredentials(context.Context, *Credentials) error
}
