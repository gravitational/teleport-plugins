package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"

	"github.com/drone/drone-go/drone"
	"github.com/google/go-github/v38/github"
	"github.com/gravitational/trace"
)

func readFlags() (string, string, string, error) {
	tag := flag.String("tag", "", "tag of Teleport and Teleport Plugins to release")
	teleportCommit := flag.String("teleport-commit", "", "Teleport commit to tag and release")
	pluginsCommit := flag.String("plugins-commit", "", "Teleport Plugins commit to tag and release")

	flag.Parse()

	if *tag == "" || *teleportCommit == "" || *pluginsCommit == "" {
		return "", "", "", fmt.Errorf("usage: release -tag v1.2.3 --teleport-commit 419119f8 --plugins-commit a59afea8")
	}
	if semver.Canonical(*tag) == "" {
		return "", "", "", fmt.Errorf("invalid tag %v", *tag)
	}

	return *tag, *teleportCommit, *pluginsCommit, nil
}

func buildClients() (drone.Client, *github.Client, error) {
	// Read in Drone credentials.
	droneServer := os.Getenv("DRONE_SERVER")
	droneToken := os.Getenv("DRONE_TOKEN")

	// Read in GitHub credentials.
	homedir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	bytes, err := ioutil.ReadFile(filepath.Join(homedir, ".config", "gh", "hosts.yml"))
	if err != nil {
		return nil, nil, err
	}
	var config ghConfig
	if err := yaml.Unmarshal(bytes, &config); err != nil {
		return nil, nil, trace.Wrap(err)
	}
	ghToken := config.Host.Token

	// Validate all credentials are available.
	if droneServer == "" || droneToken == "" || ghToken == "" {
		fmt.Printf(`Failed to find needed credentials: to use this release script you need
credentials for Drone (to monitor and promote builds) and GitHub (to create
tags and releases).

Found the following credentials:

  droneServer: %v
  droneToken:  %v
  ghToken:     %v

If you are missing Drone credentials, navigate to https://drone.teleport.dev
click on the user icon in the top left corner of the screen and then click on
"User settings". Once on the "User settings" page, copy the environment
variables listed under "Example CLI Usage" to your terminal.

If you are missing GitHub credentials, navigate to https://cli.github.com and
install the "gh" CLI tool and type "gh auth login" to fetch GitHub credentials
for your account.

`, droneServer, droneToken, ghToken)
		return nil, nil, fmt.Errorf("credentials missing")
	}

	// Build Drone and GitHub clients.
	droneConfig := new(oauth2.Config)
	droneClient := drone.NewClient(droneServer, droneConfig.Client(
		oauth2.NoContext,
		&oauth2.Token{
			AccessToken: droneToken,
		},
	))
	ghClient := github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: ghToken,
		},
	)))

	return droneClient, ghClient, nil
}

func releaseTeleport(ctx context.Context, droneClient drone.Client, ghClient *github.Client, tag string, commit string) error {
	//// Create the tags needed for the release.
	//if err := createTags(ghClient, []string{tag}, commit); err != nil {
	//	return trace.Wrap(err)
	//}

	//// Monitor the builds, once complete, promote.
	//if err := build(droneClient, version); err != nil {
	//	return trace.Wrap(err)
	//}

	//// Create the GitHub release.
	//if err := createRelease(ghClient, version); err != nil {
	//	return trace.Wrap(err)
	//}

	//// Create post-release PRs.
	//if err := createPostRelease(ghClient, version); err != nil {
	//	return trace.Wrap(err)
	//}

	return nil
}

func releaseTeleportPlugins(ctx context.Context, droneClient drone.Client, ghClient *github.Client, tag string, commit string) error {
	fmt.Printf("Releasing Teleport Plugins: %v at %v.\n", tag, commit)

	// Teleport Plugins at the moment requires the creation of multiple release
	// tags due to how Drone performs builds. Create a tag for each plugin as
	// well as a meta tag that will be used to create a GitHub release.
	tags := preparePluginsTags(tag)

	// Create the tags needed for the release.
	if err := createTags(ctx, ghClient, "gh-actions-poc", tags, commit); err != nil {
		return trace.Wrap(err)
	}

	// Monitor the builds, once complete, promote.
	if err := build(droneClient, tags); err != nil {
		return trace.Wrap(err)
	}

	//// Create the GitHub release.
	//if err := createRelease(ctx, ghClient, tag); err != nil {
	//	return trace.Wrap(err)
	//}

	fmt.Printf("Teleport Plugins successfully released.\n")
	return nil
}

