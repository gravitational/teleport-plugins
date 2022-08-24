package main

import (
	"context"
	"fmt"
	"time"

	cards "github.com/DanielTitkov/go-adaptive-cards"
	"github.com/google/uuid"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// validate installs the application for a user if required and sends the Hello, world! message
func validate(configPath, userID string) error {
	var uid = userID

	lib.PrintVersion(appName, Version, Gitref)
	fmt.Println()

	c, err := LoadConfig(configPath)
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Printf(" - Checking application %v status...\n", c.MSAPI.TeamsAppID)

	b, err := NewBot(c.MSAPI, "local", "")
	if err != nil {
		return trace.Wrap(err)
	}

	teamApp, err := b.GetTeamsApp(context.Background())
	if trace.IsNotFound(err) {
		fmt.Printf("Application %v not found in the org app store. Please, ensure that you have the application uploaded and installed for your team.", c.MSAPI.TeamsAppID)
		return nil
	} else if err != nil {
		return trace.Wrap(err)
	} else {
		fmt.Printf(" - Application found in the team app store (internal ID: %v)\n", teamApp.ID)
	}

	if lib.IsEmail(uid) {
		userID, err := b.GetUserIDByEmail(context.Background(), uid)
		if trace.IsNotFound(err) {
			fmt.Printf(" - User %v not found! Try to use user ID instead\n", uid)
			return nil
		}
		if err != nil {
			return trace.Wrap(err)
		}

		fmt.Printf(" - User %v found: %v\n", uid, userID)

		uid = userID
	}

	userData, err := b.FetchUser(context.Background(), uid)
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Printf(" - Application installation ID for user: %v\n", userData.App.ID)
	fmt.Printf(" - Chat ID for user: %v\n", userData.Chat.ID)
	fmt.Printf(" - Chat web URL: %v\n", userData.Chat.WebURL)

	card := cards.New([]cards.Node{
		&cards.TextBlock{
			Text: "Hello, world!",
			Size: "large",
		},
		&cards.TextBlock{
			Text: "*Sincerely yours,*",
		},
		&cards.TextBlock{
			Text: "Teleport Bot!",
		},
	}, []cards.Node{}).
		WithSchema(cards.DefaultSchema).
		WithVersion(cards.Version12)

	body, err := card.StringIndent("", "    ")
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Println(" - Hailing the user...")

	id, err := b.PostAdaptiveCardActivity(context.Background(), userData.ID, body, "")
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Printf(" - Message sent, ID: %v\n", id)

	data := plugindata.AccessRequestData{
		User:          "foo",
		Roles:         []string{"editor"},
		RequestReason: "Example request posted by 'validate' command.",
		ReviewsCount:  1,
	}

	reviews := []types.AccessReview{
		{
			Author:        "bar",
			Roles:         []string{"reviewer"},
			ProposedState: types.RequestState_APPROVED,
			Reason:        "Looks fine",
			Created:       time.Now(),
		},
		{
			Author:        "baz",
			Roles:         []string{"reviewer"},
			ProposedState: types.RequestState_DENIED,
			Reason:        "Not good",
			Created:       time.Now(),
		},
	}

	body, err = BuildCard(uuid.NewString(), nil, "local-cluster", data, reviews)
	if err != nil {
		return trace.Wrap(err)
	}

	_, err = b.PostAdaptiveCardActivity(context.Background(), userData.ID, body, "")
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Println()
	fmt.Println("Check your MS Teams!")

	return nil
}
