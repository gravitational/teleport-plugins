package slack

import (
	"context"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gravitational/teleport-plugins/access/common/auth/oauth"
	"github.com/gravitational/teleport-plugins/access/common/auth/state"
	"github.com/gravitational/trace"
)

type Authorizer struct {
	client *resty.Client

	clientID     string
	clientSecret string
}

func NewAuthorizer(clientID string, clientSecret string) *Authorizer {
	// TODO: Deduplicate with (*SlackConfig).NewBot
	client := resty.
		NewWithClient(&http.Client{
			Timeout: slackHTTPTimeout,
			Transport: &http.Transport{
				MaxConnsPerHost:     slackMaxConns,
				MaxIdleConnsPerHost: slackMaxConns,
			},
		}).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHostURL("https://slack.com/api/")

	return &Authorizer{
		client:       client,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// Exchange implements oauth.Authorizer
func (a *Authorizer) Exchange(ctx context.Context, authorizationCode string, redirectURI string) (*state.Credentials, error) {
	var result AccessResponse

	_, err := a.client.R().
		SetQueryParam("client_id", a.clientID).
		SetQueryParam("client_secret", a.clientSecret).
		SetQueryParam("code", authorizationCode).
		SetQueryParam("redirect_uri", redirectURI).
		SetResult(&result).
		Post("https://slack.com/api/oauth.v2.access")

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

// Refresh implements oauth.Authorizer
func (a *Authorizer) Refresh(ctx context.Context, refreshToken string) (*state.Credentials, error) {
	var result AccessResponse
	_, err := a.client.R().
		SetQueryParam("client_id", a.clientID).
		SetQueryParam("client_secret", a.clientSecret).
		SetQueryParam("grant_type", "refresh_token").
		SetQueryParam("refresh_token", refreshToken).
		SetResult(&result).
		Post("https://slack.com/api/oauth.v2.access")

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
