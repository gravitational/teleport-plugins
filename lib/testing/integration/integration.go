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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-version"

	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport-plugins/lib/tctl"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

var regexpVersion = regexp.MustCompile(`^Teleport( Enterprise)? ([^ ]+)`)

type Integration struct {
	mu           sync.Mutex
	teleportPath string
	tctlPath     string
	licensePath  string
	workDir      string
	cleanup      []func() error
	version      Version
}

type Service interface {
	PublicAddr() string
	ConfigPath() string
	Run(context.Context) error
	WaitReady(ctx context.Context) (bool, error)
	Err() error
	Shutdown(context.Context) error
	ErrorOutput() string
}

type Version struct {
	*version.Version
	IsEnterprise bool
}

const serviceShutdownTimeout = 5 * time.Second

// New initializes a Teleport installation.
func New(ctx context.Context, teleportPath, tctlPath, licensePath string) (*Integration, error) {
	integration := Integration{
		teleportPath: teleportPath,
		tctlPath:     tctlPath,
		licensePath:  licensePath,
	}

	teleportVersion, err := getBinaryVersion(ctx, teleportPath)
	if err != nil {
		return nil, trace.Wrap(err, "failed to get teleport version")
	}
	tctlVersion, err := getBinaryVersion(ctx, tctlPath)
	if err != nil {
		return nil, trace.Wrap(err, "failed to get tctl version")
	}
	if !teleportVersion.Equal(tctlVersion.Version) {
		return nil, trace.Wrap(err, "teleport version %s does not match tctl version %s", teleportVersion.Version, tctlVersion.Version)
	}
	if teleportVersion.IsEnterprise {
		if licensePath == "" {
			return nil, trace.Errorf("%q appears to be an Enterprise binary but license path is not specified", teleportPath)
		}
		if _, err := os.Stat(licensePath); err != nil {
			return nil, trace.Wrap(err, "failed to read license file %q", licensePath)
		}
	}

	integration.version = teleportVersion

	integration.workDir, err = ioutil.TempDir("", "teleport-plugins-integration-*")
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize work directory")
	}
	integration.registerCleanup(func() error { return os.RemoveAll(integration.workDir) })

	return &integration, nil
}

// NewFromEnv initializes Teleport installation reading binary paths from environment variables such as
// TELEPORT_BINARY, TELEPORT_BINARY_TCTL or just PATH.
func NewFromEnv(ctx context.Context) (*Integration, error) {
	var err error

	teleportPath := os.Getenv("TELEPORT_BINARY")
	if teleportPath == "" {
		teleportPath = "teleport"
	}
	if teleportPath, err = exec.LookPath(teleportPath); err != nil {
		return nil, trace.Wrap(err)
	}

	tctlPath := os.Getenv("TELEPORT_BINARY_TCTL")
	if tctlPath == "" {
		tctlPath = "tctl"
	}
	if tctlPath, err = exec.LookPath(tctlPath); err != nil {
		return nil, trace.Wrap(err)
	}

	licensePath := os.Getenv("TELEPORT_LICENSE")
	if licensePath == "" {
		licensePath = "/var/lib/teleport/license.pem"
	}

	return New(ctx, teleportPath, tctlPath, licensePath)
}

// Close stops all the spawned processes and does a cleanup.
func (integration *Integration) Close() {
	integration.mu.Lock()
	cleanup := integration.cleanup
	integration.cleanup = nil
	integration.mu.Unlock()

	for idx := range cleanup {
		if err := cleanup[len(cleanup)-idx-1](); err != nil {
			logger.Standard().WithError(trace.Wrap(err)).Error("Cleanup operation failed")
		}
	}
}

// Version returns an auth server version.
func (integration *Integration) Version() Version {
	return integration.version
}

// NewAuthServer creates a new auth server instance.
func (integration *Integration) NewAuthServer() (Service, error) {
	dataDir, err := integration.tempDir("data-*")
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize data directory")
	}

	configFile, err := integration.tempFile("teleport-auth-*.yaml")
	if err != nil {
		return nil, trace.Wrap(err, "failed to write config file")
	}
	yaml := strings.ReplaceAll(teleportAuthYAML, "{{TELEPORT_DATA_DIR}}", dataDir)
	yaml = strings.ReplaceAll(yaml, "{{TELEPORT_LICENSE_FILE}}", integration.licensePath)
	if _, err := configFile.WriteString(yaml); err != nil {
		return nil, trace.Wrap(err, "failed to write config file")
	}
	if err := configFile.Close(); err != nil {
		return nil, trace.Wrap(err, "failed to write config file")
	}

	auth := newAuthServer(integration.teleportPath, configFile.Name())
	integration.registerService(auth)
	return auth, nil
}

