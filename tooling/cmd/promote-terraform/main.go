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

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/coreos/go-semver/semver"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/teleport-plugins/tooling/internal/staging"
	"github.com/gravitational/teleport-plugins/tooling/internal/terraform/registry"
)

const (
	objectStoreKeyPrefix = "store"
	registryKeyPrefix    = "registry"
)

func main() {
	args := parseCommandLine()

	workspace, err := ensureWorkspaceExists(args.workingDir)
	if err != nil {
		log.WithError(err).Fatalf("Failed setting up workspace")
	}

	log.StandardLogger().SetLevel(logLevel(args))

	signingEntity, err := loadSigningEntity(args.signingKeyText)
	if err != nil {
		log.WithError(err).Fatalf("Failed decoding signing key")
	}

	files, err := downloadStagedArtifacts(context.Background(), args.providerTag, workspace.stagingDir, &args.staging)
	if err != nil {
		log.WithError(err).Fatalf("Failed fetching artifacts")
	}

	objectStoreUrl := args.registryURL + "store/"

	versionRecord, newFiles, err := repackProviders(files, workspace, objectStoreUrl, signingEntity, args.protocolVersions, args.providerNamespace, args.providerName)
	if err != nil {
		log.WithError(err).Fatalf("Failed repacking artifacts")
	}

	err = updateRegistry(context.Background(), &args.production, workspace, args.providerNamespace, args.providerName, versionRecord, newFiles)
	if err != nil {
		log.WithError(err).Fatal("Failed updating registry")
	}
}

func logLevel(args *args) log.Level {
	switch {
	case args.verbosity >= 2:
		return log.TraceLevel

	case args.verbosity == 1:
		return log.DebugLevel

	default:
		return log.InfoLevel
	}
}

// updateRegistry fetches the live registry and adds our new providers to it.
// It's possible for another process to update the `versions` index in the
// bucket while we are modifying it here, and unfortunately AWS doesn't give
// us a nice way to prevent this simply with S3.
//
// We could layer a locking mechanism on top of another AWS service, but for
// now we are relying on drone to honour its concurrency limits (i.e. 1) to
// serialise access to the live `versions` file.
func updateRegistry(ctx context.Context, prodBucket *bucketConfig, workspace *workspacePaths, namespace, provider string, newVersion registry.Version, files []string) error {
	s3client, err := newS3ClientFromBucketConfig(ctx, prodBucket)
	if err != nil {
		return trace.Wrap(err)
	}

	versionsFileKey, versionsFilePath := makeVersionsFilePaths(workspace.registryDir, namespace, provider)
	log.Infof("Downloading version index for %s/%s from %s", namespace, provider, versionsFileKey)

	// Try downloading the versions file. This may not exist in an empty/new
	// registry, so if the download fails with a NotFound error we ignore it
	// and use an empty index.
	versions := registry.Versions{}
	err = download(ctx, s3client, versionsFilePath, prodBucket.bucketName, versionsFileKey)
	switch {
	case err == nil:
		versions, err = registry.LoadVersionsFile(versionsFilePath)
		if err != nil {
			return trace.Wrap(err)
		}

	case isNotFound(err):
		log.Info("No index found. Using empty index.")

	default:
		return trace.Wrap(err, "failed downloading index")
	}

	// Index the available version by their semver version, so that we can find the
	// appropriate release if we're overwriting an existing version
	versionIndex := map[semver.Version]registry.Version{}
	for _, v := range versions.Versions {
		versionIndex[v.Version] = v
	}

	// add/overwrite the existing version entry
	versionIndex[newVersion.Version] = newVersion
	versions.Versions = flattenVersionIndex(versionIndex)

	if err = versions.Save(versionsFilePath); err != nil {
		return trace.Wrap(err, "failed saving index file")
	}
	files = append(files, versionsFilePath)

	// Finally, push the new index files to the production bucket
	err = uploadRegistry(ctx, s3client, prodBucket.bucketName, workspace.productionDir, files)
	if err != nil {
		return trace.Wrap(err, "failed uploading new files")
	}

	return nil
}

