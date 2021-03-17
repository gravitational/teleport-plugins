/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	spdystream "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	remotecommandconsts "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

// remoteCommandRequest is a request to execute a remote command
type remoteCommandRequest struct {
	podNamespace       string
	podName            string
	containerName      string
	cmd                []string
	stdin              bool
	stdout             bool
	stderr             bool
	tty                bool
	httpRequest        *http.Request
	httpResponseWriter http.ResponseWriter
	onResize           resizeCallback
	context            context.Context
	pingPeriod         time.Duration
}

func (req remoteCommandRequest) eventPodMeta(ctx context.Context, creds *kubeCreds) events.KubernetesPodMetadata {
	meta := events.KubernetesPodMetadata{
		KubernetesPodName:       req.podName,
		KubernetesPodNamespace:  req.podNamespace,
		KubernetesContainerName: req.containerName,
	}
	if creds == nil || creds.kubeClient == nil {
		return meta
	}

	// Optionally, try to get more info about the pod.
	//
	// This can fail if a user has set tight RBAC rules for teleport. Failure
	// here shouldn't prevent a session from starting.
	pod, err := creds.kubeClient.CoreV1().Pods(req.podNamespace).Get(ctx, req.podName, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Debugf("Failed fetching pod from kubernetes API; skipping additional metadata on the audit event")
		return meta
	}
	meta.KubernetesNodeName = pod.Spec.NodeName

	// If a container name was provided, find its image name.
	if req.containerName != "" {
		for _, c := range pod.Spec.Containers {
			if c.Name == req.containerName {
				meta.KubernetesContainerImage = c.Image
				break
			}
		}
	}
	// If no container name was provided, use the default one.
	if req.containerName == "" && len(pod.Spec.Containers) > 0 {
		meta.KubernetesContainerName = pod.Spec.Containers[0].Name
		meta.KubernetesContainerImage = pod.Spec.Containers[0].Image
	}
	return meta
}

func createRemoteCommandProxy(req remoteCommandRequest) (*remoteCommandProxy, error) {
	protocol, err := httpstream.Handshake(req.httpRequest, req.httpResponseWriter, []string{StreamProtocolV4Name})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	streamCh := make(chan streamAndReply)

	upgrader := spdystream.NewResponseUpgraderWithPings(req.pingPeriod)
	conn := upgrader.UpgradeResponse(req.httpResponseWriter, req.httpRequest, func(stream httpstream.Stream, replySent <-chan struct{}) error {
		select {
		case streamCh <- streamAndReply{Stream: stream, replySent: replySent}:
			return nil
		case <-req.context.Done():
			return trace.BadParameter("request has been cancelled")
		}
	})
	// from this point on, we can no longer call methods on response
	if conn == nil {
		// The upgrader is responsible for notifying the client of any errors that
		// occurred during upgrading. All we can do is return here at this point
		// if we weren't successful in upgrading.
		return nil, trace.ConnectionProblem(trace.BadParameter("missing connection"), "missing connection")
	}

	conn.SetIdleTimeout(IdleTimeout)

	var handler protocolHandler
	switch protocol {
	case "":
		log.Warningf("Client did not request protocol negotiation.")
		fallthrough
	case StreamProtocolV4Name:
		log.Infof("Negotiated protocol %v.", protocol)
		handler = &v4ProtocolHandler{}
	default:
		return nil, trace.BadParameter("protocol %v is not supported. upgrade the client", protocol)
	}

	// count the streams client asked for, starting with 1
	expectedStreams := 1
	if req.stdin {
		expectedStreams++
	}
	if req.stdout {
		expectedStreams++
	}
	if req.stderr {
		expectedStreams++
	}
	if req.tty && handler.supportsTerminalResizing() {
		expectedStreams++
	}

	expired := time.NewTimer(DefaultStreamCreationTimeout)
	defer expired.Stop()

	proxy, err := handler.waitForStreams(req.context, streamCh, expectedStreams, expired.C)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	proxy.conn = conn
	proxy.tty = req.tty

	if proxy.resizeStream != nil {
		proxy.resizeQueue = newTermQueue(req.context, req.onResize)
		go proxy.resizeQueue.handleResizeEvents(proxy.resizeStream)
	}
	return proxy, nil
}

// remoteCommandProxy contains the connection and streams used when
// forwarding an attach or execute session into a container.
type remoteCommandProxy struct {
	conn         io.Closer
	stdinStream  io.ReadCloser
	stdoutStream io.WriteCloser
	stderrStream io.WriteCloser
	writeStatus  func(status *apierrors.StatusError) error
	resizeStream io.ReadCloser
	tty          bool
	resizeQueue  *termQueue
}

func (s *remoteCommandProxy) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *remoteCommandProxy) options() remotecommand.StreamOptions {
	opts := remotecommand.StreamOptions{
		Stdout: s.stdoutStream,
		Stdin:  s.stdinStream,
		Stderr: s.stderrStream,
		Tty:    s.tty,
	}
	// done to prevent this problem: https://golang.org/doc/faq#nil_error
	if s.resizeQueue != nil {
		opts.TerminalSizeQueue = s.resizeQueue
	}
	return opts
}

