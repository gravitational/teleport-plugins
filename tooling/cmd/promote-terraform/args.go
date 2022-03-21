package main

import (
	"os"
	"strings"

	"github.com/gravitational/kingpin"
)

type bucketConfig struct {
	region          string
	bucketName      string
	accessKeyID     string
	secretAccessKey string
}

type args struct {
	providerTag       string
	workingDir        string
	registryURL       string
	staging           bucketConfig
	production        bucketConfig
	signingKeyText    string
	protocolVersions  []string
	providerNamespace string
	providerName      string
	verbosity         int
}

func parseCommandLine() *args {
	app := kingpin.New("update-registry", "Adds files to a terraform registry")
	result := &args{}

	app.Flag("tag", "The version tag identifying  version of provider to promote").
		Required().
		StringVar(&result.providerTag)

	app.Flag("staging-bucket", "S3 Staging bucket url (where to fetch tarballs for promotion)").
		Envar("STAGING_BUCKET").
		Required().
		StringVar(&result.staging.bucketName)

	app.Flag("staging-region", "AWS region the staging bucket is in").
		Default("us-west-2").
		StringVar(&result.staging.region)

	app.Flag("staging-access-key-id", "AWS access key id for staging bucket").
		Envar("STAGING_ACCESS_KEY_ID").
		Required().
		StringVar(&result.staging.accessKeyID)

	app.Flag("staging-secret-access-key", "AWS secret access key for staging bucket").
		Envar("STAGING_SECRET_ACCESS_KEY").
		Required().
		StringVar(&result.staging.secretAccessKey)

	app.Flag("prod-bucket", "S3 production bucket url (where to push the resulting registry)").
		Envar("PROD_BUCKET").
		StringVar(&result.production.bucketName)

	app.Flag("prod-region", "AWS region the production bucket is in").
		Default("us-east-1").
		StringVar(&result.production.region)

	app.Flag("prod-access-key-id", "AWS access key id for production bucket").
		Envar("PROD_ACCESS_KEY_ID").
		Required().
		StringVar(&result.production.accessKeyID)

	app.Flag("prod-secret-access-key", "AWS secret access key for production bucket").
		Envar("PROD_SECRET_ACCESS_KEY").
		Required().
		StringVar(&result.production.secretAccessKey)

	app.Flag("working-dir", "Working directory to store generated files").
		Short('d').
		Default("./workspace").
		StringVar(&result.workingDir)

	app.Flag("signing-key", "GPG signing key in ASCII armor format").
		Short('k').
		Envar("SIGNING_KEY").
		StringVar(&result.signingKeyText)

	app.Flag("protocol", "Terraform protocol supported by files").
		Short('p').
		Default("4.0", "5.1").
		StringsVar(&result.protocolVersions)

	app.Flag("registry-url", "Address where registry objects will be served.").
		Default("https://terraform.releases.teleport.dev/").
		StringVar(&result.registryURL)

	app.Flag("namespace", "Terraform provider namespace").
		Default("gravitational").
		StringVar(&result.providerNamespace)

	app.Flag("name", "Terraform provider name").
		Default("teleport").
		StringVar(&result.providerName)

	app.Flag("verbose", "Output more trace output").
		Short('v').
		CounterVar(&result.verbosity)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Marshal the arguments into a canonical format here, so we don't have to
	// second guess the format later on when we're in the thick of doing the
	// actual work...

	if !strings.HasSuffix(result.registryURL, "/") {
		result.registryURL = result.registryURL + "/"
	}

	return result
}
