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

package tsh

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

// Tsh is a runner of tsh command.
type Tsh struct {
	Path     string
	Proxy    string
	Identity string
	Insecure bool
}

var (
	regexpRequestsOriginated = regexp.MustCompile(`(?m)^\* Requests originated: (\d+)$`)
	regexpRequestsFailed     = regexp.MustCompile(`(?m)^\* Requests failed: (\d+)$`)
)

// BenchFlags is an options for `tsh bench` command.
type BenchFlags struct {
	Interactive bool
	Login       string
	Rate        int
	Duration    time.Duration
}

// BenchResult is a result of running `tsh bench` command.
type BenchResult struct {
	Output             string
	RequestsOriginated int
	RequestsFailed     int
}

// CheckExecutable checks if `tsh` executable exists in the system.
func (tsh Tsh) CheckExecutable() error {
	_, err := exec.LookPath(tsh.cmd())
	return trace.Wrap(err, "tsh executable is not found")
}

// Bench runs tsh bench on behalf of userHost
func (tsh Tsh) Bench(ctx context.Context, flags BenchFlags, userHost, command string) (BenchResult, error) {
	log := logger.Get(ctx)
	args := append(tsh.baseArgs(), "bench")
	if flags.Interactive {
		args = append(args, "--interactive")
	}
	if flags.Rate > 0 {
		args = append(args, strconv.Itoa(flags.Rate))
	}
	if flags.Duration > 0 {
		args = append(args, flags.Duration.String())
	}
	args = append(args, userHost, command)
	cmd := exec.CommandContext(ctx, tsh.cmd(), args...)
	log.Debugf("Running %s", cmd)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	result := BenchResult{Output: output}
	if err != nil {
		return result, trace.Wrap(err)
	}

	if submatch := regexpRequestsOriginated.FindStringSubmatch(output); len(submatch) > 0 {
		result.RequestsOriginated, err = strconv.Atoi(submatch[1])
		if err != nil {
			return result, trace.Wrap(err)
		}
	} else {
		return result, trace.Errorf("failed to parse tsh bench result")
	}

	if submatch := regexpRequestsFailed.FindStringSubmatch(output); len(submatch) > 0 {
		result.RequestsFailed, err = strconv.Atoi(submatch[1])
		if err != nil {
			return result, trace.Wrap(err)
		}
	} else {
		return result, trace.Errorf("failed to parse tsh bench result")
	}

	return result, nil
}

// SSHCommand creates exec.CommandContext for tsh ssh --tty on behalf of userHost
func (tsh Tsh) SSHCommand(ctx context.Context, userHost string) *exec.Cmd {
	log := logger.Get(ctx)
	args := append(tsh.baseArgs(), "ssh", "--tty", userHost)

	cmd := exec.CommandContext(ctx, tsh.cmd(), args...)
	log.Debugf("Running %s", cmd)

	return cmd
}

func (tsh Tsh) cmd() string {
	if tsh.Path != "" {
		return tsh.Path
	}
	return "tsh"
}

func (tsh Tsh) baseArgs() (args []string) {
	if tsh.Insecure {
		args = append(args, "--insecure")
	}
	if tsh.Identity != "" {
		args = append(args, "--identity", tsh.Identity)
	}
	if tsh.Proxy != "" {
		args = append(args, "--proxy", tsh.Proxy)
	}
	return
}
