/*
Copyright 2015 Gravitational, Inc.

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

package web

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gravitational/trace"

	"github.com/kardianos/osext"
)

const (
	webAssetsMissingError = "the teleport binary was built without web assets, try building with `make release`"
	webAssetsReadError    = "failure reading web assets from the binary"
)

// NewStaticFileSystem returns the initialized implementation of http.FileSystem
// interface which can be used to serve Teleport Proxy Web UI
func NewStaticFileSystem() (http.FileSystem, error) {
	// Use the zip archive attached to the executable:
	return loadZippedExeAssets()
}

// NewDebugFileSystem returns the HTTP file system implementation rooted
// at the specified assetsPath.
func NewDebugFileSystem(assetsPath string) (http.FileSystem, error) {
	assetsToCheck := []string{"index.html", "/app"}
	if assetsPath == "" {
		exePath, err := osext.ExecutableFolder()
		if err != nil {
			return nil, trace.Wrap(err)
		}

		_, err = os.Stat(path.Join(exePath, "../../e"))
		isEnterprise := !os.IsNotExist(err)

		if isEnterprise {
			// enterprise web assets
			assetsPath = path.Join(exePath, "../../webassets/e/teleport")
		} else {
			// community web assets
			assetsPath = path.Join(exePath, "../webassets/teleport")
		}
	}

	for _, af := range assetsToCheck {
		_, err := os.Stat(filepath.Join(assetsPath, af))
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}
	log.Infof("Using filesystem for serving web assets: %s.", assetsPath)
	return http.Dir(assetsPath), nil
}

// LoadWebResources returns a filesystem implementation compatible
// with http.Serve.
//
// The "filesystem" is served from a zip file attached at the end of
// the executable
//
func loadZippedExeAssets() (ResourceMap, error) {
	// open ourselves (teleport binary) for reading:
	// NOTE: the file stays open to serve future Read() requests
	myExe, err := osext.Executable()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return readZipArchive(myExe)
}

func readZipArchive(archivePath string) (ResourceMap, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// feed the binary into the zip reader and enumerate all files
	// found in the attached zip file:
	info, err := file.Stat()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	zreader, err := zip.NewReader(file, info.Size())
	if err != nil {
		// this often happens when teleport is launched without the web assets
		// zip file attached to the binary. for launching it in such mode
		// set DEBUG environment variable to 1
		if err == zip.ErrFormat {
			return nil, trace.NotFound(webAssetsMissingError)
		}
		return nil, trace.NotFound("%s %v", webAssetsReadError, err)
	}
	entries := make(ResourceMap)
	for _, file := range zreader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		entries[file.Name] = file
	}
	// no entries found?
	if len(entries) == 0 {
		return nil, trace.Wrap(os.ErrInvalid)
	}
	return entries, nil
}

// resource struct implements http.File interface on top of zip.File object
type resource struct {
	reader io.ReadCloser
	file   *zip.File
	pos    int64
}

func (rsc *resource) Read(p []byte) (n int, err error) {
	n, err = rsc.reader.Read(p)
	rsc.pos += int64(n)
	return n, err
}

func (rsc *resource) Seek(offset int64, whence int) (int64, error) {
	var (
		pos int64
		err error
	)
	// zip.File does not support seeking. To implement Seek on top of it,
	// we close the existing reader, re-open it, and read 'offset' bytes from
	// the beginning
	if err = rsc.reader.Close(); err != nil {
		return 0, err
	}
	if rsc.reader, err = rsc.file.Open(); err != nil {
		return 0, err
	}
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = rsc.pos + offset
	case io.SeekEnd:
		pos = int64(rsc.file.UncompressedSize64) + offset
	}
	if pos > 0 {
		b := make([]byte, pos)
		if _, err = rsc.reader.Read(b); err != nil {
			return 0, err
		}
	}
	rsc.pos = pos
	return pos, nil
}

func (rsc *resource) Readdir(count int) ([]os.FileInfo, error) {
	return nil, trace.Wrap(os.ErrPermission)
}

func (rsc *resource) Stat() (os.FileInfo, error) {
	return rsc.file.FileInfo(), nil
}

func (rsc *resource) Close() (err error) {
	log.Debugf("zip::Close(%s).", rsc.file.FileInfo().Name())
	return rsc.reader.Close()
}

type ResourceMap map[string]*zip.File

func (rm ResourceMap) Open(name string) (http.File, error) {
	log.Debugf("GET zip:%s.", name)
	f, ok := rm[strings.Trim(name, "/")]
	if !ok {
		return nil, trace.Wrap(os.ErrNotExist)
	}
	reader, err := f.Open()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &resource{
		reader: reader,
		file:   f,
	}, nil
}
