# Using `github.com/gravitational/teleport-plugins/lib/testing` package for writing tests

This package implements Teleport integration testing.

For the complete examples, please refer to [email plugin tests](access/email/email_test.go).

```go
import . "github.com/gravitational/teleport-plugins/lib/testing"
import "github.com/gravitational/teleport-plugins/lib/testing/integration"
```

# Initialization

To start new Teleport instance in background:

```go
teleport, err := integration.NewFromEnv(ctx)
require.NoError(t, err)
defer teleport.Close()

auth, err := teleport.NewAuthService()
require.NoError(t, err)

pong, err := api.Ping(ctx)
require.NoError(t, err)
teleportFeatures := pong.GetServerFeatures()
```

There are several environment variables available:

* `TELEPORT_GET_VERSION` - Teleport version. Will download Teleport binaries if missing in the host system.
* `TELEPORT_BINARY` - path to teleport binary (default: `teleport`).
* `TELEPORT_BINARY_TCTL` - path to tctl binary (default: `tctl`).
* `TELEPORT_ENTERPRISE_LICENSE` - path to license file (default: `/var/lib/teleport/license.pem`).
* `CI` - indicates that tests are run on the CI.

Please, beware of `teleportFeatures` variable.

# Writing tests for Teleport Enterprise

The following snippet skips current test unless Teleport has Entrerprise features enabled. To enable Enterprise features, please use provided license file or put your license to `/var/lib/teleport/license.pem`.

```go
if !teleportFeatures.AdvancedAccessWorkflows {
	t.Skip("Doesn't work in OSS version")
}
```

# Creating default users and roles

Use `integration.Bootstrap` type to create default users and roles. In the following example we create admin and regular user:

```go
var bootstrap integration.Bootstrap

conditions = types.RoleConditions{}
if teleportFeatures.AdvancedAccessWorkflows {
	// For Teleport Enterprise only. This role holders can review admin access request.
	conditions.ReviewRequests = &types.AccessReviewConditions{Roles: []string{"admin"}}
}
role, err = bootstrap.AddRole("admin", types.RoleSpecV4{})
require.NoError(t, err)

role, err = bootstrap.AddRole("user", types.RoleSpecV4{Allow: conditions})
require.NoError(t, err)

adminUser, err = bootstrap.AddUserWithRoles("admin", role.GetName())
require.NoError(t, err)

defaultUser, err = bootstrap.AddUserWithRoles("user", role.GetName())
require.NoError(t, err)

err = teleport.Bootstrap(ctx, auth, bootstrap.Resources())
require.NoError(t, err)
```

# Saving client instances for a users

```go
adminClient, err = teleport.NewClient(ctx, auth, "admin")
require.NoError(t, err)

userClient, err = teleport.NewClient(ctx, auth, "user")
require.NoError(t, err)
```

Use this variables to perform requests on behalf of a specific users.

# Exporting identity file 

You can export identity file for a desired user using the following snippet:

```go
// auth is obtained on initialization
identityPath, err := teleport.Sign(ctx, auth, "user")
require.NoError(t, err)
```

# Implementing an example test

Let's ensure that a plugin generates specific number of API calls. To register incoming calls, we would use `httptest` server incapsulated into [`MockMailgun`](access/email/mock_mailgun.go) type which registers Mailgun API calls. Let's assume that our plugin is running in background (see `startApp()` method in [`email_test.go`](access/email/email_test.go))

Please note that the following snippet is simplified and would not work as is.

```go
func (t *testing.T) TestNewThreads() {
	// Start plugin process in background
	startApp()

	// Create Access Request via API, set requesting user to "requestor@example.com", pass suggested reviewers.
	// This access request will be consumed by the plugin.
	request := createAccessRequest("requestor@example.com", []string{"reviewer1@example.com", "reviewer2@example.com"})

	// We expect three messages to be generated. 
	// First one is sent to "all@example.com", other two are sent to our reviewers.
	// getMessages() method reads three received messages from API mock server (via channel with capacity).
	// If there are less than three messages generated, getMessages will fail on timeout.
	var messages = getMessages(contextWithTimeout, t, 3)

	// 3 messages were received
	assert.Len(t, messages, 3)

	// Ensure that all messages belong to the Request generated in this method
	assert.Contains(t, messages[0], request.GetName())
	assert.Contains(t, messages[1], request.GetName())
	assert.Contains(t, messages[2], request.GetName())
}

```

Please note that Teleport instance does not get restarted between tests. Hence, *Teleport state does not get reset*. Otherwise, it will significantly slow tests down. 

So, in this example, if there are more than three messages generated, they will be processed by the other tests in a suite.  This could lead to unobvious random failures. You need to check that messages you have received belong to the test case you are working with in this specific method.