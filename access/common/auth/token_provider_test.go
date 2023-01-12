package auth

import (
	"context"
	"testing"
	"time"

	"github.com/gravitational/teleport-plugins/access/common/auth/state"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

type mockRefresher struct {
	refresh func(string) (*state.Credentials, error)
}

// Refresh implements oauth.Refresher
func (r *mockRefresher) Refresh(ctx context.Context, refreshToken string) (*state.Credentials, error) {
	return r.refresh(refreshToken)
}

type mockState struct {
	getCredentials func() (*state.Credentials, error)
	putCredentials func(*state.Credentials) error
}

// GetCredentials implements state.State
func (s *mockState) GetCredentials(ctx context.Context) (*state.Credentials, error) {
	return s.getCredentials()
}

// PutCredentials implements state.State
func (s *mockState) PutCredentials(ctx context.Context, creds *state.Credentials) error {
	return s.putCredentials(creds)
}

type RotatedAccessTokenProviderSuite struct {
	suite.Suite

	refresher *mockRefresher
	state     *mockState
	clock     clockwork.FakeClock
	log       *logrus.Entry

	initialCreds *state.Credentials
}

func (r *RotatedAccessTokenProviderSuite) SetupSuite() {
	r.refresher = &mockRefresher{}
	r.state = &mockState{}
	r.clock = clockwork.NewFakeClock()
	log := logrus.New()
	log.Level = logrus.DebugLevel
	r.log = logrus.NewEntry(log)

	r.initialCreds = &state.Credentials{
		AccessToken:  "my-access-token",
		RefreshToken: "my-refresh-token",
		ExpiresAt:    r.clock.Now().Add(2 * time.Hour),
	}
}

func (r *RotatedAccessTokenProviderSuite) newProvider(ctx context.Context) *RotatedAccessTokenProvider {
	return &RotatedAccessTokenProvider{
		ctx:       ctx,
		state:     r.state,
		refresher: r.refresher,
		clock:     r.clock,

		retryInterval:       1 * time.Minute,
		tokenBufferInterval: 1 * time.Hour,

		creds: r.initialCreds,
		log:   r.log,
	}

}

func (r *RotatedAccessTokenProviderSuite) TestInit() {
	r.state.getCredentials = func() (*state.Credentials, error) {
		return r.initialCreds, nil
	}

	provider, err := NewRotatedTokenProvider(RotatedAccessTokenProviderConfig{
		Ctx:       context.Background(),
		State:     r.state,
		Refresher: r.refresher,
		Clock:     r.clock,
	})
	r.Require().NoError(err)
	creds, err := provider.GetAccessToken()
	r.Require().NoError(err)
	r.Require().Equal(r.initialCreds.AccessToken, creds)
}

func (r *RotatedAccessTokenProviderSuite) TestRefresh() {
	var storedCreds *state.Credentials

	newCreds := &state.Credentials{
		AccessToken:  "my-access-token2",
		RefreshToken: "my-refresh-token2",
		ExpiresAt:    r.clock.Now().Add(4 * time.Hour),
	}

	r.state.getCredentials = func() (*state.Credentials, error) {
		return r.initialCreds, nil
	}
	r.state.putCredentials = func(creds *state.Credentials) error {
		storedCreds = creds
		return nil
	}

	var refreshCalled int
	r.refresher.refresh = func(refreshToken string) (*state.Credentials, error) {
		refreshCalled++
		r.Require().Equal(refreshToken, r.initialCreds.RefreshToken)

		// fail the first call
		if refreshCalled == 1 {
			return nil, trace.Errorf("some error")
		}

		return newCreds, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := r.newProvider(ctx)

	go provider.RefreshLoop()

	r.clock.BlockUntil(1)
	r.Require().Nil(storedCreds) // before attempting refresh

	r.clock.Advance(1 * time.Hour) // trigger refresh (2 hours - 1 hour buffer)
	r.clock.BlockUntil(1)
	r.Require().Nil(storedCreds) // after first refresh has failed

	r.clock.Advance(1 * time.Minute) // trigger refresh (after retry period)
	r.clock.BlockUntil(1)
	r.Require().Equal(newCreds, storedCreds)
}

func (r *RotatedAccessTokenProviderSuite) TestCancel() {
	finished := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())

	provider := r.newProvider(ctx)

	go func() {
		provider.RefreshLoop()
		finished <- struct{}{}
	}()

	cancel()
	r.Require().Eventually(func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	}, time.Second, time.Second/10)
}

func TestRotatedAccessTokenProvider(t *testing.T) { suite.Run(t, &RotatedAccessTokenProviderSuite{}) }
