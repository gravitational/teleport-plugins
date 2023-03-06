/*
Copyright 2023 Gravitational, Inc.

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
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gravitational/teleport-plugins/lib"
)

//go:embed download_sha.dsv_1204
var downloadVersionsDSV1204 string

func TestDownloadVersionsHash(t *testing.T) {
	dv, ok := downloadVersionsHash(context.TODO(), downloadVersionsDSV1204, downloadVersionKey{
		ver:        "v12.0.4",
		os:         "linux",
		arch:       "amd64",
		enterprise: false,
	})
	require.True(t, ok, "expected to find hash for key, but didn't")
	require.Equal(t, dv.sha256, lib.MustHexSHA256("84ce1cd7297499e6b52acf63b1334890abc39c926c7fc2d0fe676103d200752a"))
}
