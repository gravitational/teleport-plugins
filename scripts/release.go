package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/drone/drone-go/drone"
)

var releaseList = []string{
	"teleport-event-handler",
	"teleport-jira",
	"teleport-mattermost",
	"teleport-slack",
	"teleport-pagerduty",
	"terraform-provider-teleport",
}

func readCredentials() (string, string, error) {
	droneServer := os.Getenv("DRONE_SERVER")
	droneToken := os.Getenv("DRONE_TOKEN")

	if droneServer == "" || droneToken == "" {
		return "", "", fmt.Errorf("credentials missing")
	}
	return droneServer, droneToken, nil
}

func hasBuild(name string, builds []*drone.Build) (*drone.Build, error) {
	for _, build := range builds {
		if strings.HasSuffix(build.Ref, name) {
			return build, nil
		}
	}
	return nil, fmt.Errorf("no matching build")
}

func waitForTag(client drone.Client, version string) error {
	ticker := time.NewTicker(10 * time.Second)
	timer := time.NewTimer(5 * time.Minute)
	for {
		select {
		case <-ticker.C:
			log.Printf("Checking if all tags have been built.")
			builds, err := client.BuildList("gravitational", "teleport-plugins", drone.ListOptions{
				Page: 0,
				Size: 10,
			})
			if err != nil {
				return err
			}
			var failed bool
			for _, release := range releaseList {
				build, err := hasBuild(fmt.Sprintf("%v-v%v", release, version), builds)
				if err != nil {
					failed = true
					log.Printf("Failed to find build: %v, waiting.", err)
					continue
				}
				if build.Status != "success" {
					failed = true
					log.Printf("Build state not success, waiting.")
					continue
				}
				fmt.Printf("--> Found: %v.\n", build.Ref)
			}
			if !failed {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("timed out waiting for builds")
		}
	}

}

func main() {
	droneServer, droneToken, err := readCredentials()
	if err != nil {
		log.Fatalf("Failed to read in credentials: %v.", err)
	}

	// Create Drone client authenticted with ambient credentials.
	config := new(oauth2.Config)
	auther := config.Client(
		oauth2.NoContext,
		&oauth2.Token{
			AccessToken: droneToken,
		},
	)
	client := drone.NewClient(droneServer, auther)

	err = waitForTag(client, "7.0.2")
	if err != nil {
		log.Fatalf("Failed waiting for tag: %v.", err)
	}

	if err := promoteAndWait(); err != nil {
		log.Fatalf("Failed to promote: %v.", err)
	}

	// gh create release...
}
