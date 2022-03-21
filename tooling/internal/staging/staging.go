// Package staging contains tools for manipulating the plugin artifacts
// stored in the staging bucket.
package staging

import (
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	keyPrefix = "teleport-plugins/tag/"
)

// FetchByTag pulls all of the staging artifacts out of the staging storage
// bucket based on the tag naming scheme
func FetchByTag(ctx context.Context, client *s3.Client, dstDir string, bucket, tag string) ([]string, error) {
	taggedPrefix := tagPrefix(tag)

	objectKeys, err := listKeysWithPrefix(ctx, client, bucket, taggedPrefix)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if len(objectKeys) == 0 {
		return nil, nil
	}

	downloader := manager.NewDownloader(client)
	result := make([]string, 0, len(objectKeys))

	for _, key := range objectKeys {
		filename, err := fetchObject(ctx, downloader, dstDir, bucket, key, taggedPrefix)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		result = append(result, filename)
	}

	return result, nil
}

type downloader interface {
	Download(ctx context.Context, w io.WriterAt, input *s3.GetObjectInput, options ...func(*manager.Downloader)) (n int64, err error)
}

func tagPrefix(tag string) string {
	return path.Join(keyPrefix, tag)
}

func fetchObject(ctx context.Context, client downloader, dstDirRoot string, bucket, key string, keyPrefix string) (string, error) {
	dstFilename, err := filenameForKey(key, keyPrefix)
	if err != nil {
		return "", trace.Wrap(err)
	}
	dstFilename = filepath.Join(dstDirRoot, dstFilename)

	log.Infof("Fetching %s", key)
	log.Tracef("... into %s", dstFilename)
	dstDir := filepath.Dir(dstFilename)
	err = os.MkdirAll(dstDir, 0700)
	if err != nil {
		return "", trace.Wrap(err, "failed ensuring dst dir %q exists", dstDir)
	}

	dstFile, err := os.Create(dstFilename)
	if err != nil {
		return "", trace.Wrap(err, "failed creating destination file")
	}
	defer dstFile.Close()

	_, err = client.Download(ctx, dstFile, &s3.GetObjectInput{
		Key:    aws.String(key),
		Bucket: aws.String(bucket),
	})

	if err != nil {
		return "", trace.Wrap(err, "download failed")
	}

	return dstFilename, nil
}

func filenameForKey(key, pfx string) (string, error) {
	if !strings.HasPrefix(key, pfx) {
		return "", trace.Errorf("Key missing staging prefix: %q", key)
	}
	return key[len(pfx):], nil
}

func listKeysWithPrefix(ctx context.Context, client *s3.Client, bucket, pfx string) ([]string, error) {
	log.Infof("Listing objects in %s with key prefix %s", bucket, pfx)
	objectKeys := []string{}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(pfx),
	}

	for {
		output, err := client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, object := range output.Contents {
			objectKeys = append(objectKeys, *object.Key)
		}

		if output.ContinuationToken == nil {
			break
		}

		input.ContinuationToken = output.ContinuationToken
	}

	return objectKeys, nil
}
