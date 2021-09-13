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
	"path"
	"regexp"
	"runtime"
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

const IntegrationAdminRole = "integration-admin"
const DefaultLicensePath = "/var/lib/teleport/license.pem"

var regexpVersion = regexp.MustCompile(`^Teleport( Enterprise)? ([^ ]+)`)

type Integration struct {
	mu    sync.Mutex
	paths struct {
		BinPaths
		license string
	}
	workDir string
	cleanup []func() error
	version Version
}

type BinPaths struct {
	Teleport string
	Tctl     string
}

type Service interface {
	PublicAddr() string
	ConfigPath() string
	Run(context.Context) error
	WaitReady(ctx context.Context) (bool, error)
	Err() error
	Shutdown(context.Context) error
}

type Version struct {
	*version.Version
	IsEnterprise bool
}

const serviceShutdownTimeout = 10 * time.Second

// New initializes a Teleport installation.
func New(ctx context.Context, paths BinPaths, licenseStr string) (*Integration, error) {
	var err error

	var integration Integration
	integration.paths.BinPaths = paths
	initialized := false
	defer func() {
		if !initialized {
			integration.Close()
		}
	}()

	integration.workDir, err = ioutil.TempDir("", "teleport-plugins-integration-*")
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize work directory")
	}
	integration.registerCleanup(func() error { return os.RemoveAll(integration.workDir) })

	teleportVersion, err := getBinaryVersion(ctx, integration.paths.Teleport)
	if err != nil {
		return nil, trace.Wrap(err, "failed to get teleport version")
	}
	tctlVersion, err := getBinaryVersion(ctx, integration.paths.Tctl)
	if err != nil {
		return nil, trace.Wrap(err, "failed to get tctl version")
	}
	if !teleportVersion.Equal(tctlVersion.Version) {
		return nil, trace.Wrap(err, "teleport version %s does not match tctl version %s", teleportVersion.Version, tctlVersion.Version)
	}
	if teleportVersion.IsEnterprise {
		if licenseStr == "" {
			return nil, trace.Errorf("%s appears to be an Enterprise binary but license path is not specified", integration.paths.Teleport)
		}
		if strings.HasPrefix(licenseStr, "-----BEGIN CERTIFICATE-----") || strings.Contains(licenseStr, "\n") {
			// If it looks like a license file content lets write it to temporary file.
			licenseFile, err := integration.tempFile("license-*.pem")
			if err != nil {
				return nil, trace.Wrap(err, "failed to write license file")
			}
			if _, err := licenseFile.WriteString(licenseStr); err != nil {
				return nil, trace.Wrap(err, "failed to write license file")
			}
			if err := licenseFile.Close(); err != nil {
				return nil, trace.Wrap(err, "failed to write license file")
			}
			integration.paths.license = licenseFile.Name()
		} else if licenseStr != "" {
			integration.paths.license = licenseStr
			if !fileExists(integration.paths.license) {
				return nil, trace.NotFound("license file not found")
			}
		}
	}

	integration.version = teleportVersion

	initialized = true
	return &integration, nil
}

// NewFromEnv initializes Teleport installation reading binary paths from environment variables such as
// TELEPORT_BINARY, TELEPORT_BINARY_TCTL or just PATH.
func NewFromEnv(ctx context.Context) (*Integration, error) {
	var err error

	licenseStr, ok := os.LookupEnv("TELEPORT_ENTERPRISE_LICENSE")
	if !ok && fileExists(DefaultLicensePath) {
		licenseStr = DefaultLicensePath
	}

	var paths BinPaths

	if os.Getenv("CI") != "" {
		if licenseStr == "" {
			return nil, trace.AccessDenied("tests on CI should run with enterprise license")
		}
	}

	if version := os.Getenv("TELEPORT_GET_VERSION"); version == "" {
		paths = BinPaths{
			Teleport: os.Getenv("TELEPORT_BINARY"),
			Tctl:     os.Getenv("TELEPORT_BINARY_TCTL"),
		}

		// Look up binaries either in file system or in PATH.

		if paths.Teleport == "" {
			paths.Teleport = "teleport"
		}
		if paths.Teleport, err = exec.LookPath(paths.Teleport); err != nil {
			return nil, trace.Wrap(err)
		}

		if paths.Tctl == "" {
			paths.Tctl = "tctl"
		}
		if paths.Tctl, err = exec.LookPath(paths.Tctl); err != nil {
			return nil, trace.Wrap(err)
		}
	} else {
		_, goFile, _, ok := runtime.Caller(0)
		if !ok {
			return nil, trace.Errorf("failed to get caller information")
		}
		outDir := path.Join(path.Dir(goFile), "..", "..", "..", ".teleport") // subdir in repo root
		if licenseStr != "" {
			paths, err = GetEnterprise(ctx, version, outDir)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		} else {
			paths, err = GetOSS(ctx, version, outDir)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		}
	}

	return New(ctx, paths, licenseStr)
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
func (integration *Integration) NewAuthServer() (*AuthServer, error) {
	dataDir, err := integration.tempDir("data-*")
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize data directory")
	}

	configFile, err := integration.tempFile("teleport-auth-*.yaml")
	if err != nil {
		return nil, trace.Wrap(err, "failed to write config file")
	}
	yaml := strings.ReplaceAll(teleportAuthYAML, "{{TELEPORT_DATA_DIR}}", dataDir)
	yaml = strings.ReplaceAll(yaml, "{{TELEPORT_LICENSE_FILE}}", integration.paths.license)
	if _, err := configFile.WriteString(yaml); err != nil {
		return nil, trace.Wrap(err, "failed to write config file")
	}
	if err := configFile.Close(); err != nil {
		return nil, trace.Wrap(err, "failed to write config file")
	}

	auth := newAuthServer(integration.paths.Teleport, configFile.Name())
	integration.registerService(auth)
	return auth, nil
}

func (integration *Integration) Bootstrap(ctx context.Context, service Service, resources []types.Resource) error {
	return integration.tctl(service).Create(ctx, resources)
}

// Client builds an API client for a given user.
func (integration *Integration) NewClient(ctx context.Context, service Service, userName string) (*Client, error) {
	outPath, err := integration.Sign(ctx, service, userName)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	apiClient, err := client.New(ctx, client.Config{
		Addrs:       []string{service.PublicAddr()},
		Credentials: []client.Credentials{client.LoadIdentityFile(outPath)},
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	client := &Client{Client: apiClient}
	integration.registerCleanup(client.Close)
	return client, nil
}

func (integration *Integration) MakeAdmin(ctx context.Context, service Service, userName string) (*Client, error) {
	var bootstrap Bootstrap
	if _, err := bootstrap.AddRole(IntegrationAdminRole, types.RoleSpecV4{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.Rule{
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
		},
	}); err != nil {
		return nil, trace.Wrap(err, fmt.Sprintf("failed to initialize %s role", IntegrationAdminRole))
	}
	if _, err := bootstrap.AddUserWithRoles(userName, IntegrationAdminRole); err != nil {
		return nil, trace.Wrap(err, fmt.Sprintf("failed to initialize %s user", userName))
	}
	if err := integration.Bootstrap(ctx, service, bootstrap.Resources()); err != nil {
		return nil, trace.Wrap(err, fmt.Sprintf("failed to bootstrap admin user %s", userName))
	}
	return integration.NewClient(ctx, service, userName)
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
	cmd := exec.CommandContext(ctx, binaryPath, "version")
	logger.Get(ctx).Debugf("Running %s", cmd)
	out, err := cmd.Output()
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
		Path:       integration.paths.Tctl,
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
