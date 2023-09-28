# Access request plugins

The access request plugins in this directory allow Teleport users to integrate access request notifications and approval workflows with third party messaging, project management, and scheduling solutions. These plugins also serve as examples for building your own integration.
If you have a self-hosted Teleport deployment, you can find information for configuring these access
request plugins in [Just-in-Time Access Request Plugins](https://goteleport.com/docs/access-controls/access-request-plugins/).

For an overview of the complete workflow for access requests and how messaging, project management, and scheduling solutions integrate with Teleport, see the [Access Requests for Cloud Infrastructure](https://goteleport.com/blog/access-requests/) blog post.

## Access API

The Teleport Access API has been moved into the main Teleport repository.
You can import it from `github.com/gravitational/teleport/api`. To see examples of how to get started with the Teleport API, see the [go-client example](https://github.com/gravitational/teleport/tree/master/examples/go-client) or read the [API docs](https://goteleport.com/docs/api/introduction/).
For more specific examples of how to build a custom access request workflow with the Teleport API, see [How to Build an Access Request Plugin](https://goteleport.com/docs/api/access-plugin/).

## Existing plugin guides

The Teleport documentation includes access request plugins guides for integration
with the following solutions:

- [Discord](https://goteleport.com/docs/ver/15.x/access-controls/access-request-plugins/ssh-approval-discord/)
- [Email](https://goteleport.com/docs/ver/15.x/access-controls/access-request-plugins/ssh-approval-email/)
- [JIRA](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-jira/)
- [Mattermost](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-mattermost/)
- [Microsoft Teams](https://goteleport.com/docs/ver/15.x/access-controls/access-request-plugins/ssh-approval-msteams/)
- [PagerDuty](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-pagerduty/)
- [Slack](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-slack/)