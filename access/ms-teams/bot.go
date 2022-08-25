package main

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/gravitational/teleport-plugins/access/ms-teams/msapi"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// UserData represents cached data for a user
type UserData struct {
	// User ID user id
	ID string
	// App app installation for user
	App msapi.InstalledApp
	// Chat chat for user
	Chat msapi.Chat
}

// Bot represents the facade to MS teams API
type Bot struct {
	// Config MS API configuration
	msapi.Config
	// teamsApp represents MS Teams app installed for an org
	teamsApp *msapi.TeamsApp
	// graphClient represents MS API Graph client
	graphClient *msapi.GraphClient
	// botClient represents MS Bot Framework client
	botClient *msapi.BotFrameworkClient
	// mu users access mutex
	mu *sync.RWMutex
	// apps represents the cache of apps
	users map[string]UserData
	// webProxyURL represents Web UI address, if enabled
	webProxyURL *url.URL
	// clusterName cluster name
	clusterName string
}

// NewBot creates new bot struct
func NewBot(c msapi.Config, clusterName, webProxyAddr string) (*Bot, error) {
	var (
		webProxyURL *url.URL
		err         error
	)

	if webProxyAddr != "" {
		webProxyURL, err = lib.AddrToURL(webProxyAddr)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	bot := &Bot{
		Config:      c,
		graphClient: msapi.NewGraphClient(c),
		botClient:   msapi.NewBotFrameworkClient(c),
		users:       make(map[string]UserData),
		webProxyURL: webProxyURL,
		clusterName: clusterName,
		mu:          &sync.RWMutex{},
	}

	return bot, nil
}

// GetTeamsApp finds the application in org store and caches it in a bot instance
func (b *Bot) GetTeamsApp(ctx context.Context) (*msapi.TeamsApp, error) {
	teamsApp, err := b.graphClient.GetTeamsApp(ctx, b.Config.TeamsAppID)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	b.teamsApp = teamsApp
	return b.teamsApp, nil
}

// GetUserIDByEmail gets a user ID by email. NotFoundError if not found.
func (b Bot) GetUserIDByEmail(ctx context.Context, email string) (string, error) {
	user, err := b.graphClient.GetUserByEmail(ctx, email)
	if trace.IsNotFound(err) {
		return "", trace.Wrap(err, "try user id instead")
	} else if err != nil {
		return "", trace.Wrap(err)
	}

	return user.ID, nil
}

// UserExists return true if a user exists. Returns NotFoundError if not found.
func (b Bot) UserExists(ctx context.Context, id string) error {
	_, err := b.graphClient.GetUserByID(ctx, id)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (b Bot) UninstallAppForUser(ctx context.Context, userIDOrEmail string) error {
	if b.teamsApp == nil {
		return trace.Errorf("Bot is not configured, run GetTeamsApp first")
	}

	userID, err := b.getUserID(ctx, userIDOrEmail)
	if err != nil {
		return trace.Wrap(err)
	}

	installedApp, err := b.graphClient.GetAppForUser(ctx, b.teamsApp, userID)
	if trace.IsNotFound(err) {
		// App is already uninstalled, nothing to do
		return nil
	} else if err != nil {
		return trace.Wrap(err)
	}

	err = b.graphClient.UninstallAppForUser(ctx, userID, installedApp.ID)
	return trace.Wrap(err)
}

// FetchUser fetches app id for user, installs app for a user if missing, fetches chat id and saves
// everything to cache. This method is used for priming the cache. Returns trace.NotFound if a
// user was not found.
func (b Bot) FetchUser(ctx context.Context, userIDOrEmail string) (*UserData, error) {
	if b.teamsApp == nil {
		return nil, trace.Errorf("Bot is not configured, run GetTeamsApp first")
	}

	b.mu.RLock()
	d, ok := b.users[userIDOrEmail]
	b.mu.RUnlock()
	if ok {
		return &d, nil
	}

	userID := userIDOrEmail

	userID, err := b.getUserID(ctx, userIDOrEmail)
	if err != nil {
		return &UserData{}, trace.Wrap(err)
	}

	var installedApp *msapi.InstalledApp

	installedApp, err = b.graphClient.GetAppForUser(ctx, b.teamsApp, userID)
	if trace.IsNotFound(err) {
		err := b.graphClient.InstallAppForUser(ctx, userID, b.teamsApp.ID)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		installedApp, err = b.graphClient.GetAppForUser(ctx, b.teamsApp, userID)
		if err != nil {
			return nil, trace.Wrap(err, "Failed to install app %v for user %v", b.teamsApp.ID, userID)
		}
	} else if err != nil {
		return nil, trace.Wrap(err)
	}

	chat, err := b.graphClient.GetChatForInstalledApp(ctx, userID, installedApp.ID)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	d = UserData{userID, *installedApp, chat}

	b.mu.Lock()
	b.users[userIDOrEmail] = d
	b.mu.Unlock()

	return &d, nil
}

// getUserID takes a userID or an email, checks if it exists, and returns the userID.
func (b Bot) getUserID(ctx context.Context, userIDOrEmail string) (string, error) {
	if lib.IsEmail(userIDOrEmail) {
		uid, err := b.GetUserIDByEmail(ctx, userIDOrEmail)
		if err != nil {
			return "", trace.Wrap(err)
		}

		return uid, nil
	}
	_, err := b.graphClient.GetUserByID(ctx, userIDOrEmail)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return userIDOrEmail, nil
}

// PostAdaptiveCardActivity sends the AdaptiveCard to a user
func (b Bot) PostAdaptiveCardActivity(ctx context.Context, userIDOrEmail, cardBody, updateID string) (string, error) {
	userData, err := b.FetchUser(ctx, userIDOrEmail)
	if err != nil {
		return "", trace.Wrap(err)
	}

	id, err := b.botClient.PostAdaptiveCardActivity(
		ctx, userData.App.ID, userData.Chat.ID, cardBody, updateID,
	)
	if err != nil {
		return "", trace.Wrap(err)
	}

	return id, nil
}

// PostMessages sends a message to a set of recipients. Returns array of TeamsMessage to cache.
func (b Bot) PostMessages(ctx context.Context, recipients []string, id string, reqData plugindata.AccessRequestData) ([]TeamsMessage, error) {
	var data []TeamsMessage
	var errors []error

	body, err := BuildCard(id, b.webProxyURL, b.clusterName, reqData, nil)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for _, recipient := range recipients {
		id, err := b.PostAdaptiveCardActivity(ctx, recipient, body, "")
		if err != nil {
			errors = append(errors, trace.Wrap(err))
			continue
		}
		msg := TeamsMessage{
			ID:          id,
			Timestamp:   time.Now().Format(time.RFC822),
			RecipientID: recipient,
		}
		data = append(data, msg)
	}

	if len(errors) == 0 {
		return data, nil
	}

	return data, trace.NewAggregate(errors...)
}

// UpdateMessages posts message updates
func (b Bot) UpdateMessages(ctx context.Context, id string, data PluginData, reviews []types.AccessReview) error {
	var errors []error

	body, err := BuildCard(id, b.webProxyURL, b.clusterName, data.AccessRequestData, reviews)
	if err != nil {
		return trace.Wrap(err)
	}

	for _, msg := range data.TeamsData {
		_, err := b.PostAdaptiveCardActivity(ctx, msg.RecipientID, body, msg.ID)
		if err != nil {
			errors = append(errors, trace.Wrap(err))
		}
	}

	if len(errors) == 0 {
		return nil
	}

	return trace.NewAggregate(errors...)
}
