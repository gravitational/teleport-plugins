package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/gravitational/teleport-plugins/tooling/internal/staging"
	"github.com/gravitational/teleport-plugins/tooling/internal/terraform/registry"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

const (
	objectStoreKeyPrefix = "store"
	registryKeyPrefix    = "registry"
)

func main() {
	args := parseCommandLine()
	log.Infof("Version tag is %s\n", args.providerTag)

	workspace, err := ensureWorkspaceExists(args.workingDir)
	if err != nil {
		log.WithError(err).Fatalf("Failed setting up workspace")
	}

	log.StandardLogger().SetLevel(log.DebugLevel)

	signingKey, err := loadSigningKey(args.signingKeyText)
	if err != nil {
		log.WithError(err).Fatalf("Failed decosing signing key")
	}

	files, err := downloadStagedArtifacts(context.Background(), args.providerTag, workspace.stagingDir, &args.staging)
	if err != nil {
		log.WithError(err).Fatalf("Failed fetching artifacts")
	}

	objectStoreUrl := args.registryURL + "store/"

	versionRecord := registry.Version{
		Protocols: args.protocolVersions,
	}

	for _, fn := range files {
		if !registry.IsProviderTarball(fn) {
			continue
		}

		log.Infof("Found provider tarball %s", fn)

		registryInfo, err := registry.RepackProvider(workspace.objectStoreDir, fn, signingKey)
		if err != nil {
			log.WithError(err).Fatalf("Failed repacking provider")
		}

		log.Infof("Provider repacked to %s/%s", workspace.objectStoreDir, registryInfo.Zip)

		if versionRecord.Version == "" {
			versionRecord.Version = registryInfo.Version
		} else if versionRecord.Version != registryInfo.Version {
			log.Fatalf("Version mismatch. Expected %s, got %s", versionRecord.Version, registryInfo.Version)
		}

		downloadInfo, err := registry.NewDownloadFromRepackResult(registryInfo, args.protocolVersions, objectStoreUrl)
		if err != nil {
			log.WithError(err).Fatalf("Failed creating download info record")
		}

		err = downloadInfo.Save(workspace.registryDir, args.providerNamespace, args.providerName, registryInfo.Version)
		if err != nil {
			log.WithError(err).Fatalf("Failed saving download info record")
		}

		versionRecord.Platforms = append(versionRecord.Platforms, registry.Platform{
			OS:   registryInfo.OS,
			Arch: registryInfo.Arch,
		})
	}

	err = updateRegistry(context.Background(), &args.production, workspace.registryDir, args.providerNamespace, args.providerName, &versionRecord)
	if err != nil {
		log.WithError(err).Fatal("Failed updating registry")
	}
}

func updateRegistry(ctx context.Context, prodBucket *bucketConfig, registryDir, namespace, provider string, newVersion *registry.Version) error {
	s3client, err := newS3ClientFromBucketConfig(ctx, prodBucket)
	if err != nil {
		return trace.Wrap(err)
	}

	versionsFileKey, versionsFilePath := makeVersionsFilePaths(registryDir, namespace, provider)
	log.Infof("Downloading version index for %s/%s from %s", namespace, provider, versionsFileKey)

	// Try downloading the versions file. This may not exist in an empty/new
	// registry, so if the download fails with a NotFound error we ignore it
	// and use an empty index.
	versions := registry.NewVersions()
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

	versions.Versions = append(versions.Versions, newVersion)

	if err = versions.Save(versionsFilePath); err != nil {
		trace.Wrap(err, "failed saving index file")
	}

	return nil
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

	return s3.NewFromConfig(cfg), nil
}

func loadSigningKey(keyText string) (*openpgp.Entity, error) {
	log.Info("Decoding signing key")

	strings.NewReader(keyText)

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
