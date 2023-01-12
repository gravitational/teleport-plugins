package auth

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/teleport-plugins/access/common/auth/oauth"
	"github.com/gravitational/teleport-plugins/access/common/auth/state"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

const defaultRefreshRetryPeriod = 1 * time.Minute
const tokenRotationBufferPeriod = 1 * time.Hour

type AccessTokenProvider interface {
	GetAccessToken() (string, error)
}

type StaticAccessTokenProvider struct {
	token string
}

func NewStaticAccessTokenProvider(token string) *StaticAccessTokenProvider {
	return &StaticAccessTokenProvider{token: token}
}

func (s *StaticAccessTokenProvider) GetAccessToken() (string, error) {
	return s.token, nil
}

type RotatedAccessTokenProvider struct {
	ctx         context.Context
	retryPeriod time.Duration
	state       state.State
	authorizer  oauth.Authorizer

	log logrus.FieldLogger

	lock  sync.RWMutex // protects the below fields
	creds *state.Credentials
}

func NewRotatedTokenProvider(ctx context.Context, state state.State, authorizer oauth.Authorizer) (*RotatedAccessTokenProvider, error) {
	provider := &RotatedAccessTokenProvider{
		ctx:         ctx,
		retryPeriod: defaultRefreshRetryPeriod,
		state:       state,
		authorizer:  authorizer,
		log:         logger.Standard(),
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

func (r *RotatedAccessTokenProvider) GetAccessToken() (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.creds.AccessToken, nil
}

func (r *RotatedAccessTokenProvider) RefreshLoop() {
	r.lock.RLock()
	creds := r.creds
	r.lock.RUnlock()

	period := r.getRefreshPeriod(creds)

	timer := time.NewTimer(period)
	defer timer.Stop()
	r.log.Debugf("Will attempt token refresh in: %s", period)

	for {
		select {
		case <-r.ctx.Done():
			r.log.Debug("Shutting down")
			return
		case <-timer.C:
			creds, _ := r.state.GetCredentials(r.ctx)

			// Skip if the credentials are sufficiently fresh
			// (in a HA setup another instance might have refreshed the credentials).
			if creds != nil && !r.shouldRefresh(creds) {
				r.lock.Lock()
				r.creds = creds
				r.lock.Unlock()

				period := r.getRefreshPeriod(creds)
				timer.Reset(period)
				r.log.Debugf("Next refresh in: %s", period)
				continue
			}

			creds, err := r.refresh(r.ctx)
			if err != nil {
				r.log.Errorf("Error while refreshing: %s", err)
				timer.Reset(r.retryPeriod)
			} else {
				err := r.state.PutCredentials(r.ctx, creds)
				if err != nil {
					r.log.Errorf("Error while storing the refreshed credentials: %s", err)
					timer.Reset(r.retryPeriod)
					continue
				}

				r.lock.Lock()
				r.creds = creds
				r.lock.Unlock()

				period := r.getRefreshPeriod(creds)
				timer.Reset(period)
				r.log.Debugf("Successfully refreshed credentials. Next refresh in: %s", period)
			}
		}
	}
}

func (r *RotatedAccessTokenProvider) getRefreshPeriod(creds *state.Credentials) time.Duration {
	d := creds.ExpiresAt.Sub(time.Now()) - tokenRotationBufferPeriod

	// Ticker panics of duration is negative
	if d < 0 {
		d = time.Duration(1)
	}
	return d
}

func (r *RotatedAccessTokenProvider) refresh(ctx context.Context) (*state.Credentials, error) {
	creds, err := r.authorizer.Refresh(ctx, r.creds.RefreshToken)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return creds, nil
}

func (r *RotatedAccessTokenProvider) shouldRefresh(creds *state.Credentials) bool {
	return time.Now().After(creds.ExpiresAt.Add(-tokenRotationBufferPeriod))
}