func uploadRegistry(ctx context.Context, s3Client *s3.Client, bucketName string, productionDir string, files []string) error {
	uploader := manager.NewUploader(s3Client)
	log.Infof("Production dir: %s", productionDir)
	for _, f := range files {
		log.Infof("Uploading %s", f)

		if !strings.HasPrefix(f, productionDir) {
			return trace.Errorf("file outside of registry dir")
		}

		key, err := filepath.Rel(productionDir, f)
		if err != nil {
			return trace.Wrap(err, "failed to extract key")
		}

		log.Tracef("... to %s", key)

		doUpload := func() error {
			f, err := os.Open(f)
			if err != nil {
				return trace.Wrap(err)
			}
			defer f.Close()

			_, err = uploader.Upload(ctx, &s3.PutObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(key),
				Body:   f,
			})

			return trace.Wrap(err)
		}

		if err = doUpload(); err != nil {
			return trace.Wrap(err, "failed uploading registry file")
		}
	}

	return nil
}

func flattenVersionIndex(versionIndex map[semver.Version]registry.Version) []registry.Version {

	// We want to output a list of versions with semver ordering, so first we
	// generate a sorted list of keys
	keys := make([]semver.Version, 0, len(versionIndex))
	for k, _ := range versionIndex {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].LessThan(keys[j]) })

	// Now we can simply walk the index using the sorted key list and we have
	// our sorted output list
	result := make([]registry.Version, 0, len(keys))
	for _, k := range keys {
		result = append(result, versionIndex[k])
	}

	return result
}

func repackProviders(candidateFilenames []string, workspace *workspacePaths, objectStoreUrl string, signingEntity *openpgp.Entity, protocolVersions []string, providerNamespace, providerName string) (registry.Version, []string, error) {

	versionRecord := registry.Version{
		Protocols: protocolVersions,
	}

	newFiles := []string{}

	unsetVersion := semver.Version{}

	for _, fn := range candidateFilenames {
		if !registry.IsProviderTarball(fn) {
			continue
		}

		log.Infof("Found provider tarball %s", fn)

		registryInfo, err := registry.RepackProvider(workspace.objectStoreDir, fn, signingEntity)
		if err != nil {
			return registry.Version{}, nil, trace.Wrap(err, "failed repacking provider")
		}

		log.Infof("Provider repacked to %s/%s", workspace.objectStoreDir, registryInfo.Zip)
		newFiles = append(newFiles, registryInfo.Zip, registryInfo.Sum, registryInfo.Sig)

		if versionRecord.Version == unsetVersion {
			versionRecord.Version = registryInfo.Version
		} else if !versionRecord.Version.Equal(registryInfo.Version) {
			return registry.Version{}, nil, trace.Wrap(err, "version mismatch. Expected %s, got %s", versionRecord.Version, registryInfo.Version)
		}

		downloadInfo, err := registry.NewDownloadFromRepackResult(registryInfo, protocolVersions, objectStoreUrl)
		if err != nil {
			return registry.Version{}, nil, trace.Wrap(err, "failed creating download info record")
		}

		filename, err := downloadInfo.Save(workspace.registryDir, providerNamespace, providerName, registryInfo.Version)
		if err != nil {
			return registry.Version{}, nil, trace.Wrap(err, "Failed saving download info record")
		}
		newFiles = append(newFiles, filename)

		versionRecord.Platforms = append(versionRecord.Platforms, registry.Platform{
			OS:   registryInfo.OS,
			Arch: registryInfo.Arch,
		})
	}

	return versionRecord, newFiles, nil
}

