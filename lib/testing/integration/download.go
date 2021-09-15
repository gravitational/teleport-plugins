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
	"io"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/backoff"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport-plugins/lib/tar"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
)

type downloadVersionKey struct {
	ver        string
	os         string
	arch       string
	enterprise bool
}

type downloadVersion struct {
	sha256 lib.SHA256Sum
}

var downloadVersions = map[downloadVersionKey]downloadVersion{
	// Teleport v7.1.1 Enterprise binaries
	downloadVersionKey{"v7.1.1", "darwin", "amd64", true}: downloadVersion{
		sha256: lib.MustHexSHA256("384e621fa8ee7cdc3bd59ea5fa3eac792c2fd586579e00731a0a4690e2c7c52e"),
	},
	downloadVersionKey{"v7.1.1", "linux", "amd64", true}: downloadVersion{
		sha256: lib.MustHexSHA256("b493f5642fdb6f34b14185ed4375362515775df548b60b8d2ee18fa017d83635"),
	},
	downloadVersionKey{"v7.1.1", "linux", "arm64", true}: downloadVersion{
		sha256: lib.MustHexSHA256("ece7d7c7b462bb479598a938a10ee6a5d27eaa83b1f54e5d6e2d88365541f4ff"),
	},
	downloadVersionKey{"v7.1.1", "linux", "arm", true}: downloadVersion{
		sha256: lib.MustHexSHA256("5e5dd5fd6282e6e1d1512e696e8ab8223321d1a80d3a30233a41ceedfb2e40e1"),
	},
	// Teleport v7.1.0 OSS binaries
	downloadVersionKey{"v7.1.0", "darwin", "amd64", false}: downloadVersion{
		sha256: lib.MustHexSHA256("ccf302455206adbf53bf907189fcfd076df171908678847d1e4b6a3f1e53a52b"),
	},
	downloadVersionKey{"v7.1.0", "linux", "amd64", false}: downloadVersion{
		sha256: lib.MustHexSHA256("bc66dba99dca0809db28369ac04de4e2bd954f3e9d9657c67756803adef24227"),
	},
	downloadVersionKey{"v7.1.0", "linux", "arm64", true}: downloadVersion{
		sha256: lib.MustHexSHA256("55c8e9e757b9536d322a82114fa30557881fb3ebd824f0ed0a4cb467d68fca8a"),
	},
	downloadVersionKey{"v7.1.0", "linux", "arm", true}: downloadVersion{
		sha256: lib.MustHexSHA256("2cb2f316a4b2bddf0b8f348fde842d2c69ca894bb0868ade5712ef3dc14edf38"),
	},
}

// GetEnterprise downloads a Teleport Enterprise distribution.
func GetEnterprise(ctx context.Context, ver, outDir string) (BinPaths, error) {
	logger.Get(ctx).Debugf("Looking up Teleport Enterprise distribution %s", ver)
	key := downloadVersionKey{
		ver:        ver,
		os:         runtime.GOOS,
		arch:       runtime.GOARCH,
		enterprise: true,
	}
	version, ok := downloadVersions[key]
	if !ok {
		return BinPaths{}, trace.NotFound("teleport enterprise version %s-%s-%s is unknown", key.ver, key.os, key.arch)
	}
	distStr := fmt.Sprintf("teleport-ent-%s-%s-%s", key.ver, key.os, key.arch)
	return getBinaries(ctx, distStr, outDir, version.sha256)
}

// GetEnterprise downloads a Teleport OSS distribution.
func GetOSS(ctx context.Context, ver, outDir string) (BinPaths, error) {
	logger.Get(ctx).Debugf("Looking up Teleport OSS distribution %s", ver)
	key := downloadVersionKey{
		ver:  ver,
		os:   runtime.GOOS,
		arch: runtime.GOARCH,
	}
	version, ok := downloadVersions[key]
	if !ok {
		return BinPaths{}, trace.NotFound("teleport oss version %s-%s-%s is unknown", key.ver, key.os, key.arch)
	}
	distStr := fmt.Sprintf("teleport-%s-%s-%s", key.ver, key.os, key.arch)
	return getBinaries(ctx, distStr, outDir, version.sha256)
}

