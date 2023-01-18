# Testing access plugins

## What

The library which allows to run an access plugin integration tests using the standalone Teleport binary instead of the standard integration testing package.

## Why

Having Teleport as the dependency is hard to maintain. Teleport requires pre-built binary files to compile. Those files are generated in the Teleport `Makefile` before build. `go mod tidy` fails because it does not know anything about those preliminary steps.

## Details

Access plugins generate various effects based on a Teleport cluster events. For example, email plugin sends an email when access request is created, updated or destroyed. `event-handler` sends all events to a fluentd instance, and so forth.

All plugins require Teleport instance and a target service instance to work. 

We can judge if a plugin works correctly by analysing a target service output. Hence, to make an integration test work, we need to start the standalone Teleport instance, the plugin pointing to that instance and mock the target service to be able to check its output.

[testing](lib/testing) package facilitates the implementation of this scenario.

### Startup sequence

The integration test startup sequence looks as following:

1. If Teleport executables are not available in `$PATH`, they are downloaded to a temporary folder.
2. Teleport instance is started in a temporary directory with the requested configuration and services enabled. Configuration includes predefined users and roles.
3. Mock plugin target service is started.
3. Plugin service is started.

Tests run.

### Configuration variables

* `TELEPORT_GET_VERSION` - requested Teleport version. If Teleport binaries are unavailable in `$PATH` or the available Teleport version mismatches the requested - the required Teleport binaries are downloaded and extracted into `<project_root>/.teleport` folder. Please, check [download.go](lib/testing/integration/download.go) for the list of predefined binaries.
* `TELEPORT_BINARY` - path to teleport binary (default: `teleport`).
* `TELEPORT_BINARY_TCTL` - path to tctl binary (default: `tctl`).
* `TELEPORT_ENTERPRISE_LICENSE` - path to license file (default: `/var/lib/teleport/license.pem`).
* `CI` - indicates that tests are run on the CI, the existence of the Enterprices license is assumed.

Example:

```TELEPORT_GET_VERSION=9.3.0 go test```

### Import

```go
import "github.com/gravitational/teleport-plugins/lib/testing/integration"
```

### Define the test suite

First, we need to define the test suite. `integration.Suite` struct implements context management, setup and test helper methods.

There are the following custom suite types defined:

```go
// Starts AuthService
type AuthSetup struct {
	BaseSetup
	Auth         *AuthService
	CacheEnabled bool
}

// Starts ProxyService
type ProxySetup struct {
	AuthSetup
	Proxy *ProxyService
}

// Starts SSHService
type SSHSetup struct {
	ProxySetup
	SSH *SSHService
}
```

Each type implements custom setup logic and gives access to a service instance.

Define your test suite:

```go
type TestSuite struct {
	// Indicates that AuthService is sufficient for this test
	integration.AuthSetup
	// clients represents the set of connections to Teleport instance using different roles
	clients          map[string]*integration.Client
	// teleportFeatures represents Teleport feature flags (including Teleport enterprise features)
	teleportFeatures *proto.Features
	// teleportConfig represents Teleport access configuration
	teleportConfig   lib.TeleportConfig
	// admin admin user name
	admin string
	// regularUser regular user name
	regularUser string
	// pluginUser user name
	pluginUser string
}
```

### Suite contexts

`teleportTesting.Suite` uses two contexts `app` and `test`. `app` is passed to a plugin service, `test` is passed to a test method. Both contexts are `WithTimeout`. It guarantees that tests won't be run forever. 

`test` context fails `500ms` earlier. It guarantees that error happened in a test would be shown first, before the plugin service fails with timeout.

Suite has `SetContextTimeout` method. It sets the base timeout for both contexts and returns the new `test` context. This method is called during the setup phase with `5m` by default. The timeout value depends on a nature of your test and must include overhead on a possible Teleport binary download.

### Setup suite

Several things need to happend to make our test suite work. We need the setup method, which:

1. Starts Teleport instance.
2. Creates the admin user and saves its connection for later API calls.
3. Gets and saves server features.
4. Creates the regular user and saves its connection for later API calls.
5. Creates plugin user and saves the identity file for later use in the plugin service configuration.

The setup method will look as following:

