package watcherjob

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/lib/job"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	"google.golang.org/grpc"
)

const DefaultMaxConcurrency = 128
const DefaultEventFuncTimeout = time.Second * 5

type EventFunc func(context.Context, types.Event) error

type Config struct {
	Watch            types.Watch
	MaxConcurrency   int
	EventFuncTimeout time.Duration
}

type watcherJob struct {
	config    Config
	eventFunc EventFunc
	events    types.Events
	eventCh   chan *types.Event
}

type eventKey struct {
	kind string
	name string
}

func NewJob(client *client.Client, config Config, fn EventFunc) job.Job {
	client = client.WithCallOptions(grpc.WaitForReady(true)) // Enable backoff on reconnecting.
	return NewJobWithEvents(client, config, fn)
}

func NewJobWithEvents(events types.Events, config Config, fn EventFunc) job.Job {
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = DefaultMaxConcurrency
	}
	if config.EventFuncTimeout == 0 {
		config.EventFuncTimeout = DefaultEventFuncTimeout
	}
	return watcherJob{
		events:    events,
		config:    config,
		eventFunc: fn,
		eventCh:   make(chan *types.Event, config.MaxConcurrency),
	}
}

func (watcherJob watcherJob) DoJob(ctx context.Context) error {
	process := job.MustGetProcess(ctx)

	// Run a separate event loop thread which does not depend on streamer context.
	defer close(watcherJob.eventCh)
	process.SpawnFunc(watcherJob.eventLoop)

	log := logger.Get(ctx)
	for {
		err := watcherJob.watchEvents(ctx)
		switch {
		case trace.IsConnectionProblem(err):
			log.WithError(err).Error("Failed to connect to Teleport Auth server. Reconnecting...")
		case trace.IsEOF(err):
			log.WithError(err).Error("Watcher stream closed. Reconnecting...")
		case err != nil:
			log.WithError(err).Error("Watcher event loop failed")
			return trace.Wrap(err)
		default:
			return nil
		}
	}
}

// watchEvents spawns a watcher and reads events from it.
func (watcherJob watcherJob) watchEvents(ctx context.Context) error {
	watcher, err := watcherJob.events.NewWatcher(ctx, watcherJob.config.Watch)
	if err != nil {
		return trace.Wrap(err)
	}

	if err := watcherJob.waitInit(ctx, watcher, 15*time.Second); err != nil {
		return trace.Wrap(err)
	}

	logger.Get(ctx).Debug("Watcher connected")
	job.SetReady(ctx, true)

	for {
		select {
		case event := <-watcher.Events():
			watcherJob.eventCh <- &event
		case <-job.Stopped(ctx):
			logger.Get(ctx).Debug("Gracefully terminating the watcher")
			if err := watcher.Close(); err != nil {
				return trace.Wrap(err)
			}
			return nil
		case <-watcher.Done():
			return trace.Wrap(watcher.Error())
		}
	}
}

// waitInit waits for OpInit event be received on a stream.
func (watcherJob watcherJob) waitInit(ctx context.Context, watcher types.Watcher, timeout time.Duration) error {
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

// eventLoop goes through event stream and spawns the event jobs.
//
// Queue processing algorithm is a bit tricky.
// We want to process events concurrently each in its own job.
// On the other hand, we want to avoid potential race conditions so it seems
// that in some cases it's better to process events sequentially in
// the order they came to the queue.
//
// The algorithm combines two approaches, concurrent and sequential.
// It follows the rules:
// - Events for different resources being processed concurrently.
// - Events for the same resource being processed "sequentially" i.e. in the order they came to the queue.
//
// By "sameness" we mean that Kind and Name fields of one resource object are the same as in the other resource object.
func (watcherJob watcherJob) eventLoop(ctx context.Context) error {
	var concurrency int
	process := job.MustGetProcess(ctx)
	log := logger.Get(ctx)
	queues := make(map[eventKey][]types.Event)
	eventDone := make(chan eventKey, watcherJob.config.MaxConcurrency)

	for {
		var eventCh <-chan *types.Event
		if concurrency < watcherJob.config.MaxConcurrency {
			// If haven't yet reached the limit then we allowed to read from the queue.
			// Otherwise, eventCh would be nil which is a forever-blocking channel.
			eventCh = watcherJob.eventCh
		}

		select {
		case eventPtr := <-eventCh:
			if eventPtr == nil { // channel is closed when the parent job is done so just quit normally.
				return nil
			}
			event := *eventPtr
			resource := event.Resource
			if resource == nil {
				log.Error("received an event with empty resource field")
			}
			key := eventKey{kind: resource.GetKind(), name: resource.GetName()}
			if queue, loaded := queues[key]; loaded {
				queues[key] = append(queue, event)
			} else {
				queues[key] = nil
				process.SpawnFunc(watcherJob.eventFuncHandler(event, key, eventDone))
			}
			concurrency++

		case key := <-eventDone:
			concurrency--
			queue, ok := queues[key]
			if !ok {
				continue
			}
			if len(queue) > 0 {
				event := queue[0]
				process.SpawnFunc(watcherJob.eventFuncHandler(event, key, eventDone))
				queue = queue[1:]
				queues[key] = queue
			}
			if len(queue) == 0 {
				delete(queues, key)
			}

		case <-ctx.Done(): // Stop processing immediately because the context was cancelled.
			return trace.Wrap(ctx.Err())
		}
	}
}

// eventFuncHandler returns an event handler ready to spawn.
func (watcherJob watcherJob) eventFuncHandler(event types.Event, key eventKey, doneCh chan<- eventKey) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		defer func() {
			select {
			case doneCh <- key:
			case <-ctx.Done():
			}
		}()
		eventCtx, cancel := context.WithTimeout(ctx, watcherJob.config.EventFuncTimeout)
		defer cancel()
		return watcherJob.eventFunc(eventCtx, event)
	}
}