func (s *remoteCommandProxy) sendStatus(err error) error {
	if err == nil {
		return s.writeStatus(&apierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusSuccess,
		}})
	}
	if exitErr, ok := err.(utilexec.ExitError); ok && exitErr.Exited() {
		rc := exitErr.ExitStatus()
		return s.writeStatus(&apierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Reason: remotecommandconsts.NonZeroExitCodeReason,
			Details: &metav1.StatusDetails{
				Causes: []metav1.StatusCause{
					{
						Type:    remotecommandconsts.ExitCodeCauseType,
						Message: fmt.Sprintf("%d", rc),
					},
				},
			},
			Message: fmt.Sprintf("command terminated with non-zero exit code: %v", exitErr),
		}})
	}
	err = trace.BadParameter("error executing command in container: %v", err)
	return s.writeStatus(apierrors.NewInternalError(err))
}

// streamAndReply holds both a Stream and a channel that is closed when the stream's reply frame is
// enqueued. Consumers can wait for replySent to be closed prior to proceeding, to ensure that the
// replyFrame is enqueued before the connection's goaway frame is sent (e.g. if a stream was
// received and right after, the connection gets closed).
type streamAndReply struct {
	httpstream.Stream
	replySent <-chan struct{}
}

func newTermQueue(parentContext context.Context, onResize resizeCallback) *termQueue {
	ctx, cancel := context.WithCancel(parentContext)
	return &termQueue{
		ch:       make(chan remotecommand.TerminalSize),
		cancel:   cancel,
		done:     ctx,
		onResize: onResize,
	}
}

type resizeCallback func(remotecommand.TerminalSize)

type termQueue struct {
	ch       chan remotecommand.TerminalSize
	cancel   context.CancelFunc
	done     context.Context
	onResize resizeCallback
}

func (t *termQueue) Next() *remotecommand.TerminalSize {
	select {
	case size := <-t.ch:
		t.onResize(size)
		return &size
	case <-t.done.Done():
		return nil
	}
}

func (t *termQueue) Done() {
	t.cancel()
}

func (t *termQueue) handleResizeEvents(stream io.Reader) {
	decoder := json.NewDecoder(stream)
	for {
		size := remotecommand.TerminalSize{}
		if err := decoder.Decode(&size); err != nil {
			if err != io.EOF {
				log.Warningf("Failed to decode resize event: %v", err)
			}
			t.cancel()
			return
		}
		select {
		case t.ch <- size:
		case <-t.done.Done():
			return
		}
	}
}

type protocolHandler interface {
	// waitForStreams waits for the expected streams or a timeout, returning a
	// remoteCommandContext if all the streams were received, or an error if not.
	waitForStreams(ctx context.Context, streams <-chan streamAndReply, expectedStreams int, expired <-chan time.Time) (*remoteCommandProxy, error)
	// supportsTerminalResizing returns true if the protocol handler supports terminal resizing
	supportsTerminalResizing() bool
}

// v4ProtocolHandler implements the V4 protocol version for streaming command execution. It only differs
// in from v3 in the error stream format using an json-marshaled metav1.Status which carries
// the process' exit code.
type v4ProtocolHandler struct{}

func (*v4ProtocolHandler) waitForStreams(connContext context.Context, streams <-chan streamAndReply, expectedStreams int, expired <-chan time.Time) (*remoteCommandProxy, error) {
	remoteProxy := &remoteCommandProxy{}
	receivedStreams := 0
	replyChan := make(chan struct{})

	stopCtx, cancel := context.WithCancel(connContext)
	defer cancel()
WaitForStreams:
	for {
		select {
		case stream := <-streams:
			streamType := stream.Headers().Get(StreamType)
			switch streamType {
			case StreamTypeError:
				remoteProxy.writeStatus = v4WriteStatusFunc(stream)
				go waitStreamReply(stopCtx, stream.replySent, replyChan)
			case StreamTypeStdin:
				remoteProxy.stdinStream = stream
				go waitStreamReply(stopCtx, stream.replySent, replyChan)
			case StreamTypeStdout:
				remoteProxy.stdoutStream = stream
				go waitStreamReply(stopCtx, stream.replySent, replyChan)
			case StreamTypeStderr:
				remoteProxy.stderrStream = stream
				go waitStreamReply(stopCtx, stream.replySent, replyChan)
			case StreamTypeResize:
				remoteProxy.resizeStream = stream
				go waitStreamReply(stopCtx, stream.replySent, replyChan)
			default:
				log.Warningf("Ignoring unexpected stream type: %q", streamType)
			}
		case <-replyChan:
			receivedStreams++
			if receivedStreams == expectedStreams {
				break WaitForStreams
			}
		case <-expired:
			return nil, trace.BadParameter("timed out waiting for client to create streams")
		case <-connContext.Done():
			return nil, trace.BadParameter("onnectoin has dropped, exiting")
		}
	}

	return remoteProxy, nil
}

// supportsTerminalResizing returns true because v4ProtocolHandler supports it
func (*v4ProtocolHandler) supportsTerminalResizing() bool { return true }

// waitStreamReply waits until either replySent or stop is closed. If replySent is closed, it sends
// an empty struct to the notify channel.
func waitStreamReply(ctx context.Context, replySent <-chan struct{}, notify chan<- struct{}) {
	select {
	case <-replySent:
		notify <- struct{}{}
	case <-ctx.Done():
	}
}

// v4WriteStatusFunc returns a WriteStatusFunc that marshals a given api Status
// as json in the error channel.
func v4WriteStatusFunc(stream io.Writer) func(status *apierrors.StatusError) error {
	return func(status *apierrors.StatusError) error {
		bs, err := json.Marshal(status.Status())
		if err != nil {
			return err
		}
		_, err = stream.Write(bs)
		return err
	}
}
