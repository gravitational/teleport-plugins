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

func readFlags() (string, string, string, string, error) {
	tag := flag.String("tag", "", "tag of Teleport and Teleport Plugins to release")
	teleportCommit := flag.String("teleport-commit", "", "Teleport commit to tag and release")
	teleportChangelog := flag.String("teleport-changelog", "", "Path to Teleport changelog file")
	pluginsCommit := flag.String("plugins-commit", "", "Teleport Plugins commit to tag and release")

	flag.Parse()

	if *tag == "" || *teleportCommit == "" || *pluginsCommit == "" {
		return "", "", "", "", fmt.Errorf("usage: release -tag v1.2.3 --teleport-commit 419119f8 --plugins-commit a59afea8")
	}
	if semver.Canonical(*tag) == "" {
		return "", "", "", "", fmt.Errorf("invalid tag %v", *tag)
	}

	changelog := *teleportChangelog
	//changelog, err := ioutil.ReadFile(*teleportChangelog)
	//if err != nil {
	//	return "", "", "", "", trace.Wrap(err)
	//}

	return *tag, *teleportCommit, string(changelog), *pluginsCommit, nil
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

func releaseTeleport(ctx context.Context, droneClient drone.Client, ghClient *github.Client, tag string, commit string, changelog string) error {
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
	if err := createTags(ctx, ghClient, "teleport-plugins", append(tags, tag), commit); err != nil {
		return trace.Wrap(err)
	}

	// Monitor the builds, once complete, promote.
	if err := build(droneClient, "teleport-plugins", tags); err != nil {
		return trace.Wrap(err)
	}

	// Create the GitHub release.
	if err := createRelease(ctx, ghClient, "teleport-plugins", tag, ""); err != nil {
		return trace.Wrap(err)
	}

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
	for i, tag := range tags {
		_, _, err := client.Git.CreateRef(ctx, "gravitational", repoName, &github.Reference{
			Ref: github.String(fmt.Sprintf("refs/tags/%v", tag)),
			Object: &github.GitObject{
				SHA: github.String(commit),
			},
		})
		if err != nil {
			return trace.Wrap(err)
		}
		fmt.Printf("Created tag %v/%v %v.\n", i+1, len(tags), tag)
	}
	return nil
}

func build(client drone.Client, repoName string, tags []string) error {
	// Wait for Drone to finish building artifacts for the tag.
	if err := waitForTags(client, repoName, tags, ""); err != nil {
		return trace.Wrap(err)
	}

	// Promote artifacts to "production".
	if err := promote(client, repoName, tags); err != nil {
		return trace.Wrap(err)
	}

	// Wait for Drone to finish promoting all artifacts to "production".
	if err := waitForTags(client, repoName, tags, "production"); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func waitForTags(client drone.Client, repoName string, tags []string, deploy string) error {
	desc := "build"
	if deploy == "production" {
		desc = "deployment build"
	}

	ticker := time.NewTicker(2 * time.Second)
	timer := time.NewTimer(30 * time.Minute)

	var i int
	for i < len(tags) {
		select {
		case <-ticker.C:
			// Get list of active builds.
			builds, err := client.BuildList("gravitational", repoName, drone.ListOptions{
				Page: 0,
				Size: 40,
			})
			if err != nil {
				fmt.Printf("Failed to find %v list: %v, continuing.\n", desc, err)
				continue
			}

			// Check to see if all the required builds have completed.
			b, err := matchBuild(builds, tags[i], deploy)
			if err != nil {
				fmt.Printf("Failed to find %v for %v, continuing.\n", desc, tags[i])
				continue
			}

			fmt.Printf("Found %v #%v %v/%v %v.\n", desc, b.Number, i+1, len(tags), tags[i])
			i += 1
		case <-timer.C:
			return fmt.Errorf("timed out waiting tag %v", tags[i])
		}
	}

	return nil
}

func promote(client drone.Client, repoName string, tags []string) error {
	// Get list of active builds.
	builds, err := client.BuildList("gravitational", repoName, drone.ListOptions{
		Page: 0,
		Size: 40,
	})
	if err != nil {
		return err
	}

	for _, tag := range tags {
		// Find matching tag that has finished building.
		b, err := matchBuild(builds, tag, "")
		if err != nil {
			return trace.Wrap(err)
		}

		// Promote tag to "production".
		_, err = client.Promote("gravitational", repoName, int(b.Number), "production", nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func matchBuild(builds []*drone.Build, tag string, deploy string) (*drone.Build, error) {
	for _, build := range builds {
		if strings.HasSuffix(build.Ref, tag) &&
			build.Deploy == deploy &&
			build.Status == "success" {
			return build, nil
		}
	}
	return nil, trace.NotFound("not found")
}

type release struct {
	Version   string
	Changelog string
}

func createRelease(ctx context.Context, client *github.Client, repoName string, tag string, changelog string) error {
	// Build release notes. Note that prerelease notes are the same for Teleport
	// and Teleport Plugins.
	var err error
	var prerelease bool
	var releaseNotes string
	switch {
	case semver.Prerelease(tag) != "":
		prerelease = true
		releaseNotes = prereleaseNotes
	case repoName == "teleport":
		releaseNotes, err = teleportNotes(changelog)
		if err != nil {
			return trace.Wrap(err)
		}
	case repoName == "teleport-plugins":
		releaseNotes, err = teleportPluginsNotes(tag)
		if err != nil {
			return trace.Wrap(err)
		}
		//case repoName == "gh-actions-poc":
		//	releaseNotes, err = teleportPluginsNotes(tag)
		//	if err != nil {
		//		return trace.Wrap(err)
		//	}
	}

	// Build release name.
	releaseName := fmt.Sprintf("Teleport %v", strings.TrimPrefix(tag, "v"))
	if repoName == "teleport-plugins" {
		releaseName = fmt.Sprintf("Teleport Plugins %v", strings.TrimPrefix(tag, "v"))
	}

	// Create the tag on the remote repo.
	_, _, err = client.Repositories.CreateRelease(ctx, "gravitational", repoName, &github.RepositoryRelease{
		TagName:    github.String(tag),
		Name:       github.String(releaseName),
		Body:       github.String(releaseNotes),
		Prerelease: github.Bool(prerelease),
	})
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func teleportNotes(changelog string) (string, error) {
	var buffer bytes.Buffer
	t, err := template.New("releaseNotes").Parse(teleportReleaseNotes)
	if err != nil {
		return "", trace.Wrap(err)
	}
	err = t.Execute(&buffer, &release{
		Changelog: changelog,
	})
	if err != nil {
		return "", trace.Wrap(err)
	}

	return buffer.String(), nil
}

func teleportPluginsNotes(tag string) (string, error) {
	var buffer bytes.Buffer
	t, err := template.New("releaseNotes").Parse(teleportPluginsReleaseNotes)
	if err != nil {
		return "", trace.Wrap(err)
	}
	err = t.Execute(&buffer, &release{
		Version: tag,
	})
	if err != nil {
		return "", trace.Wrap(err)
	}

	return buffer.String(), nil
}

func main() {
	// Read in and parse all command line flags.
	tag, teleportCommit, teleportChangelog, pluginsCommit, err := readFlags()
	if err != nil {
		log.Fatalf("Failed to parse flags: %v.", err)
	}
	fmt.Printf("Creating release %v for Teleport and Teleport Plugins.\n\n", tag)

	// Read in credentials and create Drone and GitHub clients.
	droneClient, ghClient, err := buildClients()
	if err != nil {
		log.Fatalf("Failed to read in credentials: %v.", err)
	}

	if err := releaseTeleport(context.Background(), droneClient, ghClient, tag, teleportCommit, teleportChangelog); err != nil {
		log.Fatalf("Failed to release Teleport: %v.", err)
	}

	if err := releaseTeleportPlugins(context.Background(), droneClient, ghClient, tag, pluginsCommit); err != nil {
		log.Fatalf("Failed to release Teleport Plugins: %v.", err)
	}

	fmt.Printf("\nRelease %v successful.\n", tag)
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

	// teleportReleaseNotes is the generic framing of all release notes attached to Teleport releases.
	teleportReleaseNotes = `
## Description

{{.Changelog}}

## Download

Download the current and previous releases of Teleport at https://gravitational.com/teleport/download.
`

	// teleportPluginsReleaseNotes are generic release notes that are attached to all
	// Teleport Plugins releases.
	teleportPluginsReleaseNotes = `
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
