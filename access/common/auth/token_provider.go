package auth

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/teleport-plugins/access/common/auth/oauth"
	"github.com/gravitational/teleport-plugins/access/common/auth/state"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
)

const defaultRefreshRetryInterval = 1 * time.Minute
const defaultTokenBufferInterval = 1 * time.Hour

// AccessTokenProvider provides a method to get the bearer token
// for use when authorizing to a 3rd-party provider API.
type AccessTokenProvider interface {
	GetAccessToken() (string, error)
}

// StaticAccessTokenProvider is an implementation of AccessTokenProvider
// that always returns the specified token.
type StaticAccessTokenProvider struct {
	token string
}

// NewStaticAccessTokenProvider creates a new StaticAccessTokenProvider.
func NewStaticAccessTokenProvider(token string) *StaticAccessTokenProvider {
	return &StaticAccessTokenProvider{token: token}
}

// GetAccessToken implements AccessTokenProvider
func (s *StaticAccessTokenProvider) GetAccessToken() (string, error) {
	return s.token, nil
}

// RotatedAccessTokenProviderConfig contains parameters and dependencies for RotatedAccessTokenProvider
type RotatedAccessTokenProviderConfig struct {
	Ctx                 context.Context
	RetryInterval       time.Duration
	TokenBufferInterval time.Duration

	State     state.State
	Refresher oauth.Refresher
	Clock     clockwork.Clock

	Log *logrus.Entry
}

// CheckAndSetDefaults validates a configuration and sets default values
func (c *RotatedAccessTokenProviderConfig) CheckAndSetDefaults() error {
	if c.Ctx == nil {
		return trace.BadParameter("Ctx must be set")
	}
	if c.RetryInterval == 0 {
		c.RetryInterval = defaultRefreshRetryInterval
	}
	if c.TokenBufferInterval == 0 {
		c.TokenBufferInterval = defaultTokenBufferInterval
	}

	if c.State == nil {
		return trace.BadParameter("State must be set")
	}
	if c.Refresher == nil {
		return trace.BadParameter("Refresher must be set")
	}
	if c.Clock == nil {
		c.Clock = clockwork.NewRealClock()
	}
	if c.Log == nil {
		c.Log = logrus.NewEntry(logrus.StandardLogger())
	}
	return nil
}

// RotatedAccessTokenProvider is an implementation of AccessTokenProvider
// that uses OAuth2 refresh token flow to renew the acess token.
// The credentials are stored in the given persistent store.
//
// To have an up-to-date token, one must run RefreshLoop() in a background goroutine.
type RotatedAccessTokenProvider struct {
	ctx                 context.Context
	retryInterval       time.Duration
	tokenBufferInterval time.Duration
	state               state.State
	refresher           oauth.Refresher
	clock               clockwork.Clock

	log logrus.FieldLogger

	lock  sync.RWMutex // protects the below fields
	creds *state.Credentials
}

// NewRotatedTokenProvider creates a new RotatedAccessTokenProvider from the given config.
// NewRotatedTokenProvider will return an error if the state does not have existing credentials,
// meaning they need to be acquired first (e.g. via OAuth2 authorization code flow).
func NewRotatedTokenProvider(cfg RotatedAccessTokenProviderConfig) (*RotatedAccessTokenProvider, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	provider := &RotatedAccessTokenProvider{
		ctx:                 cfg.Ctx,
		retryInterval:       cfg.RetryInterval,
		tokenBufferInterval: cfg.TokenBufferInterval,
		state:               cfg.State,
		refresher:           cfg.Refresher,
		clock:               cfg.Clock,
		log:                 cfg.Log,
	}

	err := provider.init()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return provider, nil
}

func (r *RotatedAccessTokenProvider) init() error {
	var err error

	r.creds, err = r.state.GetCredentials(r.ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// GetAccessToken implements AccessTokenProvider()
func (r *RotatedAccessTokenProvider) GetAccessToken() (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.creds.AccessToken, nil
}

// RefreshLoop runs the credential refresh process.
func (r *RotatedAccessTokenProvider) RefreshLoop() {
	r.lock.RLock()
	creds := r.creds
	r.lock.RUnlock()

	period := r.getRefreshInterval(creds)

	timer := r.clock.NewTimer(period)
	defer timer.Stop()
	r.log.Infof("Will attempt token refresh in: %s", period)

	for {
		select {
		case <-r.ctx.Done():
			r.log.Info("Shutting down")
			return
		case <-timer.Chan():
			creds, _ := r.state.GetCredentials(r.ctx)

			// Skip if the credentials are sufficiently fresh
			// (in a HA setup another instance might have refreshed the credentials).
			if creds != nil && !r.shouldRefresh(creds) {
				r.lock.Lock()
				r.creds = creds
				r.lock.Unlock()

				period := r.getRefreshInterval(creds)
				timer.Reset(period)
				r.log.Infof("Next refresh in: %s", period)
				continue
			}

			creds, err := r.refresh(r.ctx)
			if err != nil {
				r.log.Errorf("Error while refreshing: %s", err)
				timer.Reset(r.retryInterval)
			} else {
				err := r.state.PutCredentials(r.ctx, creds)
				if err != nil {
					r.log.Errorf("Error while storing the refreshed credentials: %s", err)
					timer.Reset(r.retryInterval)
					continue
				}

				r.lock.Lock()
				r.creds = creds
				r.lock.Unlock()

				period := r.getRefreshInterval(creds)
				timer.Reset(period)
				r.log.Infof("Successfully refreshed credentials. Next refresh in: %s", period)
			}
		}
	}
}

func (r *RotatedAccessTokenProvider) getRefreshInterval(creds *state.Credentials) time.Duration {
	d := creds.ExpiresAt.Sub(r.clock.Now()) - r.tokenBufferInterval

	// Ticker panics of duration is negative
	if d < 0 {
		d = time.Duration(1)
	}
	return d
}

func (r *RotatedAccessTokenProvider) refresh(ctx context.Context) (*state.Credentials, error) {
	creds, err := r.refresher.Refresh(ctx, r.creds.RefreshToken)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return creds, nil
}

func (r *RotatedAccessTokenProvider) shouldRefresh(creds *state.Credentials) bool {
	now := r.clock.Now()
	refreshAt := creds.ExpiresAt.Add(-r.tokenBufferInterval)
	return now.After(refreshAt) || now.Equal(refreshAt)
}
