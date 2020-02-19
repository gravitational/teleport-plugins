package utils

import (
	"context"
	"sync"
)

type Job func(context.Context)

type Process struct {
	sync.Mutex
	// doneCh is closed when all the jobs are completed.
	doneCh chan struct{}
	// spawn runs a goroutine in the app's context as a job with waiting for
	// its completion on shutdown.
	spawn func(Job)
	// terminate signals the app to terminate gracefully.
	terminate func()
	// cancel signals the app to terminate immediately
	cancel context.CancelFunc
	// onTerminate is a list of callbacks called on terminate.
	onTerminate []Job
}

var closedChan = make(chan struct{})

func init() {
	close(closedChan)
}

func NewProcess(ctx context.Context) *Process {
	ctx, cancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})
	process := &Process{
		doneCh:      doneCh,
		cancel:      cancel,
		onTerminate: make([]Job, 0),
	}

	var jobs sync.WaitGroup

	jobs.Add(1) // Start the main "job". We have to do it for Wait() not being returned beforehand.
	go func() {
		jobs.Wait()
		close(doneCh)
	}()
	process.spawn = func(f Job) {
		jobs.Add(1)
		go func() {
			f(ctx)
			jobs.Done()
		}()
	}

	var once sync.Once
	process.terminate = func() {
		once.Do(func() {
			process.Lock()
			for _, j := range process.onTerminate {
				process.spawn(j)
			}
			process.Unlock()
			jobs.Done() // Stop the main "job".
		})
	}

	return process
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

func (p *Process) OnTerminate(f Job) {
	if p == nil {
		panic("calling OnTerminate a nil process")
	}
	p.Lock()
	p.onTerminate = append(p.onTerminate, f)
	p.Unlock()
}

// Done channel is used to wait for jobs completion.
func (p *Process) Done() <-chan struct{} {
	if p == nil {
		return closedChan
	}
	return p.doneCh
}

// Terminate signals a process to terminate. You should avoid spawning new jobs after termination.
func (p *Process) Terminate() {
	if p == nil {
		return
	}
	p.terminate()
}

// Shutdown signals a process to terminate and waits for completion of all jobs.
func (p *Process) Shutdown(ctx context.Context) error {
	p.Terminate()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.Done():
		return nil
	}
}

// Close shuts down all process jobs immediately.
func (p *Process) Close() {
	if p == nil {
		return
	}
	p.terminate()
	p.cancel()
	<-p.doneCh
}
