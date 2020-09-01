package access

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	"google.golang.org/grpc"

	log "github.com/sirupsen/logrus"
)

type WatcherJobFunc func(context.Context, Event) error

type watcherJob struct {
	utils.ServiceJob
	client    Client
	filter    Filter
	eventFunc WatcherJobFunc
}

func NewWatcherJob(client Client, filter Filter, fn WatcherJobFunc) utils.ServiceJob {
	client = client.WithCallOptions(grpc.WaitForReady(true)) // Enable backoff on reconnecting.
	watcherJob := &watcherJob{
		client:    client,
		filter:    filter,
		eventFunc: fn,
	}
	watcherJob.ServiceJob = utils.NewServiceJob(func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)

		utils.MustGetProcess(ctx).OnTerminate(func(_ context.Context) error {
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
			case utils.IsCanceled(err):
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
	watcher := job.client.WatchRequests(ctx, job.filter)
	defer watcher.Close()

	if err := watcher.WaitInit(ctx, 5*time.Second); err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Watcher connected")
	job.SetReady(true)

	process := utils.MustGetProcess(ctx)

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
