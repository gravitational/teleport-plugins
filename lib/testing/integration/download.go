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

/*
curl https://get.gravitational.com/teleport-ent-v12.0.1-darwin-amd64-bin.tar.gz.sha256
curl https://get.gravitational.com/teleport-ent-v12.0.1-linux-amd64-bin.tar.gz.sha256
curl https://get.gravitational.com/teleport-ent-v12.0.1-linux-arm64-bin.tar.gz.sha256
curl https://get.gravitational.com/teleport-ent-v12.0.1-linux-arm-bin.tar.gz.sha256

curl https://get.gravitational.com/teleport-v12.0.1-darwin-amd64-bin.tar.gz.sha256
curl https://get.gravitational.com/teleport-v12.0.1-linux-amd64-bin.tar.gz.sha256
curl https://get.gravitational.com/teleport-v12.0.1-linux-arm64-bin.tar.gz.sha256
curl https://get.gravitational.com/teleport-v12.0.1-linux-arm-bin.tar.gz.sha256
*/
var downloadVersions = map[downloadVersionKey]downloadVersion{
	// Teleport v12.0.1 Enterprise binaries
	{"v12.0.1", "darwin", "amd64", true}: {sha256: lib.MustHexSHA256("6e921e6a38295b353d9980d6a51356e540afb92f6ad0101eb917bbca61da80a0")},
	{"v12.0.1", "linux", "amd64", true}:  {sha256: lib.MustHexSHA256("e697de2ddd56fafa4e18761521e6988d37f089f1b2bbbb384c0044f06b444ea1")},
	{"v12.0.1", "linux", "arm64", true}:  {sha256: lib.MustHexSHA256("dfc12bd37e029e838c27e98813fc9350e39dd72583334a5fac195f678079db0a")},
	{"v12.0.1", "linux", "arm", true}:    {sha256: lib.MustHexSHA256("5ba10476b29d3ac79e5b57b1d05bbc3d249c1f4fe37575bbf4dbdb6e5ea39d56")},
	// Teleport v12.0.1 OSS binaries
	{"v12.0.1", "darwin", "amd64", false}: {sha256: lib.MustHexSHA256("51c69eb90339eeada89c0f5af144d3e0e8e45b2aeef70655ec510f84845394a1")},
	{"v12.0.1", "linux", "amd64", false}:  {sha256: lib.MustHexSHA256("aca53d9e24619567caa6de2bf22d19debb77d95f136e48cbfd69b4e5df36ae78")},
	{"v12.0.1", "linux", "arm64", false}:  {sha256: lib.MustHexSHA256("206f2651dd2c8e6adbb6e642bb7f0288c5b898d596c4058c50c4b8734b5ac9c9")},
	{"v12.0.1", "linux", "arm", false}:    {sha256: lib.MustHexSHA256("07767d129c45c299af5668b5794e7306255d1abf49131c03600ec4191ca55f65")},
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

// GetOSS downloads a Teleport OSS distribution.
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
		Tsh:      path.Join(outExtractDir, "tsh"),
	}

	if fileExists(existingPaths.Teleport) && fileExists(existingPaths.Tctl) && fileExists(existingPaths.Tsh) {
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
		tarOptions.Files = []string{"teleport-ent/teleport", "teleport-ent/tctl", "teleport-ent/tsh"}
	} else {
		tarOptions.Files = []string{"teleport/teleport", "teleport/tctl", "teleport/tsh"}
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
		Tsh:      tarOptions.OutFiles[tarOptions.Files[2]],
	}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
