package auth

import (
	"context"
	"testing"
	"time"

	"github.com/gravitational/teleport-plugins/access/common/auth/oauth"
	"github.com/gravitational/teleport-plugins/access/common/auth/storage"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type mockRefresher struct {
	refresh func(string) (*storage.Credentials, error)
}

// Refresh implements oauth.Refresher
func (r *mockRefresher) Refresh(ctx context.Context, refreshToken string) (*storage.Credentials, error) {
	return r.refresh(refreshToken)
}

type mockState struct {
	getCredentials func() (*storage.Credentials, error)
	putCredentials func(*storage.Credentials) error
}

// GetCredentials implements storage.Storage
func (s *mockState) GetCredentials(ctx context.Context) (*storage.Credentials, error) {
	return s.getCredentials()
}

// PutCredentials implements storage.Storage
func (s *mockState) PutCredentials(ctx context.Context, creds *storage.Credentials) error {
	return s.putCredentials(creds)
}

func TestRotatedAccessTokenProvider(t *testing.T) {
	log := logrus.New()
	log.Level = logrus.DebugLevel

	newProvider := func(ctx context.Context, state storage.Storage, refresher oauth.Refresher, clock clockwork.Clock, initialCreds *storage.Credentials) *RotatedAccessTokenProvider {
		return &RotatedAccessTokenProvider{
			state:     state,
			refresher: refresher,
			clock:     clock,

			retryInterval:       1 * time.Minute,
			tokenBufferInterval: 1 * time.Hour,

			creds: initialCreds,
			log:   log,
		}
	}

	t.Run("Init", func(t *testing.T) {
		clock := clockwork.NewFakeClock()
		initialCreds := &storage.Credentials{
			AccessToken:  "my-access-token",
			RefreshToken: "my-refresh-token",
			ExpiresAt:    clock.Now().Add(2 * time.Hour),
		}

		refresher := &mockRefresher{}
		mockState := &mockState{
			getCredentials: func() (*storage.Credentials, error) {
				return initialCreds, nil
			},
		}

		provider, err := NewRotatedTokenProvider(context.Background(), RotatedAccessTokenProviderConfig{
			State:     mockState,
			Refresher: refresher,
			Clock:     clock,
		})
		require.NoError(t, err)
		creds, err := provider.GetAccessToken()
		require.NoError(t, err)
		require.Equal(t, initialCreds.AccessToken, creds)
	})

	t.Run("InitFail", func(t *testing.T) {
		clock := clockwork.NewFakeClock()
		refresher := &mockRefresher{}
		mockState := &mockState{
			getCredentials: func() (*storage.Credentials, error) {
				return nil, trace.NotFound("not found")
			},
		}

		provider, err := NewRotatedTokenProvider(context.Background(), RotatedAccessTokenProviderConfig{
			State:     mockState,
			Refresher: refresher,
			Clock:     clock,
		})
		require.Error(t, err)
		require.Nil(t, provider)
	})

	t.Run("Refresh", func(t *testing.T) {
		clock := clockwork.NewFakeClock()
		initialCreds := &storage.Credentials{
			AccessToken:  "my-access-token",
			RefreshToken: "my-refresh-token",
			ExpiresAt:    clock.Now().Add(2 * time.Hour),
		}
		newCreds := &storage.Credentials{
			AccessToken:  "my-access-token2",
			RefreshToken: "my-refresh-token2",
			ExpiresAt:    clock.Now().Add(4 * time.Hour),
		}

		var storedCreds *storage.Credentials
		var refreshCalled int

		refresher := &mockRefresher{
			refresh: func(refreshToken string) (*storage.Credentials, error) {
				refreshCalled++
				require.Equal(t, refreshToken, initialCreds.RefreshToken)

				// fail the first call
				if refreshCalled == 1 {
					return nil, trace.Errorf("some error")
				}

				return newCreds, nil
			},
		}
		mockState := &mockState{
			getCredentials: func() (*storage.Credentials, error) {
				return initialCreds, nil
			},
			putCredentials: func(creds *storage.Credentials) error {
				storedCreds = creds
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		provider := newProvider(ctx, mockState, refresher, clock, initialCreds)

		go provider.RefreshLoop(ctx)

		clock.BlockUntil(1)
		require.Nil(t, storedCreds) // before attempting refresh

		clock.Advance(1 * time.Hour) // trigger refresh (2 hours - 1 hour buffer)
		clock.BlockUntil(1)
		require.Nil(t, storedCreds) // after first refresh has failed

		clock.Advance(1 * time.Minute) // trigger refresh (after retry period)
		clock.BlockUntil(1)
		require.Equal(t, newCreds, storedCreds)
	})

	t.Run("Cancel", func(t *testing.T) {
		clock := clockwork.NewFakeClock()
		refresher := &mockRefresher{}
		mockState := &mockState{}

		initialCreds := &storage.Credentials{
			AccessToken:  "my-access-token",
			RefreshToken: "my-refresh-token",
			ExpiresAt:    clock.Now().Add(2 * time.Hour),
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		provider := newProvider(ctx, mockState, refresher, clock, initialCreds)
		finished := make(chan struct{}, 1)

		go func() {
			provider.RefreshLoop(ctx)
			finished <- struct{}{}
		}()

		cancel()
		require.Eventually(t, func() bool {
			select {
			case <-finished:
				return true
			default:
				return false
			}
		}, time.Second, time.Second/10)
	})
}
