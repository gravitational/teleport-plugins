package slack

import (
	"context"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gravitational/teleport-plugins/access/common/auth/oauth"
	"github.com/gravitational/teleport-plugins/access/common/auth/state"
	"github.com/gravitational/trace"
)

// Authorizer implements oauth2.Authorizer for Slack API.
type Authorizer struct {
	client *resty.Client

	clientID     string
	clientSecret string
}

// NewAuthorizer returns a new Authorizer
func NewAuthorizer(clientID string, clientSecret string) *Authorizer {
	client := makeSlackClient(slackAPIURL)
	return &Authorizer{
		client:       client,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// Exchange implements oauth.Exchanger
func (a *Authorizer) Exchange(ctx context.Context, authorizationCode string, redirectURI string) (*state.Credentials, error) {
	var result AccessResponse

	_, err := a.client.R().
		SetQueryParam("client_id", a.clientID).
		SetQueryParam("client_secret", a.clientSecret).
		SetQueryParam("code", authorizationCode).
		SetQueryParam("redirect_uri", redirectURI).
		SetResult(&result).
		Post("oauth.v2.access")

	if err != nil {
		return nil, trace.Wrap(err)
	}

	if !result.Ok {
		return nil, trace.Errorf("%s", result.Error)
	}

	return &state.Credentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(result.ExpiresInSeconds) * time.Second),
	}, nil
}

// Refresh implements oauth.Refresher
func (a *Authorizer) Refresh(ctx context.Context, refreshToken string) (*state.Credentials, error) {
	var result AccessResponse
	_, err := a.client.R().
		SetQueryParam("client_id", a.clientID).
		SetQueryParam("client_secret", a.clientSecret).
		SetQueryParam("grant_type", "refresh_token").
		SetQueryParam("refresh_token", refreshToken).
		SetResult(&result).
		Post("oauth.v2.access")

	if err != nil {
		return nil, trace.Wrap(err)
	}

	if !result.Ok {
		return nil, trace.Errorf("%s", result.Error)
	}

	return &state.Credentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(result.ExpiresInSeconds) * time.Second),
	}, nil
}

var _ oauth.Authorizer = &Authorizer{}
