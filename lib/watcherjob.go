package lib

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	"google.golang.org/grpc"
)

type WatcherJobFunc func(context.Context, types.Event) error

type watcherJob struct {
	ServiceJob
	client    *client.Client
	watch     types.Watch
	eventFunc WatcherJobFunc
}

func NewWatcherJob(client *client.Client, watch types.Watch, fn WatcherJobFunc) ServiceJob {
	client = client.WithCallOptions(grpc.WaitForReady(true)) // Enable backoff on reconnecting.
	watcherJob := &watcherJob{
		client:    client,
		watch:     watch,
		eventFunc: fn,
	}
	watcherJob.ServiceJob = NewServiceJob(func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		log := logger.Get(ctx)

		MustGetProcess(ctx).OnTerminate(func(_ context.Context) error {
			cancel()
			return nil
		})

		for {
			err := watcherJob.eventLoop(ctx)
			switch {
			case trace.IsConnectionProblem(err):
				log.WithError(err).Error("Failed to connect to Teleport Auth server. Reconnecting...")
			case trace.IsEOF(err):
				log.WithError(err).Error("Watcher stream closed. Reconnecting...")
			case IsCanceled(err):
				// Context cancellation is not an error
				return nil
			default:
				return trace.Wrap(err)
			}
		}
	})
	return watcherJob
}

func (job *watcherJob) eventLoop(ctx context.Context) error {
	watcher, err := job.client.NewWatcher(ctx, job.watch)
	if err != nil {
		return trace.Wrap(err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			logger.Get(ctx).WithError(err).Error("Failed to close a watcher")
		}
	}()

	if err := job.waitInit(ctx, watcher, 5*time.Second); err != nil {
		return trace.Wrap(err)
	}

	logger.Get(ctx).Debug("Watcher connected")
	job.SetReady(true)

	process := MustGetProcess(ctx)

	for {
		select {
		case event := <-watcher.Events():
			process.Spawn(func(ctx context.Context) error {
				return job.eventFunc(ctx, event)
			})
		case <-watcher.Done():
			return trace.Wrap(watcher.Error())
		}
	}
}

func (job *watcherJob) waitInit(ctx context.Context, watcher types.Watcher, timeout time.Duration) error {
	select {
	case event := <-watcher.Events():
		if event.Type != types.OpInit {
			return trace.ConnectionProblem(nil, "unexpected event type %q", event.Type)
		}
		return nil
	case <-time.After(timeout):
		return trace.ConnectionProblem(nil, "watcher initialization timed out")
	case <-watcher.Done():
		return trace.Wrap(watcher.Error())
	case <-ctx.Done():
		return trace.Wrap(ctx.Err())
	}
}
