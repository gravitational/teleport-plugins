package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/drone/drone-go/drone"
	"github.com/google/go-github/v38/github"
)

var releaseList = []string{
	"teleport-event-handler",
	"teleport-jira",
	"teleport-mattermost",
	"teleport-slack",
	"teleport-pagerduty",
	"terraform-provider-teleport",
}

func readFlags() (string, error) {
	version := flag.String("version", "", "version of plugins to release")
	flag.Parse()

	if version == "" {
		log.Fatalf("Version string is missing.")
	}

}

func readCredentials() (string, string, error) {
	droneServer := os.Getenv("DRONE_SERVER")
	droneToken := os.Getenv("DRONE_TOKEN")

	if droneServer == "" || droneToken == "" {
		return "", "", fmt.Errorf("credentials missing")
	}
	return droneServer, droneToken, nil

	client := github.NewClient(nil)

	//droneServer, droneToken, err := readCredentials()
	//if err != nil {
	//	log.Fatalf("Failed to read in credentials: %v.", err)
	//}

}

func prepareReleases(version string) ([]string, error) {
}

func createTags(client github.Client, version string) error {
	tags, _, _ := client.Repositories.ListTags(context.Background(), "gravitational", "teleport-plugins", nil)

	for _, tag := range tags {
		fmt.Printf("--> tag: %v.\n", *tag.Name)
	}
}

func droneBuild(droneServer string, droneToken string, version string) error {

	// Prepare the release tags that will be created for this release.
	releases := prepareReleases(version)

	// Create Drone client authenticted with ambient credentials.
	config := new(oauth2.Config)
	auther := config.Client(
		oauth2.NoContext,
		&oauth2.Token{
			AccessToken: droneToken,
		},
	)
	client := drone.NewClient(droneServer, auther)

	// Wait for Drone to finish building artifacts for the tag.
	if err := waitForTag(client, version, ""); err != nil {
		return err
	}

	//// Promote artifacts to "production".
	//if err := promote(client, version); err != nil {
	//	return err
	//}

	// Wait for Drone to finish promoting all artifacts to "production".
	if err := waitForTag(client, version, "production"); err != nil {
		return err
	}

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
	// Get list of active builds.
	builds, err := client.BuildList("gravitational", "teleport-plugins", drone.ListOptions{
		Page: 0,
		Size: 40,
	})
	if err != nil {
		return err
	}

	for _, release := range releaseList {
		// Find matching tag that has finished building.
		build, err := matchBuild(fmt.Sprintf("%v-v%v", release, version), "", builds)
		if err != nil {
			return err
		}

		// Promote tag to "production".
		_, err = client.Promote("gravitational", "teleport-plugins", int(build.Number), "production", nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkBuilds(builds []*drone.Build, version string, deploy string) error {
	for _, release := range releaseList {
		if _, err := matchBuild(fmt.Sprintf("%v-v%v", release, version), deploy, builds); err != nil {
			return err
		}
	}
	return nil
}

func matchBuild(name string, deploy string, builds []*drone.Build) (*drone.Build, error) {
	for _, build := range builds {
		if strings.HasSuffix(build.Ref, name) &&
			build.Deploy == deploy &&
			build.Status == "success" {
			//log.Printf("Build %v ready: deploy=%q ref=%q.", build.Number, build.Deploy, build.Ref)
			return build, nil
		}
	}
	return nil, fmt.Errorf("failed to find build(name=%v, deploy=%v).", name)
}

func createRelease() error {
	return nil
}

func main() {
	// Read in and parse all command line flags.
	version, err := readFlags()
	if err != nil {
		log.Fatalf("Failed to parse flags: %v.", err)
	}

	// Read in credentials and create Drone and GitHub clients.
	droneClient, ghClient, err := readCredentials()
	if err != nil {
		log.Fatalf("Failed to read in credentials: %v.", err)
	}

	// Create the tags needed for the release.
	if err := createTags(ghClient, version); err != nil {
		log.Fatalf("Failed to create tags: %v.", err)
	}

	// Monitor the builds, once complete, promote.
	if err := build(droneClient, version); err != nil {
		log.Fatalf("Failed to build: %v.", err)
	}

	// Create the GitHub release.
	if err := createRelease(ghClient, version); err != nil {
		log.Fatalf("Failed to create release: %v.", err)
	}

	log.Printf("Successfully released telport-plugins %v.", version)
}
