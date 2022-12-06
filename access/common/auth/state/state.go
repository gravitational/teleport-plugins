package state

import (
	"context"
	"time"
)

type Credentials struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type State interface {
	GetCredentials(context.Context) (*Credentials, error)
	PutCredentials(context.Context, *Credentials) error
}
