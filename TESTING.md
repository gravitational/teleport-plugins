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

auth, err := teleport.NewAuthServer()
require.NoError(t, err)

// integration.*API instance, see below
api, err := teleport.NewAPI(ctx, auth)
require.NoError(t, err)

pong, err := api.Ping(ctx)
require.NoError(t, err)
teleportFeatures := pong.GetServerFeatures()
```

There are several environment variables defined:

* `TELEPORT_BINARY` - path to teleport binary (default: `teleport`).
* `TELEPORT_BINARY_TCTL` - path to tctl binary (default: `tctl`).
* `TELEPORT_LICENSE` - path to license file (default: `/var/lib/teleport/license.pem`).

Please, beware of `teleportFeatures` variable.

# Writing tests for Teleport Enterprise

The following snippet skips current test unless Teleport has Entrerprise features enabled. To enable Enterprise features, please use provided license file or put your license to `/var/lib/teleport/license.pem`.

```go
if !teleportFeatures.AdvancedAccessWorkflows {
	t.Skip("Doesn't work in OSS version")
}
```

# Creating default users and roles

Use `integration.Bootstrap` type to create default users and roles.

```go
var bootstrap integration.Bootstrap

conditions = types.RoleConditions{}
if teleportFeatures.AdvancedAccessWorkflows {
	// For Teleport Enterprise only
	conditions.ReviewRequests = &types.AccessReviewConditions{Roles: []string{"admin"}}
}
role, err = bootstrap.AddRole("default", types.RoleSpecV4{Allow: conditions})
require.NoError(t, err)

user, err = bootstrap.AddUserWithRoles("default", role.GetName())
require.NoError(t, err)

err = teleport.Bootstrap(ctx, auth, bootstrap.Resources())
require.NoError(t, err)
```

# Export identity file 

You can export identity file for a desired user using the following snippet:

```go
// auth is obtained on initialization
identityPath, err := teleport.Sign(ctx, auth, "email-plugin-user")
require.NoError(t, err)
```

# Running API calls with impersonation

`*integration.API` methods facilitate running Teleport API requests on behalf of entity owners. For example, if you want user "requestor" to create a new Access Request, you may do it using the following snippet:

```go
req, err := types.NewAccessRequest("u-u-id", "requestor", "admin")
require.NoError(t, err)
req.SetRequestReason("because of")
```

Using `*integration.API` it would look like this:

```go
err := api.CreateAccessRequest(ctx, req)
require.NoError(t, err)
```

If "requestor" won't have rights, this method will fail.

This is equivalent to, where client would be normal Teleport `api.Client` instance:

```go
// auth is obtained on initialization
client, err := integration.Client(ctx, auth, "requestor")
require.NoError(t, err)

client.CreateAccessRequest(ctx, req)
```

Feel free to add missing methods to `*integration.API`.

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

Please note that Teleport instance does not get restatrted between tests. Hence, *Teleport state does not get reset*. Otherwise, it will significantly slow tests down. 

So, in this example, if there are more than three messages generated, they will be processed by the other tests in a suite.  This could lead to unobvious random failures. You need to check that messages you have received belong to the test subject you are working with in this specific method.