func preparePluginsTags(tag string) []string {
	tags := make([]string, 0, len(pluginsTagPrefixes))
	for _, prefix := range pluginsTagPrefixes {
		tags = append(tags, fmt.Sprintf("%v-%v", prefix, tag))
	}
	return tags
}

func createTags(ctx context.Context, client *github.Client, repoName string, tags []string, commit string) error {
	for _, tag := range tags {
		_, _, err := client.Git.CreateRef(ctx, "gravitational", repoName, &github.Reference{
			Ref: github.String(fmt.Sprintf("refs/tags/%v", tag)),
			Object: &github.GitObject{
				SHA: github.String(commit),
			},
		})
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func build(client drone.Client, tags []string) error {
	//// Wait for Drone to finish building artifacts for the tag.
	//if err := waitForTags(client, tags, ""); err != nil {
	//	return trace.Wrap(err)
	//}

	//// Promote artifacts to "production".
	//if err := promote(client, tags); err != nil {
	//	return trace.Wrap(err)
	//}

	//// Wait for Drone to finish promoting all artifacts to "production".
	//if err := waitForTags(client, tags, "production"); err != nil {
	//	return trace.Wrap(err)
	//}

	return nil
}

func waitForTag(client drone.Client, version string, deploy string) error {
	ticker := time.NewTicker(10 * time.Second)
	timer := time.NewTimer(5 * time.Minute)
	for {
		select {
		case <-ticker.C:
			log.Printf("Checking deploy=%q builds.", deploy)

			// Get list of active builds.
			builds, err := client.BuildList("gravitational", "teleport-plugins", drone.ListOptions{
				Page: 0,
				Size: 20,
			})
			if err != nil {
				log.Printf("Failed to get list of builds: %v.", err)
				continue
			}

			// Check to see if all the required builds have completed.
			if err := checkBuilds(builds, version, deploy); err != nil {
				continue
			}

			log.Printf("All deploy=%q builds ready.", deploy)
			return nil
		case <-timer.C:
			return fmt.Errorf("timed out waiting for builds")
		}
	}
}

func promote(client drone.Client, version string) error {
	//// Get list of active builds.
	//builds, err := client.BuildList("gravitational", "teleport-plugins", drone.ListOptions{
	//	Page: 0,
	//	Size: 40,
	//})
	//if err != nil {
	//	return err
	//}

	//for _, release := range releaseList {
	//	// Find matching tag that has finished building.
	//	build, err := matchBuild(fmt.Sprintf("%v-v%v", release, version), "", builds)
	//	if err != nil {
	//		return err
	//	}

	//	// Promote tag to "production".
	//	_, err = client.Promote("gravitational", "teleport-plugins", int(build.Number), "production", nil)
	//	if err != nil {
	//		return err
	//	}
	//}

	return nil
}

func checkBuilds(builds []*drone.Build, version string, deploy string) error {
	//for _, release := range releaseList {
	//	if _, err := matchBuild(fmt.Sprintf("%v-v%v", release, version), deploy, builds); err != nil {
	//		return err
	//	}
	//}
	return nil
}

func matchBuild(name string, deploy string, builds []*drone.Build) (*drone.Build, error) {
	//for _, build := range builds {
	//	if strings.HasSuffix(build.Ref, name) &&
	//		build.Deploy == deploy &&
	//		build.Status == "success" {
	//		//log.Printf("Build %v ready: deploy=%q ref=%q.", build.Number, build.Deploy, build.Ref)
	//		return build, nil
	//	}
	//}
	return nil, fmt.Errorf("failed to find build(name=%v, deploy=%v).", name)
}

type release struct {
	Version string
}

func createRelease(ctx context.Context, client *github.Client, tag string) error {
	// Check the tag to figure out if this is a prerelease or a production release.
	var prerelease bool
	releaseNotes := pluginsReleaseNotes
	if semver.Prerelease(tag) != "" {
		prerelease = true
		releaseNotes = prereleaseNotes
	}

	// Build out the release notes and inject the version wherever appropriate.
	var buffer bytes.Buffer
	t, err := template.New("releaseNotes").Parse(releaseNotes)
	if err != nil {
		return trace.Wrap(err)
	}
	err = t.Execute(&buffer, &release{
		Version: tag,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	// Create the tag on the remote repo.
	_, _, err = client.Repositories.CreateRelease(ctx, "gravitational", "gh-actions-poc", &github.RepositoryRelease{
		TagName:    github.String(tag),
		Name:       github.String(fmt.Sprintf("Teleport Plugins %v", strings.TrimPrefix(tag, "v"))),
		Body:       github.String(strings.TrimSpace(buffer.String())),
		Prerelease: github.Bool(prerelease),
	})
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func main() {
	// Read in and parse all command line flags.
	tag, teleportCommit, pluginsCommit, err := readFlags()
	if err != nil {
		log.Fatalf("Failed to parse flags: %v.", err)
	}
	fmt.Printf("Creating release %v for Teleport and Teleport Plugins.\n\n", tag)

	// Read in credentials and create Drone and GitHub clients.
	droneClient, ghClient, err := buildClients()
	if err != nil {
		log.Fatalf("Failed to read in credentials: %v.", err)
	}

	if err := releaseTeleport(context.Background(), droneClient, ghClient, tag, teleportCommit); err != nil {
		log.Fatalf("Failed to release Teleport: %v.", err)
	}

	if err := releaseTeleportPlugins(context.Background(), droneClient, ghClient, tag, pluginsCommit); err != nil {
		log.Fatalf("Failed to release Teleport Plugins: %v.", err)
	}

	fmt.Printf("\nRelease %v successful.", tag)
}

var pluginsTagPrefixes = []string{
	"teleport-event-handler",
	"teleport-jira",
	"teleport-mattermost",
	"teleport-slack",
	"teleport-pagerduty",
	"terraform-provider-teleport",
}

type ghConfig struct {
	Host ghHost `yaml:"github.com"`
}

type ghHost struct {
	User     string `yaml:"user"`
	Token    string `yaml:"oauth_token"`
	Protocol string `yaml:"git_protocol"`
}

const (
	// prereleaseNotes are generic release notes that are attached to all releases.
	prereleaseNotes = `
## Warning

Pre-releases are not production ready, use at your own risk!

## Download

Download the current and previous releases of Teleport at https://gravitational.com/teleport/download.`

	// pluginsReleaseNotes are generic release notes that are attached to all
	// Teleport Plugins releases.
	pluginsReleaseNotes = `
## Description

A set of plugins for Teleport's for Access Workflows.

## Documentation

Documentation and guides are available for [Teleport Access Requests](https://goteleport.com/docs/enterprise/workflow), [Terraform Provider](https://goteleport.com/docs/setup/guides/terraform-provider), and [Event Handler](https://goteleport.com/docs/setup/guides/fluentd).

## Download

Download the current release from the links below.

* [Slack](https://get.gravitational.com/teleport-access-slack-{{.Version}}-linux-amd64-bin.tar.gz)
* [Mattermost](https://get.gravitational.com/teleport-access-mattermost-{{.Version}}-linux-amd64-bin.tar.gz)
* [Terraform Provider](https://get.gravitational.com/terraform-provider-teleport-{{.Version}}-linux-amd64-bin.tar.gz)
* [Event Handler](https://get.gravitational.com/teleport-event-handler-{{.Version}}-linux-amd64-bin.tar.gz)
* [PagerDuty](https://get.gravitational.com/teleport-access-pagerduty-{{.Version}}-linux-amd64-bin.tar.gz)
* [Jira](https://get.gravitational.com/teleport-access-jira-{{.Version}}-linux-amd64-bin.tar.gz)`
)