func getTarball(ctx context.Context, url *url.URL, outFile *os.File, checksum lib.SHA256Sum) (*os.File, error) {
	log := logger.Get(ctx)
	var err error

	outFileInfo, err := outFile.Stat()
	if err != nil {
		return nil, trace.NewAggregate(err, outFile.Close())
	}
	if outFileInfo.Size() > 0 {
		log.Debugf("Found Teleport tarball %s, calculating its checksum", outFile.Name())
		// Check if we have a tarball cached with a correct sha256 sum.
		sha256 := lib.NewSHA256()
		if _, err = io.Copy(sha256, outFile); err != nil {
			return nil, trace.NewAggregate(err, outFile.Close())
		}
		if sha256.Sum() == checksum {
			log.Debugf("Checksum of the Teleport tarball %s is correct", outFile.Name())
			return outFile, nil
		}
		log.Warningf("Teleport tarball %s checksum is incorrect. Need to redownload it", outFile.Name())
		// Rewind the file to the beginning and rewrite it.
		if _, err = outFile.Seek(0, 0); err != nil {
			return nil, trace.NewAggregate(err, outFile.Close())
		}
	}
	log.Debugf("Downloading Teleport tarball from %s", url)
	if err := outFile.Truncate(0); err != nil {
		return nil, trace.NewAggregate(err, outFile.Close())
	}
	if err := lib.DownloadAndCheck(ctx, url.String(), outFile, checksum); err != nil {
		return nil, trace.NewAggregate(err, outFile.Close())
	}
	return outFile, nil
}

func getBinaries(ctx context.Context, distStr, outDir string, checksum lib.SHA256Sum) (BinPaths, error) {
	log := logger.Get(ctx)

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return BinPaths{}, trace.Wrap(err)
	}

	outExtractDir := path.Join(outDir, distStr+"-bin")

	outFileName := distStr + "-bin.tar.gz"
	outFilePath := path.Join(outDir, outFileName)
	outFile, err := os.OpenFile(outFilePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return BinPaths{}, trace.Wrap(err)
	}

	// Make sure no other downloader does access the tarball.
	backoff := backoff.NewDecorrWithMul(500*time.Millisecond, 7*time.Second, 5, clockwork.NewRealClock())
	for {
		err := syscall.Flock(int(outFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Successfully acquired the advisory lock.
			// Once the file is closed it will be unlocked too.
			break
		}
		if err != syscall.EWOULDBLOCK {
			// Advisory lock is acquired by another process.
			return BinPaths{}, trace.NewAggregate(trace.ConvertSystemError(err), outFile.Close())
		}
		log.Debugf("File %s is occupied by another process, lets wait...", outFile.Name())
		if err := backoff.Do(ctx); err != nil {
			return BinPaths{}, trace.NewAggregate(trace.ConvertSystemError(err), outFile.Close())
		}
	}

	existingPaths := BinPaths{
		Teleport: path.Join(outExtractDir, "teleport"),
		Tctl:     path.Join(outExtractDir, "tctl"),
	}

	if fileExists(existingPaths.Teleport) && fileExists(existingPaths.Tctl) {
		log.Debugf("Teleport binaries are found in %s. No need to download anything", outExtractDir)
		return existingPaths, trace.Wrap(outFile.Close())
	}

	url, err := url.Parse("https://get.gravitational.com/" + outFileName)
	if err != nil {
		return BinPaths{}, trace.Wrap(err)
	}
	tarFile, err := getTarball(ctx, url, outFile, checksum)
	if err != nil {
		return BinPaths{}, trace.Wrap(err)
	}
	if _, err = tarFile.Seek(0, 0); err != nil {
		return BinPaths{}, trace.NewAggregate(err, tarFile.Close())
	}

	// Downloading file could take a long time, lets check if can proceed further.
	select {
	case <-ctx.Done():
		return BinPaths{}, trace.NewAggregate(ctx.Err(), tarFile.Close())
	default:
	}

	tarOptions := tar.ExtractOptions{
		Compression:     tar.GzipCompression,
		OutDir:          outExtractDir,
		StripComponents: 1,
		OutFiles:        make(map[string]string),
	}
	if strings.HasPrefix(distStr, "teleport-ent") {
		tarOptions.Files = []string{"teleport-ent/teleport", "teleport-ent/tctl"}
	} else {
		tarOptions.Files = []string{"teleport/teleport", "teleport/tctl"}
	}

	log.Debugf("Extracting Teleport binaries into %s", outExtractDir)

	if err := os.MkdirAll(outExtractDir, 0755); err != nil {
		return BinPaths{}, trace.NewAggregate(err, tarFile.Close())
	}
	if err := trace.NewAggregate(tar.Extract(tarFile, tarOptions), tarFile.Close()); err != nil {
		return BinPaths{}, trace.Wrap(err)
	}

	return BinPaths{
		Teleport: tarOptions.OutFiles[tarOptions.Files[0]],
		Tctl:     tarOptions.OutFiles[tarOptions.Files[1]],
	}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
