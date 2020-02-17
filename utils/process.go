package utils

import (
	"context"
	"sync"
)

type Job func(context.Context)

type Process struct {
	// doneCh is closed when all the jobs are completed.
	doneCh chan struct{}
	// spawn runs a goroutine in the app's context as a job with waiting for
	// its completion on shutdown.
	spawn func(Job)
	// terminate signals the app to terminate gracefully.
	terminate func()
	// cancel signals the app to terminate immediately
	cancel context.CancelFunc
}

func NewProcess(ctx context.Context) *Process {
	ctx, cancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})
	var jobs sync.WaitGroup
	var once sync.Once

	jobs.Add(1) // Start the main "job". We have to do it for Wait() not being returned beforehand.
	go func() {
		jobs.Wait()
		close(doneCh)
	}()

	return &Process{
		terminate: func() {
			once.Do(func() {
				jobs.Done() // Stop the main "job".
			})
		},
		spawn: func(j Job) {
			jobs.Add(1)
			go func() {
				j(ctx)
				jobs.Done()
			}()
		},
		doneCh: doneCh,
		cancel: cancel,
	}
}

func (p *Process) Wait() {
	if p == nil {
		return
	}
	<-p.doneCh
}

func (p *Process) Spawn(f Job) {
	if p == nil {
		panic("spawning a job on a nil process")
	}
	select {
	case <-p.doneCh:
		panic("spawning a job on a finished process")
	default:
		p.spawn(f)
	}
}

func (p *Process) Shutdown(ctx context.Context) error {
	if p == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	p.terminate()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.doneCh:
		return nil
	}
}

func (p *Process) Close() {
	if p == nil {
		return
	}
	p.terminate()
	p.cancel()
	<-p.doneCh
}