func download(ctx context.Context, client *s3.Client, dstFileName, bucket, key string) error {

	err := os.MkdirAll(filepath.Dir(dstFileName), 0700)
	if err != nil {
		return trace.Wrap(err, "failed creating destination dir for download")
	}

	dst, err := os.Create(dstFileName)
	if err != nil {
		return trace.Wrap(err, "failed creating destination file download")
	}
	defer dst.Close()

	downloader := manager.NewDownloader(client)

	_, err = downloader.Download(ctx, dst, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return trace.Wrap(err, "failed downloading object")
}

func isNotFound(err error) bool {
	var responseError *awshttp.ResponseError
	if !errors.As(err, &responseError) {
		return false
	}
	return responseError.ResponseError.HTTPStatusCode() == http.StatusNotFound
}

func makeVersionsFilePaths(registryDir, namespace, provider string) (string, string) {
	key := registry.VersionsFilePath(registryKeyPrefix, namespace, provider)
	path := registry.VersionsFilePath(registryDir, namespace, provider)
	return key, path
}

type workspacePaths struct {
	stagingDir     string
	productionDir  string
	registryDir    string
	objectStoreDir string
}

func ensureWorkspaceExists(workspaceDir string) (*workspacePaths, error) {
	// Ensure that the working dir and so on exist
	stagingDir := filepath.Join(workspaceDir, "staging")
	err := os.MkdirAll(stagingDir, 0700)
	if err != nil {
		return nil, trace.Wrap(err, "failed ensuring staging dir %s exists", stagingDir)
	}

	productionDir := filepath.Join(workspaceDir, "production")

	registryDir := filepath.Join(productionDir, "registry")
	err = os.MkdirAll(registryDir, 0700)
	if err != nil {
		return nil, trace.Wrap(err, "failed ensuring registry output dir %s exists", registryDir)
	}

	objectStoreDir := filepath.Join(productionDir, "store")
	err = os.MkdirAll(objectStoreDir, 0700)
	if err != nil {
		return nil, trace.Wrap(err, "failed ensuring registry output dir %s exists", objectStoreDir)
	}

	return &workspacePaths{
		stagingDir:     stagingDir,
		productionDir:  productionDir,
		registryDir:    registryDir,
		objectStoreDir: objectStoreDir,
	}, nil
}

func downloadStagedArtifacts(ctx context.Context, tag string, dstDir string, stagingBucket *bucketConfig) ([]string, error) {
	log.Debugf("listing plugins in %s %s", stagingBucket.region, stagingBucket.bucketName)
	log.Debugf("listing plugins as %s", stagingBucket.accessKeyID)
	client, err := newS3ClientFromBucketConfig(ctx, stagingBucket)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return staging.FetchByTag(ctx, client, dstDir, stagingBucket.bucketName, tag)
}

func newS3ClientFromBucketConfig(ctx context.Context, bucket *bucketConfig) (*s3.Client, error) {

	creds := credentials.NewStaticCredentialsProvider(
		bucket.accessKeyID, bucket.secretAccessKey, "")

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(bucket.region),
		config.WithCredentialsProvider(creds))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if bucket.roleARN == "" {
		return s3.NewFromConfig(cfg), nil
	}

	log.Debugf("Configuring deployment role %q", bucket.roleARN)

	stsClient := sts.NewFromConfig(cfg)
	stsCreds := stscreds.NewAssumeRoleProvider(stsClient, bucket.roleARN)
	stsAwareCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(bucket.region),
		config.WithCredentialsProvider(stsCreds))

	if err != nil {
		return nil, trace.Wrap(err)
	}

	return s3.NewFromConfig(stsAwareCfg), nil
}

func loadSigningEntity(keyText string) (*openpgp.Entity, error) {
	log.Info("Decoding signing key")

	block, err := armor.Decode(strings.NewReader(keyText))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	entity, err := openpgp.ReadEntity(packet.NewReader(block.Body))
	if err != nil {
		return nil, trace.Wrap(err, "failed loading entity from private key")
	}

	return entity, nil
}
