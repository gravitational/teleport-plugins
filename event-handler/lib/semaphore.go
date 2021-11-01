package lib

import "context"

// Semaphore represents a semaphore pattern implementation
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore returns new Semaphore with a specific limit
func NewSemaphore(limit int) Semaphore {
	return Semaphore{ch: make(chan struct{}, limit)}
}

// Acquire increases semaphore internal counter
func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release decreases semaphore internal counter
func (s *Semaphore) Release(ctx context.Context) error {
	select {
	case <-s.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
