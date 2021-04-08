package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

type SemaphoreLockConfig struct {
	// Service is the service against which all semaphore
	// operations are performed.
	Service Semaphores
	// Expiry is an optional lease expiry parameter.
	Expiry time.Duration
	// TickRate is the rate at which lease renewals are attempted
	// and defaults to 1/2 expiry.  Used to accelerate tests.
	TickRate time.Duration
	// Params holds the semaphore lease acquisition parameters.
	Params AcquireSemaphoreRequest
}

// CheckAndSetDefaults checks and sets default parameters
func (l *SemaphoreLockConfig) CheckAndSetDefaults() error {
	if l.Service == nil {
		return trace.BadParameter("missing semaphore service")
	}
	if l.Expiry == 0 {
		l.Expiry = defaults.SessionControlTimeout
	}
	if l.Expiry < time.Millisecond {
		return trace.BadParameter("sub-millisecond lease expiry is not supported: %v", l.Expiry)
	}
	if l.TickRate == 0 {
		l.TickRate = l.Expiry / 2
	}
	if l.TickRate >= l.Expiry {
		return trace.BadParameter("tick-rate must be less than expiry")
	}
	if l.Params.Expires.IsZero() {
		l.Params.Expires = time.Now().UTC().Add(l.Expiry)
	}
	if err := l.Params.Check(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// SemaphoreLock provides a convenient interface for managing
// semaphore lease keepalive operations.
type SemaphoreLock struct {
	cfg       SemaphoreLockConfig
	lease0    SemaphoreLease
	retry     utils.Retry
	ticker    *time.Ticker
	doneC     chan struct{}
	closeOnce sync.Once
	renewalC  chan struct{}
	cond      *sync.Cond
	err       error
	fin       bool
}

// finish registers the final result of the background
// goroutine.  must be called even if err is nil in
// order to wake any goroutines waiting on the error
// and mark the lock as finished.
func (l *SemaphoreLock) finish(err error) {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()
	l.err = err
	l.fin = true
	l.cond.Broadcast()
}

// Done signals that lease keepalive operations
// have stopped.
func (l *SemaphoreLock) Done() <-chan struct{} {
	return l.doneC
}

// Wait blocks until the final result is available.  Note that
// this method may block longer than desired since cancellation of
// the parent context triggers the *start* of the release operation.
func (l *SemaphoreLock) Wait() error {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()
	for !l.fin {
		l.cond.Wait()
	}
	return l.err
}

// Stop stops associated lease keepalive.
func (l *SemaphoreLock) Stop() {
	l.closeOnce.Do(func() {
		l.ticker.Stop()
		close(l.doneC)
	})
}

// Renewed notifies on next successful lease keepalive.
// Used in tests to block until next renewal.
func (l *SemaphoreLock) Renewed() <-chan struct{} {
	return l.renewalC
}

func (l *SemaphoreLock) KeepAlive(ctx context.Context) {
	var nodrop bool
	var err error
	lease := l.lease0
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		l.Stop()
		defer l.finish(err)
		if nodrop {
			// non-standard exit conditions; don't bother handling
			// cancellation/expiry.
			return
		}
		if lease.Expires.After(time.Now().UTC()) {
			// parent context is closed. create orphan context with generous
			// timeout for lease cancellation scope.  this will not block any
			// caller that is not explicitly waiting on the final error value.
			cancelContext, cancel := context.WithTimeout(context.Background(), l.cfg.Expiry/4)
			defer cancel()
			err = l.cfg.Service.CancelSemaphoreLease(cancelContext, lease)
			if err != nil {
				log.Warnf("Failed to cancel semaphore lease %s/%s: %v", lease.SemaphoreKind, lease.SemaphoreName, err)
			}
		} else {
			log.Errorf("Semaphore lease expired: %s/%s", lease.SemaphoreKind, lease.SemaphoreName)
		}
	}()
Outer:
	for {
		select {
		case tick := <-l.ticker.C:
			leaseContext, leaseCancel := context.WithDeadline(ctx, lease.Expires)
			nextLease := lease
			nextLease.Expires = tick.Add(l.cfg.Expiry)
			for {
				err = l.cfg.Service.KeepAliveSemaphoreLease(leaseContext, nextLease)
				if trace.IsNotFound(err) {
					leaseCancel()
					// semaphore and/or lease no longer exist; best to log the error
					// and exit immediately.
					log.Warnf("Halting keepalive on semaphore %s/%s early: %v", lease.SemaphoreKind, lease.SemaphoreName, err)
					nodrop = true
					return
				}
				if err == nil {
					leaseCancel()
					lease = nextLease
					l.retry.Reset()
					select {
					case l.renewalC <- struct{}{}:
					default:
					}
					continue Outer
				}
				log.Debugf("Failed to renew semaphore lease %s/%s: %v", lease.SemaphoreKind, lease.SemaphoreName, err)
				l.retry.Inc()
				select {
				case <-l.retry.After():
				case <-leaseContext.Done():
					leaseCancel() // demanded by linter
					return
				case <-l.Done():
					leaseCancel()
					return
				}
			}
		case <-ctx.Done():
			return
		case <-l.Done():
			return
		}
	}
}

// AcquireSemaphoreLock attempts to acquire and hold a semaphore lease.  If successfully acquired,
// background keepalive processes are started and an associated lock handle is returned.  Cancelling
// the supplied context releases the semaphore.
func AcquireSemaphoreLock(ctx context.Context, cfg SemaphoreLockConfig) (*SemaphoreLock, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	// set up retry with a ratio which will result in 3-4 retries before the lease expires
	retry, err := utils.NewLinear(utils.LinearConfig{
		Max:    cfg.Expiry / 4,
		Step:   cfg.Expiry / 16,
		Jitter: utils.NewJitter(),
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	lease, err := cfg.Service.AcquireSemaphore(ctx, cfg.Params)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	lock := &SemaphoreLock{
		cfg:      cfg,
		lease0:   *lease,
		retry:    retry,
		ticker:   time.NewTicker(cfg.TickRate),
		doneC:    make(chan struct{}),
		renewalC: make(chan struct{}),
		cond:     sync.NewCond(&sync.Mutex{}),
	}
	return lock, nil
}

// SemaphoreSpecSchemaTemplate is a template for Semaphore schema.
const SemaphoreSpecSchemaTemplate = `{
	"type": "object",
	"additionalProperties": false,
	"properties": {
	  "leases": {
		"type": "array",
		"items": {
		"type": "object",
		"properties": {
		  "lease_id": { "type": "string" },
		  "expires": { "type": "string" },
		  "holder": { "type": "string" }
		  }
		}
	  }
	}
  }`

// GetSemaphoreSchema returns the validation schema for this object
func GetSemaphoreSchema() string {
	return fmt.Sprintf(V2SchemaTemplate, MetadataSchema, SemaphoreSpecSchemaTemplate, DefaultDefinitions)
}

// UnmarshalSemaphore unmarshals the Semaphore resource from JSON.
func UnmarshalSemaphore(bytes []byte, opts ...MarshalOption) (Semaphore, error) {
	var semaphore SemaphoreV3

	if len(bytes) == 0 {
		return nil, trace.BadParameter("missing resource data")
	}

	cfg, err := CollectOptions(opts)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if cfg.SkipValidation {
		if err := utils.FastUnmarshal(bytes, &semaphore); err != nil {
			return nil, trace.BadParameter(err.Error())
		}
	} else {
		err = utils.UnmarshalWithSchema(GetSemaphoreSchema(), &semaphore, bytes)
		if err != nil {
			return nil, trace.BadParameter(err.Error())
		}
	}

	err = semaphore.CheckAndSetDefaults()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if cfg.ID != 0 {
		semaphore.SetResourceID(cfg.ID)
	}
	if !cfg.Expires.IsZero() {
		semaphore.SetExpiry(cfg.Expires)
	}
	return &semaphore, nil
}

// MarshalSemaphore marshals the Semaphore resource to JSON.
func MarshalSemaphore(c Semaphore, opts ...MarshalOption) ([]byte, error) {
	cfg, err := CollectOptions(opts)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	switch resource := c.(type) {
	case *SemaphoreV3:
		if !cfg.PreserveResourceID {
			// avoid modifying the original object
			// to prevent unexpected data races
			copy := *resource
			copy.SetResourceID(0)
			resource = &copy
		}
		return utils.FastMarshal(resource)
	default:
		return nil, trace.BadParameter("unrecognized resource version %T", c)
	}
}
