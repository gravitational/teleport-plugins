package msapi

import "time"

const (
	backoffBase = time.Millisecond
	backoffMax  = time.Second * 5
	httpTimeout = time.Second * 30
)

// Config represents MS Graph API and Bot API config
type Config struct {
	// AppID application id (uuid, for bots must be underlying app id, not bot's id)
	AppID string `toml:"app_id"`
	// AppSecret application secret token
	AppSecret string `toml:"app_secret"`
	// TenantID ms tenant id
	TenantID string `toml:"tenant_id"`
	// Region bot framework api AP region
	Region string `toml:"region"`
	// TeamsAppID represents Teams App ID
	TeamsAppID string `toml:"teams_app_id"`
	// url represents url configuration for testing
	url struct {
		tokenBaseURL        string
		graphBaseURL        string
		botFrameworkBaseURL string
	} `toml:"-"`
}

// SetBaseURLs is used to point MS Graph API to test servers
func (c *Config) SetBaseURLs(token, graph, bot string) {
	c.url.tokenBaseURL = token
	c.url.graphBaseURL = graph
	c.url.botFrameworkBaseURL = bot
}
