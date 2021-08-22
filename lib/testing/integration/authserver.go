/*
Copyright 2021 Gravitational, Inc.

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

package integration

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

var regexpAuthStarting = regexp.MustCompile(`Auth service [^ ]+ is starting on [^ ]+:(\d+)`)

type AuthServer struct {
	mu           sync.Mutex
	teleportPath string
	configPath   string
	authAddr     string
	readyCh      chan struct{}
	doneCh       chan struct{}
	terminate    context.CancelFunc
	setErr       func(error)
	error        error
	stderr       bytes.Buffer
}

func newAuthServer(teleportPath, configPath string) Service {
	var setErrOnce sync.Once
	var auth AuthServer
	auth = AuthServer{
		teleportPath: teleportPath,
		configPath:   configPath,
		readyCh:      make(chan struct{}),
		doneCh:       make(chan struct{}),
		terminate:    func() {}, // dummy noop that will be overridden by Run(),
		setErr: func(err error) {
			setErrOnce.Do(func() {
				auth.mu.Lock()
				defer auth.mu.Unlock()
				auth.error = err
			})
		},
	}
	return &auth
}

// Run spawns an auth server instance.
func (auth *AuthServer) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log := logger.Get(ctx)

	cmd := exec.CommandContext(ctx, auth.teleportPath, "start", "--debug", "--config", auth.configPath)
	log.Debugf("Running Auth Server: %s", cmd)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		err = trace.Wrap(err, "failed to get stdout")
		auth.setErr(err)
		return err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		err = trace.Wrap(err, "failed to get stderr")
		auth.setErr(err)
		return err
	}

	if err := cmd.Start(); err != nil {
		err = trace.Wrap(err, "failed to start teleport")
		auth.setErr(err)
		return err
	}

	ctx, log = logger.WithField(ctx, "pid", cmd.Process.Pid)
	log.Debug("Auth Server process has been started")

	auth.mu.Lock()
	var terminateOnce sync.Once
	auth.terminate = func() {
		terminateOnce.Do(func() {
			log.Debug("Terminating Auth Server process")
			// Signal the process to gracefully terminate by sending SIGINT.
			cmd.Process.Signal(os.Interrupt)
			// If we're not done in 5 minutes, just kill the process by cancelling its context.
			go func() {
				select {
				case <-auth.doneCh:
				case <-time.After(serviceShutdownTimeout):
					log.Debug("Killing Auth Server process")
				}
				// cancel() results in sending SIGKILL to a process if it's still alive.
				cancel()
			}()
		})
	}
	auth.mu.Unlock()

	var ioWork sync.WaitGroup
	ioWork.Add(2)

	// Parse stdout of a Teleport process.
	go func() {
		defer ioWork.Done()

		var initialized bool
		stdout := bufio.NewReader(stdoutPipe)
		for {
			line, err := stdout.ReadString('\n')
			if line != "" {
				auth.parseLine(ctx, line)
				if !initialized {
					if addr := auth.PublicAddr(); addr != "" {
						log.Debugf("Found listen addr of Auth Server process: %v", addr)
						initialized = true
						close(auth.readyCh)
					}
				}
			}
			if err == io.EOF {
				return
			}
			if err := trace.Wrap(err); err != nil {
				log.WithError(err).Error("failed to read process stdout")
				return
			}
		}
	}()

	// Save stderr to a buffer.
	go func() {
		defer ioWork.Done()

		stderr := bufio.NewReader(stderrPipe)
		data := make([]byte, stderr.Size())
		for {
			n, err := stderr.Read(data)
			auth.saveStderr(data[:n])
			if err == io.EOF {
				return
			}
			if err := trace.Wrap(err); err != nil {
				log.WithError(err).Error("failed to read process stderr")
				return
			}
		}
	}()

	// Wait for process completeness after processing both outputs.
	go func() {
		ioWork.Wait()
		err := trace.Wrap(cmd.Wait())
		auth.setErr(err)
		close(auth.doneCh)
	}()

	<-auth.doneCh
	return trace.Wrap(auth.Err())
}

// configPath returns auth server config file path.
func (auth *AuthServer) ConfigPath() string {
	return auth.configPath
}

// PublicAddr returns auth server external address.
func (auth *AuthServer) PublicAddr() string {
	return auth.authAddr
}

// Err returns auth server error. It's nil If process is not done yet.
func (auth *AuthServer) Err() error {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	return auth.error
}

// Shutdown terminates the auth server process and waits for its completion.
func (auth *AuthServer) Shutdown(ctx context.Context) error {
	auth.doTerminate()
	select {
	case <-auth.doneCh:
		return nil
	case <-ctx.Done():
		return trace.Wrap(ctx.Err())
	}
}

// ErrorOutput returns auth server process stderr.
func (auth *AuthServer) ErrorOutput() string {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	return auth.stderr.String()
}

// WaitReady waits for auth server initialization.
func (auth *AuthServer) WaitReady(ctx context.Context) (bool, error) {
	select {
	case <-auth.readyCh:
		return true, nil
	case <-ctx.Done():
		return false, trace.Wrap(ctx.Err(), "auth server is not ready")
	}
}

func (auth *AuthServer) doTerminate() {
	auth.mu.Lock()
	terminate := auth.terminate
	auth.mu.Unlock()
	terminate()
}

func (auth *AuthServer) parseLine(ctx context.Context, line string) {
	submatch := regexpAuthStarting.FindStringSubmatch(line)
	if submatch != nil {
		auth.mu.Lock()
		defer auth.mu.Unlock()
		auth.authAddr = "127.0.0.1:" + submatch[1]
		return
	}
}

func (auth *AuthServer) saveStderr(chunk []byte) {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	auth.stderr.Write(chunk)
}