```go
func (s *TestSuite) SetupSuite() {
	s.clients = make(map[string]*integration.Client)

	// Here all the magic happens. Teleport is downloaded and started, AuthService is enabled.
	s.AuthSetup.SetupSuite()
	s.AuthSetup.SetupService()

	// Create the admin user and get the Teleport connection under his name
	s.admin = "admin@example.com"
	client, _ := s.Integration.MakeAdmin(s.Context(), s.Auth, s.admin)
	s.clients[s.admin] = client

	// Get the server features.
	pong, _ := client.Ping(s.Context())
	s.teleportFeatures := pong.GetServerFeatures()

	// Bootstrap struct contains predefined resource definitions (users and roles, for now).
	var bootstrap integration.Bootstrap

	// Set up user who can request the access to role "editor".
	conditions := types.RoleConditions{
		Request: &types.AccessRequestConditions{Roles: []string{"editor"}},
	}
	role, _ := bootstrap.AddRole("foo", types.RoleSpecV6{Allow: conditions})
	user, _ := bootstrap.AddUserWithRoles("user@example.com", role.GetName())
	s.regularUser = user.GetName()

	// Set up the plugin user
	role, _ = bootstrap.AddRole("access-email", types.RoleSpecV6{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("access_request", []string{"list", "read"}),
				types.NewRule("access_plugin_data", []string{"update"}),
			},
		},
	})
	user, _ = bootstrap.AddUserWithRoles("access-email", role.GetName())
	s.pluginUser = user.GetName()

	// Create users and roles defined above
	teleport.Bootstrap(s.Context(), auth, bootstrap.Resources())

	// Read the *teleport.Client instance for the regularUser
	client, _ = teleport.NewClient(s.Context(), auth, s.regularUser)
	s.clients[s.regularUser] = client

	// Save the identity file for the plugin user. It is typically required by a plugin to connect.
	// We do not need plugin user connection in our test code.
	identityPath, err := teleport.Sign(ctx, auth, s.userNames.plugin)
	require.NoError(t, err)

	// Save the instance params for later use
	s.teleportConfig.Addr = auth.AuthAddr().String()
	s.teleportConfig.Identity = identityPath
}
```

Ideally, those steps should happen before every test method is run. Practically, such approach would increase tests duration dramatically. The drawback is that the Teleport state does not get reset between tests.

### Setup test

Now, we need to setup a mock target service and a plugin service. 

All access plugins must meet the following interface:

```go
// AppI is an app that can be spawned along with running test.
type AppI interface {
	// Run starts the application
	Run(ctx context.Context) error
	// WaitReady waits till the application finishes initialization
	WaitReady(ctx context.Context) (bool, error)
	// Err returns last error
	Err() error
	// Shutdown shuts the application down
	Shutdown(ctx context.Context) error
}
```

They can be started and stopped programmatically. It is handled by the suite. All we need is to provide correct configuration to a plugin service object.

Let's say we want to start mock SMTP server and email plugin service instance. We want email plugin to connect the test Teleport instance.

```go
func (s *TestSuite) SetupTest() {
	// MockMailgunServer is the net/http/httptest struct with blows and whistles
	s.mockMailgun = NewMockMailgunServer()
	s.mockMailgun.Start()

	// Config is the email plugin configuration structure
	var conf Config
	conf.Teleport = s.teleportConfig // This config points to the test instance
	conf.Mailgun = &MailgunConfig{
		APIBase:    s.mockMailgun.GetURL(),
	}
	s.appConfig = conf

	// Initialize email plugin application service structure and start it
	app, _ := NewApp(s.appConfig)
	s.StartApp(app)
}
```

### Enterprise feature flag

The following snippet skips current test unless Teleport has Entrerprise features enabled. 

```go
if !s.teleportFeatures.AdvancedAccessWorkflows {
	t.Skip("Doesn't work in OSS version")
}
```

### Writing the test

Let's ensure that a email plugin sends the specific number of emails. We have mock SMTP server and plugin service up and running. 

We need to emulate access request creation on the behalf of the regular user and ensure that Teleport creates the required event, plugin processes it correctly and the email is sent. 

The mock SMTP server writes all received message to the buffered channel which we could read later on.

```go
func (s *TestSuite) TestNewThreads() {
	t := s.T()

	// Create AccessRequest object
	req, err := types.NewAccessRequest(uuid.New().String(), s.regularUser, "editor")
	require.NoError(t, err)
	req.SetRequestReason("ASAP")
	req.SetSuggestedReviewers([]string{"reviewer1@example.com", "reviewer2@example.com"})

	// Get API connection for the regular user
	client := s.clients[s.regularUser]

	// Send the request via API. We expect three emails to be sent.
	err = client.CreateAccessRequest(s.Context(), req)
	require.NoError(t, err)

	// We expect three messages to be generated. 
	// First one is sent to "all@example.com", other two are sent to our reviewers.
	// If there are less than three messages generated, getMessages will fail on timeout.
	messages := s.getMessages(s.Context() t, 3)

	// 3 messages were received
	assert.Len(t, messages, 3)

	// Ensure that all messages belong to the Request generated in this method
	assert.Contains(t, messages[0], request.GetName())
	assert.Contains(t, messages[1], request.GetName())
	assert.Contains(t, messages[2], request.GetName())
}

// getMessages returns next n email messages
func (s *TestSuite) getMessages(ctx context.Context, t *testing.T, n int) []MockMailgunMessage {
	messages := make([]MockMailgunMessage, n)
	for i := 0; i < n; i++ {
		m, err := s.mockMailgun.GetMessage(ctx)
		require.NoError(t, err)
		messages[i] = m
	}

	return messages
}
```

Please refer to [email plugin test](access/email/email_test.go), [event handler test](event-handler/event_handler_test.go) and others for further examples.