func (integration *Integration) Bootstrap(ctx context.Context, service Service, resources []types.Resource) error {
	return integration.tctl(service).Create(ctx, resources)
}

func (integration *Integration) NewAPI(ctx context.Context, service Service) (*API, error) {
	var bootstrap Bootstrap
	if _, err := bootstrap.AddRole(APIUsername, types.RoleSpecV4{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.Rule{
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
		},
	}); err != nil {
		return nil, trace.Wrap(err, fmt.Sprintf("failed to initialize %q role", APIUsername))
	}
	if _, err := bootstrap.AddUserWithRoles(APIUsername, APIUsername); err != nil {
		return nil, trace.Wrap(err, fmt.Sprintf("failed to initialize %q user", APIUsername))
	}

	if err := integration.Bootstrap(ctx, service, bootstrap.Resources()); err != nil {
		return nil, trace.Wrap(err, fmt.Sprintf("failed to bootstrap %q resources", APIUsername))
	}

	return newAPI(ctx, service, integration)
}

// Client builds an API client for a given user.
func (integration *Integration) Client(ctx context.Context, service Service, userName string) (*client.Client, error) {
	outPath, err := integration.Sign(ctx, service, userName)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client.New(ctx, client.Config{
		Addrs:       []string{service.PublicAddr()},
		Credentials: []client.Credentials{client.LoadIdentityFile(outPath)},
	})
}

// Sign generates a credentials file for the user.
func (integration *Integration) Sign(ctx context.Context, service Service, userName string) (string, error) {
	outFile, err := integration.tempFile(fmt.Sprintf("credentials-%s-*", userName))
	if err != nil {
		return "", trace.Wrap(err)
	}
	if err := outFile.Close(); err != nil {
		return "", trace.Wrap(err)
	}
	outPath := outFile.Name()
	if err := integration.tctl(service).Sign(ctx, userName, outPath); err != nil {
		return "", trace.Wrap(err)
	}
	return outPath, nil
}

func getBinaryVersion(ctx context.Context, binaryPath string) (Version, error) {
	out, err := exec.CommandContext(ctx, binaryPath, "version").Output()
	if err != nil {
		return Version{}, trace.Wrap(err)
	}
	submatch := regexpVersion.FindStringSubmatch(string(out))
	if submatch == nil {
		return Version{}, trace.Wrap(err)
	}

	version, err := version.NewVersion(submatch[2])
	if err != nil {
		return Version{}, trace.Wrap(err)
	}

	return Version{Version: version, IsEnterprise: submatch[1] != ""}, nil
}

func (integration *Integration) tctl(service Service) tctl.Tctl {
	return tctl.Tctl{
		Path:       integration.tctlPath,
		AuthServer: service.PublicAddr(),
		ConfigPath: service.ConfigPath(),
	}
}

func (integration *Integration) registerCleanup(fn func() error) {
	integration.mu.Lock()
	defer integration.mu.Unlock()
	integration.cleanup = append(integration.cleanup, fn)
}

func (integration *Integration) registerService(service Service) {
	integration.registerCleanup(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), serviceShutdownTimeout+10*time.Millisecond)
		defer cancel()
		return service.Shutdown(ctx)
	})
}

func (integration *Integration) tempFile(pattern string) (*os.File, error) {
	file, err := ioutil.TempFile(integration.workDir, pattern)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	integration.registerCleanup(func() error { return os.Remove(file.Name()) })
	return file, trace.Wrap(err)
}

func (integration *Integration) tempDir(pattern string) (string, error) {
	dir, err := ioutil.TempDir(integration.workDir, pattern)
	if err != nil {
		return "", trace.Wrap(err)
	}
	integration.registerCleanup(func() error { return os.RemoveAll(dir) })
	return dir, nil
}
