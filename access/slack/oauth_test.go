// Copyright 2023 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/require"
)

type testOAuthServer struct {
	clientID          string
	clientSecret      string
	authorizationCode string
	redirectURI       string
	refreshToken      string

	exchangeResponse AccessResponse
	refreshResponse  AccessResponse

	srv *httptest.Server
	t   *testing.T
}

func (s *testOAuthServer) handler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if grantType := r.URL.Query().Get("grant_type"); grantType == "refresh_token" {
		s.refresh(w, r)
	} else {
		s.exchange(w, r)
	}
}

func (s *testOAuthServer) exchange(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	require.Equal(s.t, s.clientID, q.Get("client_id"))
	require.Equal(s.t, s.clientSecret, q.Get("client_secret"))
	require.Equal(s.t, s.redirectURI, q.Get("redirect_uri"))
	require.Equal(s.t, s.authorizationCode, q.Get("code"))

	w.Header().Add("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(s.exchangeResponse)
	require.NoError(s.t, err)
}

func (s *testOAuthServer) refresh(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	require.Equal(s.t, s.clientID, q.Get("client_id"))
	require.Equal(s.t, s.clientSecret, q.Get("client_secret"))
	require.Equal(s.t, s.refreshToken, q.Get("refresh_token"))

	w.Header().Add("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(s.refreshResponse)
	require.NoError(s.t, err)
}

func (s *testOAuthServer) start() {
	router := httprouter.New()
	router.POST("/oauth.v2.access", s.handler)

	s.srv = httptest.NewServer(router)
}

func (s *testOAuthServer) url() string {
	return s.srv.URL + "/"
}

func (s *testOAuthServer) close() {
	s.srv.Close()
}

func TestOAuth(t *testing.T) {
	const (
		clientID          = "my-client-id"
		clientSecret      = "my-client-secret"
		authorizationCode = "12345678"
		redirectURI       = "https://foobar.com/callback"
		refreshToken      = "my-refresh-token1"
		expiresInSeconds  = 43200
	)

	newServer := func(t *testing.T) *testOAuthServer {
		s := &testOAuthServer{
			clientID:          clientID,
			clientSecret:      clientSecret,
			authorizationCode: authorizationCode,
			redirectURI:       redirectURI,
			refreshToken:      refreshToken,

			t: t,
		}
		s.start()
		return s
	}

	ok := func(accessToken string, refreshToken string, expiresInSeconds int) AccessResponse {
		return AccessResponse{
			APIResponse:      APIResponse{Ok: true},
			AccessToken:      accessToken,
			RefreshToken:     refreshToken,
			ExpiresInSeconds: expiresInSeconds,
		}
	}

	fail := func(e string) AccessResponse {
		return AccessResponse{
			APIResponse: APIResponse{
				Ok:    false,
				Error: e,
			},
		}
	}

	t.Run("ExchangeOK", func(t *testing.T) {
		s := newServer(t)
		defer s.close()
		s.exchangeResponse = ok("my-access-token1", "my-refresh-token2", expiresInSeconds)

		authorizer := newAuthorizer(makeSlackClient(s.url()), clientID, clientSecret)

		creds, err := authorizer.Exchange(context.Background(), s.authorizationCode, s.redirectURI)
		require.NoError(t, err)
		require.Equal(t, s.exchangeResponse.AccessToken, creds.AccessToken)
		require.Equal(t, s.exchangeResponse.RefreshToken, creds.RefreshToken)
		require.WithinDuration(t, time.Now().Add(time.Duration(expiresInSeconds)*time.Second), creds.ExpiresAt, 1*time.Second)
	})

	t.Run("ExchangeFail", func(t *testing.T) {
		s := newServer(t)
		defer s.close()
		s.exchangeResponse = fail("invalid_code")

		authorizer := newAuthorizer(makeSlackClient(s.url()), clientID, clientSecret)

		_, err := authorizer.Exchange(context.Background(), s.authorizationCode, s.redirectURI)
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid_code")

	})

	t.Run("RefreshOK", func(t *testing.T) {
		s := newServer(t)
		defer s.close()
		s.refreshResponse = ok("my-access-token2", "my-refresh-token3", expiresInSeconds)

		authorizer := newAuthorizer(makeSlackClient(s.url()), clientID, clientSecret)

		creds, err := authorizer.Refresh(context.Background(), refreshToken)
		require.NoError(t, err)
		require.Equal(t, s.refreshResponse.AccessToken, creds.AccessToken)
		require.Equal(t, s.refreshResponse.RefreshToken, creds.RefreshToken)
		require.WithinDuration(t, time.Now().Add(time.Duration(expiresInSeconds)*time.Second), creds.ExpiresAt, 1*time.Second)
	})

	t.Run("RefreshFail", func(t *testing.T) {

		s := newServer(t)
		defer s.close()
		s.refreshResponse = fail("expired_token")

		authorizer := newAuthorizer(makeSlackClient(s.url()), clientID, clientSecret)

		_, err := authorizer.Refresh(context.Background(), refreshToken)
		require.Error(t, err)
		require.ErrorContains(t, err, "expired_token")
	})
}
