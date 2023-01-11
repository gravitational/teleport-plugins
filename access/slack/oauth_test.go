package slack

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gravitational/teleport-plugins/lib/testing/integration"
	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/suite"
)

type OAuthSuite struct {
	integration.Suite

	// Request parameters
	clientID          string
	clientSecret      string
	authorizationCode string
	redirectURI       string
	refreshToken      string

	// Response parameters
	exchangedAccessToken  string
	exchangedRefreshToken string
	refreshedAccessToken  string
	refreshedRefreshToken string
	expiresInSeconds      int

	exchangeError error
	refreshError  error

	srv        *httptest.Server
	authorizer *Authorizer
}

func (s *OAuthSuite) SetupSuite() {
	s.clientID = "my-client-id"
	s.clientSecret = "my-client-secret"
	s.authorizationCode = "12345678"
	s.redirectURI = "https://foobar.com/callback"
	s.refreshToken = "my-refresh-token1"
	s.exchangedAccessToken = "my-access-token1"
	s.exchangedRefreshToken = "my-refresh-token2"
	s.refreshedAccessToken = "my-access-token2"
	s.refreshedRefreshToken = "my-refresh-token3"
	s.expiresInSeconds = 43200

	exchange := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.Require().Equal(s.clientID, q.Get("client_id"))
		s.Require().Equal(s.clientSecret, q.Get("client_secret"))
		s.Require().Equal(s.redirectURI, q.Get("redirect_uri"))
		s.Require().Equal(s.authorizationCode, q.Get("code"))

		resp := AccessResponse{
			APIResponse: APIResponse{Ok: true},
		}
		if s.exchangeError != nil {
			resp.Ok = false
			resp.Error = s.exchangeError.Error()
		} else {
			resp.AccessToken = s.exchangedAccessToken
			resp.RefreshToken = s.exchangedRefreshToken
			resp.ExpiresInSeconds = s.expiresInSeconds
		}

		w.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(resp)
		s.Require().NoError(err)
	}

	refresh := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.Require().Equal(s.clientID, q.Get("client_id"))
		s.Require().Equal(s.clientSecret, q.Get("client_secret"))
		s.Require().Equal(s.refreshToken, q.Get("refresh_token"))

		resp := AccessResponse{
			APIResponse: APIResponse{Ok: true},
		}
		if s.refreshError != nil {
			resp.Ok = false
			resp.Error = s.refreshError.Error()
		} else {
			resp.AccessToken = s.refreshedAccessToken
			resp.RefreshToken = s.refreshedRefreshToken
			resp.ExpiresInSeconds = s.expiresInSeconds
		}

		w.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(resp)
		s.Require().NoError(err)
	}

	handler := func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		if grantType := r.URL.Query().Get("grant_type"); grantType == "refresh_token" {
			refresh(w, r)
		} else {
			exchange(w, r)
		}
	}
	router := httprouter.New()
	router.POST("/oauth.v2.access", handler)

	s.srv = httptest.NewServer(router)
	s.authorizer = newAuthorizer(makeSlackClient(s.srv.URL+"/"), s.clientID, s.clientSecret)
}

func (s *OAuthSuite) SetupTest() {
	s.exchangeError = nil
	s.refreshError = nil
}

func (s *OAuthSuite) TearDownSuite() {
	s.srv.Close()
}

func (s *OAuthSuite) TestExchangeOK() {
	creds, err := s.authorizer.Exchange(s.Context(), s.authorizationCode, s.redirectURI)
	s.Require().NoError(err)
	s.Require().Equal(s.exchangedAccessToken, creds.AccessToken)
	s.Require().Equal(s.exchangedRefreshToken, creds.RefreshToken)
	s.Require().WithinDuration(time.Now().Add(time.Duration(s.expiresInSeconds)*time.Second), creds.ExpiresAt, 1*time.Second)
}

func (s *OAuthSuite) TestExchangeError() {
	s.exchangeError = errors.New("invalid_code")
	_, err := s.authorizer.Exchange(s.Context(), s.authorizationCode, s.redirectURI)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "invalid_code")
}

func (s *OAuthSuite) TestRefreshOK() {
	creds, err := s.authorizer.Refresh(s.Context(), s.refreshToken)
	s.Require().NoError(err)
	s.Require().Equal(s.refreshedAccessToken, creds.AccessToken)
	s.Require().Equal(s.refreshedRefreshToken, creds.RefreshToken)
	s.Require().WithinDuration(time.Now().Add(time.Duration(s.expiresInSeconds)*time.Second), creds.ExpiresAt, 1*time.Second)
}

func (s *OAuthSuite) TestRefreshError() {
	s.refreshError = errors.New("expired_token")
	_, err := s.authorizer.Refresh(s.Context(), s.refreshToken)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "expired_token")
}

func TestOAuth(t *testing.T) { suite.Run(t, &OAuthSuite{}) }
