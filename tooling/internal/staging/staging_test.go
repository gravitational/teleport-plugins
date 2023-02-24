/*
Copyright 2022 Gravitational, Inc.

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
package staging

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockDownloader struct {
	mock.Mock
}

func (m *mockDownloader) Download(ctx context.Context, w io.WriterAt, input *s3.GetObjectInput, options ...func(*manager.Downloader)) (n int64, err error) {
	result := m.Called(ctx, w, input, options)
	return int64(result.Int(0)), result.Error(1)
}

func TestNonNestedFetch(t *testing.T) {
	const (
		bucket = "some-random-bucket"
		prefix = "teleport-plugins/tag/"
		key    = prefix + "toplevel-key"
	)

	tmpDir := t.TempDir()

	dl := mockDownloader{}
	dl.On("Download", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			input := args.Get(2).(*s3.GetObjectInput)
			require.Equal(t, bucket, *input.Bucket)
			require.Equal(t, key, *input.Key)
		}).
		Return(0, nil)

	dstfile, err := fetchObject(context.Background(), &dl, tmpDir, bucket, key, prefix)
	dl.AssertCalled(t, "Download", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, "toplevel-key")
	require.Equal(t, expected, dstfile)
	require.FileExists(t, expected)
}

func TestNestedFetch(t *testing.T) {
	const (
		bucket = "some-random-bucket"
		prefix = "teleport-plugins/tag/some/"
		key    = prefix + "nested/object/key"
	)

	tmpDir := t.TempDir()

	dl := mockDownloader{}
	dl.On("Download", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			input := args.Get(2).(*s3.GetObjectInput)
			require.Equal(t, bucket, *input.Bucket)
			require.Equal(t, key, *input.Key)
		}).
		Return(0, nil)

	dstfile, err := fetchObject(context.Background(), &dl, tmpDir, bucket, key, prefix)
	dl.AssertCalled(t, "Download", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, "nested", "object", "key")
	require.Equal(t, expected, dstfile)
	require.FileExists(t, expected)
}
