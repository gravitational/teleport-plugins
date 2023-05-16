---
title: Terraform provider resources
description: Terraform provider resources reference
---

Supported resources:

- [teleport_app](#teleport_app)
- [teleport_auth_preference](#teleport_auth_preference)
- [teleport_bot](#teleport_bot)
- [teleport_cluster_networking_config](#teleport_cluster_networking_config)
- [teleport_database](#teleport_database)
- [teleport_github_connector](#teleport_github_connector)
- [teleport_login_rule](#teleport_login_rule)
- [teleport_oidc_connector](#teleport_oidc_connector)
- [teleport_provision_token](#teleport_provision_token)
- [teleport_role](#teleport_role)
- [teleport_saml_connector](#teleport_saml_connector)
- [teleport_session_recording_config](#teleport_session_recording_config)
- [teleport_trusted_cluster](#teleport_trusted_cluster)
- [teleport_user](#teleport_user)

## Provider configuration

Ensure your Terraform version is v(=terraform.version=) or higher.

Add the following configuration section to your `terraform` configuration block:

```
terraform {
  required_providers {
    teleport = {
      version = ">= (=teleport.version=)"
      source  = "terraform.releases.teleport.dev/gravitational/teleport"
    }
  }
}
```

The provider supports the following options:

| Name                   | Type   | Description                                                                    | Environment Variable               |
| ---------------------- | ------ | ------------------------------------------------------------------------------ | ---------------------------------- |
| `addr`                 | string | Teleport auth or proxy address in "host:port" format.                          | `TF_TELEPORT_ADDR`                 |
| `cert_path`            | string | Path to Teleport certificate file.                                             | `TF_TELEPORT_CERT`                 |
| `cert_base64`          | string | Teleport certificate as base64.                                                | `TF_TELEPORT_CERT_BASE64`          |
| `identity_file_path`   | string | Path to Teleport identity file.                                                | `TF_TELEPORT_IDENTITY_FILE_PATH`   |
| `identity_file_base64` | string | Teleport identity file as base64.                                              | `TF_TELEPORT_IDENTITY_FILE_BASE64` |
| `key_path`             | string | Path to Teleport key file.                                                     | `TF_TELEPORT_KEY`                  |
| `key_base64`           | string | Teleport key as base64.                                                        | `TF_TELEPORT_KEY_BASE64`           |
| `profile_dir`          | string | Teleport profile path.                                                         | `TF_TELEPORT_PROFILE_PATH`         |
| `profile_name`         | string | Teleport profile name.                                                         | `TF_TELEPORT_PROFILE_NAME`         |
| `root_ca_path`         | string | Path to Teleport CA file.                                                      | `TF_TELEPORT_ROOT_CA`              |
| `root_ca_base64`       | string | Teleport CA as base64.                                                         | `TF_TELEPORT_ROOT_CA_BASE64`       |
| `retry_base_duration`  | string | Base duration between retries. [Format](https://pkg.go.dev/time#ParseDuration) | `TF_TELEPORT_RETRY_BASE_DURATION`  |
| `retry_cap_duration`   | string | Max duration between retries. [Format](https://pkg.go.dev/time#ParseDuration)  | `TF_TELEPORT_RETRY_CAP_DURATION`   |
| `retry_max_tries`      | string | Max number of retries.                                                         | `TF_TELEPORT_RETRY_MAX_TRIES`      |

You need to specify at least one of:

- `cert_path`, `key_path`,`root_ca_path` and `addr` to connect using key files.
- `cert_base64`, `key_base64`,`root_ca_base64` and `addr` to connect using a base64-encoded key.
- `identity_file_path` or `identity_file_base64` and `addr` to connect using an identity file.
- `profile_name`, `profile_dir` (both can be empty) and `addr` to connect using current profile from `~/.tsh`

The `retry_*` values are used to retry the API calls to Teleport when the cache is stale.

If more than one are provided, they will be tried in the order above until one succeeds.

Example:

```
provider "teleport" {
  addr         = "localhost:3025"
  cert_path    = "tf.crt"
  key_path     = "tf.key"
  root_ca_path = "tf.ca"
}
```

## teleport_app

|   Name   |  Type  | Required |               Description                |
|----------|--------|----------|------------------------------------------|
| metadata | object |          | Metadata is the app resource metadata.   |
| spec     | object |          | Spec is the app resource spec.           |
| sub_kind | string |          | SubKind is an optional resource subkind. |
| version  | string |          | Version is the resource version.         |

### metadata

Metadata is the app resource metadata.

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is the app resource spec.

|         Name         |  Type  | Required |                               Description                                |
|----------------------|--------|----------|--------------------------------------------------------------------------|
| aws                  | object |          | AWS contains additional options for AWS applications.                    |
| cloud                | string |          | Cloud identifies the cloud instance the app represents.                  |
| dynamic_labels       | object |          | DynamicLabels are the app&#39;s command labels.                          |
| insecure_skip_verify | bool   |          | InsecureSkipVerify disables app&#39;s TLS certificate verification.      |
| public_addr          | string |          | PublicAddr is the public address the application is accessible at.       |
| rewrite              | object |          | Rewrite is a list of rewriting rules to apply to requests and responses. |
| uri                  | string |          | URI is the web app endpoint.                                             |

#### spec.aws

AWS contains additional options for AWS applications.

|    Name     |  Type  | Required |                               Description                               |
|-------------|--------|----------|-------------------------------------------------------------------------|
| external_id | string |          | ExternalID is the AWS External ID used when assuming roles in this app. |

#### spec.dynamic_labels

DynamicLabels are the app's command labels.

|  Name   |       Type       | Required |              Description              |
|---------|------------------|----------|---------------------------------------|
| command | array of strings |          | Command is a command to run           |
| period  | duration         |          | Period is a time between command runs |
| result  | string           |          | Result captures standard output       |

#### spec.rewrite

Rewrite is a list of rewriting rules to apply to requests and responses.

|   Name   |       Type       | Required |                                                                    Description                                                                    |
|----------|------------------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| headers  | object           |          | Headers is a list of headers to inject when passing the request over to the application.                                                          |
| redirect | array of strings |          | Redirect defines a list of hosts which will be rewritten to the public address of the application if they occur in the &#34;Location&#34; header. |

##### spec.rewrite.headers

Headers is a list of headers to inject when passing the request over to the application.

| Name  |  Type  | Required |           Description           |
|-------|--------|----------|---------------------------------|
| name  | string |          | Name is the http header name.   |
| value | string |          | Value is the http header value. |

Example:

```
# Teleport App

resource "teleport_app" "example" {
  metadata = {
    name = "example"
    description = "Test app"
    labels = {
        "teleport.dev/origin" = "dynamic" // This label is added on Teleport side by default
    }
  }

  spec = {
    uri = "localhost:3000"
  }
}
```

## teleport_auth_preference

|   Name   |  Type  | Required |                           Description                            |
|----------|--------|----------|------------------------------------------------------------------|
| metadata | object |          | Metadata is resource metadata                                    |
| spec     | object | *        | Spec is an AuthPreference specification                          |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources |
| version  | string |          | Version is a resource version                                    |

### metadata

Metadata is resource metadata

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is an AuthPreference specification

|          Name           |  Type  | Required |                                                              Description                                                               |
|-------------------------|--------|----------|----------------------------------------------------------------------------------------------------------------------------------------|
| allow_headless          | bool   |          |                                                                                                                                        |
| allow_local_auth        | bool   |          |                                                                                                                                        |
| allow_passwordless      | bool   |          |                                                                                                                                        |
| connector_name          | string |          | ConnectorName is the name of the OIDC or SAML connector. If this value is not set the first connector in the backend will be used.     |
| device_trust            | object |          | DeviceTrust holds settings related to trusted device verification. Requires Teleport Enterprise.                                       |
| disconnect_expired_cert | bool   |          |                                                                                                                                        |
| idp                     | object |          | IDP is a set of options related to accessing IdPs within Teleport. Requires Teleport Enterprise.                                       |
| locking_mode            | string |          | LockingMode is the cluster-wide locking mode default.                                                                                  |
| message_of_the_day      | string |          |                                                                                                                                        |
| require_session_mfa     | number |          | RequireMFAType is the type of MFA requirement enforced for this cluster: 0:Off, 1:Session, 2:SessionAndHardwareKey, 3:HardwareKeyTouch |
| second_factor           | string |          | SecondFactor is the type of second factor.                                                                                             |
| type                    | string |          | Type is the type of authentication.                                                                                                    |
| u2f                     | object |          | U2F are the settings for the U2F device.                                                                                               |
| webauthn                | object |          | Webauthn are the settings for server-side Web Authentication support.                                                                  |

#### spec.device_trust

DeviceTrust holds settings related to trusted device verification. Requires Teleport Enterprise.

|    Name     |  Type  | Required |                                                                                                                                                                                                                                             Description                                                                                                                                                                                                                                              |
|-------------|--------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| auto_enroll | bool   |          | Enable device auto-enroll. Auto-enroll lets any user issue a device enrollment token for a known device that is not already enrolled. `tsh` takes advantage of auto-enroll to automatically enroll devices on user login, when appropriate. The effective cluster Mode still applies: AutoEnroll=true is meaningless if Mode=&#34;off&#34;.                                                                                                                                                          |
| mode        | string |          | Mode of verification for trusted devices.  The following modes are supported:  - &#34;off&#34;: disables both device authentication and authorization. - &#34;optional&#34;: allows both device authentication and authorization, but doesn&#39;t enforce the presence of device extensions for sensitive endpoints. - &#34;required&#34;: enforces the presence of device extensions for sensitive endpoints.  Mode is always &#34;off&#34; for OSS. Defaults to &#34;optional&#34; for Enterprise. |

#### spec.idp

IDP is a set of options related to accessing IdPs within Teleport. Requires Teleport Enterprise.

| Name |  Type  | Required |                    Description                     |
|------|--------|----------|----------------------------------------------------|
| saml | object |          | SAML are options related to the Teleport SAML IdP. |

##### spec.idp.saml

SAML are options related to the Teleport SAML IdP.

|  Name   | Type | Required | Description |
|---------|------|----------|-------------|
| enabled | bool |          |             |

#### spec.u2f

U2F are the settings for the U2F device.

|          Name          |       Type       | Required |                                                                                                Description                                                                                                |
|------------------------|------------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| app_id                 | string           |          | AppID returns the application ID for universal second factor.                                                                                                                                             |
| device_attestation_cas | array of strings |          | DeviceAttestationCAs contains the trusted attestation CAs for U2F devices.                                                                                                                                |
| facets                 | array of strings |          | Facets returns the facets for universal second factor. Deprecated: Kept for backwards compatibility reasons, but Facets have no effect since Teleport v10, when Webauthn replaced the U2F implementation. |

#### spec.webauthn

Webauthn are the settings for server-side Web Authentication support.

|          Name           |       Type       | Required |                                                                                                                                                                                                                       Description                                                                                                                                                                                                                       |
|-------------------------|------------------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| attestation_allowed_cas | array of strings |          | Allow list of device attestation CAs in PEM format. If present, only devices whose attestation certificates match the certificates specified here may be registered (existing registrations are unchanged). If supplied in conjunction with AttestationDeniedCAs, then both conditions need to be true for registration to be allowed (the device MUST match an allowed CA and MUST NOT match a denied CA). By default all devices are allowed.         |
| attestation_denied_cas  | array of strings |          | Deny list of device attestation CAs in PEM format. If present, only devices whose attestation certificates don&#39;t match the certificates specified here may be registered (existing registrations are unchanged). If supplied in conjunction with AttestationAllowedCAs, then both conditions need to be true for registration to be allowed (the device MUST match an allowed CA and MUST NOT match a denied CA). By default no devices are denied. |
| rp_id                   | string           |          | RPID is the ID of the Relying Party. It should be set to the domain name of the Teleport installation.  IMPORTANT: RPID must never change in the lifetime of the cluster, because it&#39;s recorded in the registration data on the WebAuthn device. If the RPID changes, all existing WebAuthn key registrations will become invalid and all users who use WebAuthn as the second factor will need to re-register.                                     |

Example:

```
# AuthPreference resource

resource "teleport_auth_preference" "example" {
  metadata = {
    description = "Auth preference"
    labels = {
      "example" = "yes"
      "teleport.dev/origin" = "dynamic" // This label is added on Teleport side by default
    }
  }

  spec = {
    disconnect_expired_cert = true
  }
}

```

## teleport_bot

|   Name    |         Type         | Required |                                                                          Description                                                                          |
|-----------|----------------------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------|
| name      | string               | *        | The name of the bot, i.e. the unprefixed User name                                                                                                            |
| role_name | string               |          | The name of the generated bot role                                                                                                                            |
| roles     | array of strings     | *        | A list of roles the created bot should be allowed to assume via role impersonation.                                                                           |
| token_id  | string               | *        | The bot joining token. If unset, a new random token is created and its name returned, otherwise a preexisting Bot token may be provided for IAM/OIDC joining. |
| token_ttl | string               |          | The desired TTL for the token if one is created. If unset, a server default is used                                                                           |
| traits    | map of string arrays |          |                                                                                                                                                               |
| user_name | string               |          | The name of the generated bot user                                                                                                                            |

Example:

```
# Teleport Machine ID Bot creation example

locals {
  bot_name = "example"
}

resource "random_password" "bot_token" {
  length           = 32
  special          = false
}

resource "time_offset" "bot_example_token_expiry" {
  offset_hours = 1
}

resource "teleport_provision_token" "bot_example" {
  metadata = {
    expires = time_offset.bot_example_token_expiry.rfc3339
    description = "Bot join token for ${local.bot_name} generated by Terraform"

    name = random_password.bot_token.result
  }

  spec = {
    roles = ["Bot"]
    bot_name = local.bot_name
    join_method = "token"
  }
}

resource "teleport_bot" "example" {
  name = local.bot_name
  token_id = teleport_provision_token.bot_example.metadata.name
  roles = ["access"]
}

```

## teleport_cluster_networking_config

|   Name   |  Type  | Required |                           Description                            |
|----------|--------|----------|------------------------------------------------------------------|
| metadata | object |          | Metadata is resource metadata                                    |
| spec     | object |          | Spec is a ClusterNetworkingConfig specification                  |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources |
| version  | string |          | Version is a resource version                                    |

### metadata

Metadata is resource metadata

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is a ClusterNetworkingConfig specification

|          Name           |   Type   | Required |                                                                                               Description                                                                                               |
|-------------------------|----------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| client_idle_timeout     | duration |          | ClientIdleTimeout sets global cluster default setting for client idle timeouts.                                                                                                                         |
| idle_timeout_message    | string   |          | ClientIdleTimeoutMessage is the message sent to the user when a connection times out.                                                                                                                   |
| keep_alive_count_max    | number   |          | KeepAliveCountMax is the number of keep-alive messages that can be missed before the server disconnects the connection to the client.                                                                   |
| keep_alive_interval     | duration |          | KeepAliveInterval is the interval at which the server sends keep-alive messages to the client.                                                                                                          |
| proxy_listener_mode     | number   |          | ProxyListenerMode is proxy listener mode used by Teleport Proxies.                                                                                                                                      |
| proxy_ping_interval     | duration |          | ProxyPingInterval defines in which interval the TLS routing ping message should be sent. This is applicable only when using ping-wrapped connections, regular TLS routing connections are not affected. |
| routing_strategy        | number   |          | RoutingStrategy determines the strategy used to route to nodes.                                                                                                                                         |
| session_control_timeout | duration |          | SessionControlTimeout is the session control lease expiry and defines the upper limit of how long a node may be out of contact with the auth server before it begins terminating controlled sessions.   |
| tunnel_strategy         | object   |          | TunnelStrategyV1 determines the tunnel strategy used in the cluster.                                                                                                                                    |
| web_idle_timeout        | duration |          | WebIdleTimeout sets global cluster default setting for the web UI idle timeouts.                                                                                                                        |

#### spec.tunnel_strategy

TunnelStrategyV1 determines the tunnel strategy used in the cluster.

|     Name      |  Type  | Required | Description |
|---------------|--------|----------|-------------|
| agent_mesh    | object |          |             |
| proxy_peering | object |          |             |

##### spec.tunnel_strategy.agent_mesh



|  Name  | Type | Required |                          Description                          |
|--------|------|----------|---------------------------------------------------------------|
| active | bool |          | Automatically generated field preventing empty message errors |

##### spec.tunnel_strategy.proxy_peering



|          Name          |  Type  | Required | Description |
|------------------------|--------|----------|-------------|
| agent_connection_count | number |          |             |

Example:

```
# Teleport Cluster Networking config

resource "teleport_cluster_networking_config" "example" {
   metadata = {
    description = "Networking config"
    labels = {
      "example" = "yes"
      "teleport.dev/origin" = "dynamic" // This label is added on Teleport side by default
    }
  }

  spec = {
    client_idle_timeout = "1h"
  }
}
```

## teleport_database

|   Name   |  Type  | Required |               Description                |
|----------|--------|----------|------------------------------------------|
| metadata | object |          | Metadata is the database metadata.       |
| spec     | object |          | Spec is the database spec.               |
| sub_kind | string |          | SubKind is an optional resource subkind. |
| version  | string |          | Version is the resource version.         |

### metadata

Metadata is the database metadata.

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is the database spec.

|      Name      |  Type  | Required |                                                                 Description                                                                  |
|----------------|--------|----------|----------------------------------------------------------------------------------------------------------------------------------------------|
| ad             | object |          | AD is the Active Directory configuration for the database.                                                                                   |
| aws            | object |          | AWS contains AWS specific settings for RDS/Aurora/Redshift databases.                                                                        |
| azure          | object |          | Azure contains Azure specific database metadata.                                                                                             |
| ca_cert        | string |          | CACert is the PEM-encoded database CA certificate.  DEPRECATED: Moved to TLS.CACert. DELETE IN 10.0.                                         |
| dynamic_labels | object |          | DynamicLabels is the database dynamic labels.                                                                                                |
| gcp            | object |          | GCP contains parameters specific to GCP Cloud SQL databases.                                                                                 |
| mysql          | object |          | MySQL is an additional section with MySQL database options.                                                                                  |
| protocol       | string | *        | Protocol is the database protocol: postgres, mysql, mongodb, etc.                                                                            |
| tls            | object |          | TLS is the TLS configuration used when establishing connection to target database. Allows to provide custom CA cert or override server name. |
| uri            | string | *        | URI is the database connection endpoint.                                                                                                     |

#### spec.ad

AD is the Active Directory configuration for the database.

|     Name      |  Type  | Required |                                       Description                                       |
|---------------|--------|----------|-----------------------------------------------------------------------------------------|
| domain        | string |          | Domain is the Active Directory domain the database resides in.                          |
| kdc_host_name | string |          | KDCHostName is the host name for a KDC for x509 Authentication.                         |
| keytab_file   | string |          | KeytabFile is the path to the Kerberos keytab file.                                     |
| krb5_file     | string |          | Krb5File is the path to the Kerberos configuration file. Defaults to /etc/krb5.conf.    |
| ldap_cert     | string |          | LDAPCert is a certificate from Windows LDAP/AD, optional; only for x509 Authentication. |
| spn           | string |          | SPN is the service principal name for the database.                                     |

#### spec.aws

AWS contains AWS specific settings for RDS/Aurora/Redshift databases.

|        Name         |  Type  | Required |                                                                    Description                                                                     |
|---------------------|--------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| account_id          | string |          | AccountID is the AWS account ID this database belongs to.                                                                                          |
| assume_role_arn     | string |          | AssumeRoleARN is an optional AWS role ARN to assume when accessing a database. Set this field and ExternalID to enable access across AWS accounts. |
| elasticache         | object |          | ElastiCache contains AWS ElastiCache Redis specific metadata.                                                                                      |
| external_id         | string |          | ExternalID is an optional AWS external ID used to enable assuming an AWS role across accounts.                                                     |
| memorydb            | object |          | MemoryDB contains AWS MemoryDB specific metadata.                                                                                                  |
| rds                 | object |          | RDS contains RDS specific metadata.                                                                                                                |
| rdsproxy            | object |          | RDSProxy contains AWS Proxy specific metadata.                                                                                                     |
| redshift            | object |          | Redshift contains Redshift specific metadata.                                                                                                      |
| redshift_serverless | object |          | RedshiftServerless contains AWS Redshift Serverless specific metadata.                                                                             |
| region              | string |          | Region is a AWS cloud region.                                                                                                                      |
| secret_store        | object |          | SecretStore contains secret store configurations.                                                                                                  |

##### spec.aws.elasticache

ElastiCache contains AWS ElastiCache Redis specific metadata.

|            Name            |       Type       | Required |                                    Description                                     |
|----------------------------|------------------|----------|------------------------------------------------------------------------------------|
| endpoint_type              | string           |          | EndpointType is the type of the endpoint.                                          |
| replication_group_id       | string           |          | ReplicationGroupID is the Redis replication group ID.                              |
| transit_encryption_enabled | bool             |          | TransitEncryptionEnabled indicates whether in-transit encryption (TLS) is enabled. |
| user_group_ids             | array of strings |          | UserGroupIDs is a list of user group IDs.                                          |

##### spec.aws.memorydb

MemoryDB contains AWS MemoryDB specific metadata.

|     Name      |  Type  | Required |                             Description                              |
|---------------|--------|----------|----------------------------------------------------------------------|
| acl_name      | string |          | ACLName is the name of the ACL associated with the cluster.          |
| cluster_name  | string |          | ClusterName is the name of the MemoryDB cluster.                     |
| endpoint_type | string |          | EndpointType is the type of the endpoint.                            |
| tls_enabled   | bool   |          | TLSEnabled indicates whether in-transit encryption (TLS) is enabled. |

##### spec.aws.rds

RDS contains RDS specific metadata.

|    Name     |  Type  | Required |                            Description                            |
|-------------|--------|----------|-------------------------------------------------------------------|
| cluster_id  | string |          | ClusterID is the RDS cluster (Aurora) identifier.                 |
| iam_auth    | bool   |          | IAMAuth indicates whether database IAM authentication is enabled. |
| instance_id | string |          | InstanceID is the RDS instance identifier.                        |
| resource_id | string |          | ResourceID is the RDS instance resource identifier (db-xxx).      |

##### spec.aws.rdsproxy

RDSProxy contains AWS Proxy specific metadata.

|         Name         |  Type  | Required |                              Description                              |
|----------------------|--------|----------|-----------------------------------------------------------------------|
| custom_endpoint_name | string |          | CustomEndpointName is the identifier of an RDS Proxy custom endpoint. |
| name                 | string |          | Name is the identifier of an RDS Proxy.                               |
| resource_id          | string |          | ResourceID is the RDS instance resource identifier (prx-xxx).         |

##### spec.aws.redshift

Redshift contains Redshift specific metadata.

|    Name    |  Type  | Required |                  Description                  |
|------------|--------|----------|-----------------------------------------------|
| cluster_id | string |          | ClusterID is the Redshift cluster identifier. |

##### spec.aws.redshift_serverless

RedshiftServerless contains AWS Redshift Serverless specific metadata.

|      Name      |  Type  | Required |              Description               |
|----------------|--------|----------|----------------------------------------|
| endpoint_name  | string |          | EndpointName is the VPC endpoint name. |
| workgroup_id   | string |          | WorkgroupID is the workgroup ID.       |
| workgroup_name | string |          | WorkgroupName is the workgroup name.   |

##### spec.aws.secret_store

SecretStore contains secret store configurations.

|    Name    |  Type  | Required |                    Description                     |
|------------|--------|----------|----------------------------------------------------|
| key_prefix | string |          | KeyPrefix specifies the secret key prefix.         |
| kms_key_id | string |          | KMSKeyID specifies the AWS KMS key for encryption. |

#### spec.azure

Azure contains Azure specific database metadata.

|      Name       |  Type  | Required |                            Description                             |
|-----------------|--------|----------|--------------------------------------------------------------------|
| is_flexi_server | bool   |          | IsFlexiServer is true if the database is an Azure Flexible server. |
| name            | string |          | Name is the Azure database server name.                            |
| redis           | object |          | Redis contains Azure Cache for Redis specific database metadata.   |
| resource_id     | string |          | ResourceID is the Azure fully qualified ID for the resource.       |

##### spec.azure.redis

Redis contains Azure Cache for Redis specific database metadata.

|       Name        |  Type  | Required |                           Description                           |
|-------------------|--------|----------|-----------------------------------------------------------------|
| clustering_policy | string |          | ClusteringPolicy is the clustering policy for Redis Enterprise. |

#### spec.dynamic_labels

DynamicLabels is the database dynamic labels.

|  Name   |       Type       | Required |              Description              |
|---------|------------------|----------|---------------------------------------|
| command | array of strings |          | Command is a command to run           |
| period  | duration         |          | Period is a time between command runs |
| result  | string           |          | Result captures standard output       |

#### spec.gcp

GCP contains parameters specific to GCP Cloud SQL databases.

|    Name     |  Type  | Required |                            Description                             |
|-------------|--------|----------|--------------------------------------------------------------------|
| instance_id | string |          | InstanceID is the Cloud SQL instance ID.                           |
| project_id  | string |          | ProjectID is the GCP project ID the Cloud SQL instance resides in. |

#### spec.mysql

MySQL is an additional section with MySQL database options.

|      Name      |  Type  | Required |                                              Description                                              |
|----------------|--------|----------|-------------------------------------------------------------------------------------------------------|
| server_version | string |          | ServerVersion is the server version reported by DB proxy if the runtime information is not available. |

#### spec.tls

TLS is the TLS configuration used when establishing connection to target database. Allows to provide custom CA cert or override server name.

|    Name     |  Type  | Required |                                                            Description                                                             |
|-------------|--------|----------|------------------------------------------------------------------------------------------------------------------------------------|
| ca_cert     | string |          | CACert is an optional user provided CA certificate used for verifying database TLS connection.                                     |
| mode        | number |          | Mode is a TLS connection mode. See DatabaseTLSMode for details.                                                                    |
| server_name | string |          | ServerName allows to provide custom hostname. This value will override the servername/hostname on a certificate during validation. |

Example:

```
# Teleport Database

resource "teleport_database" "example" {
    metadata = {
        name = "example"
        description = "Test database"
        labels = {
            "teleport.dev/origin" = "dynamic" // This label is added on Teleport side by default
        }
    }

    spec = {
        protocol = "postgres"
        uri = "localhost"
    }
}
```

## teleport_github_connector

|   Name   |  Type  | Required |                            Description                            |
|----------|--------|----------|-------------------------------------------------------------------|
| metadata | object |          | Metadata holds resource metadata.                                 |
| spec     | object | *        | Spec is an Github connector specification.                        |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources. |
| version  | string |          | Version is a resource version.                                    |

### metadata

Metadata holds resource metadata.

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is an Github connector specification.

|       Name       |  Type  | Required |                                                             Description                                                             |
|------------------|--------|----------|-------------------------------------------------------------------------------------------------------------------------------------|
| api_endpoint_url | string |          | APIEndpointURL is the URL of the API endpoint of the Github instance this connector is for.                                         |
| client_id        | string | *        | ClientID is the Github OAuth app client ID.                                                                                         |
| client_secret    | string | *        | ClientSecret is the Github OAuth app client secret.                                                                                 |
| display          | string |          | Display is the connector display name.                                                                                              |
| endpoint_url     | string |          | EndpointURL is the URL of the GitHub instance this connector is for.                                                                |
| redirect_url     | string |          | RedirectURL is the authorization callback URL.                                                                                      |
| teams_to_logins  | object |          | TeamsToLogins maps Github team memberships onto allowed logins/roles.  DELETE IN 11.0.0 Deprecated: use GithubTeamsToRoles instead. |
| teams_to_roles   | object |          | TeamsToRoles maps Github team memberships onto allowed roles.                                                                       |

#### spec.teams_to_logins

TeamsToLogins maps Github team memberships onto allowed logins/roles.  DELETE IN 11.0.0 Deprecated: use GithubTeamsToRoles instead.

|       Name        |       Type       | Required |                                    Description                                    |
|-------------------|------------------|----------|-----------------------------------------------------------------------------------|
| kubernetes_groups | array of strings |          | KubeGroups is a list of allowed kubernetes groups for this org/team.              |
| kubernetes_users  | array of strings |          | KubeUsers is a list of allowed kubernetes users to impersonate for this org/team. |
| logins            | array of strings |          | Logins is a list of allowed logins for this org/team.                             |
| organization      | string           |          | Organization is a Github organization a user belongs to.                          |
| team              | string           |          | Team is a team within the organization a user belongs to.                         |

#### spec.teams_to_roles

TeamsToRoles maps Github team memberships onto allowed roles.

|     Name     |       Type       | Required |                        Description                        |
|--------------|------------------|----------|-----------------------------------------------------------|
| organization | string           |          | Organization is a Github organization a user belongs to.  |
| roles        | array of strings |          | Roles is a list of allowed logins for this org/team.      |
| team         | string           |          | Team is a team within the organization a user belongs to. |

Example:

```
# Terraform Github connector

variable "github_secret" {}

resource "teleport_github_connector" "github" {
  # This section tells Terraform that role example must be created before the GitHub connector
  depends_on = [
    teleport_role.example
  ]

  metadata = {
     name = "example"
     labels = {
       example = "yes"
     }
  }
  
  spec = {
    client_id = "client"
    client_secret = var.github_secret

    teams_to_roles = [{
       organization = "gravitational"
       team = "devs"
       roles = ["example"]
    }]
  }
}

```

## teleport_login_rule

|       Name        |  Type  | Required |                                                                            Description                                                                            |
|-------------------|--------|----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| metadata          | object |          | Metadata is resource metadata.                                                                                                                                    |
| priority          | number |          | Priority is the priority of the login rule relative to other login rules in the same cluster. Login rules with a lower numbered priority will be evaluated first. |
| traits_expression | string |          | TraitsExpression is a predicate expression which should return the desired traits for the user upon login.                                                        |
| traits_map        | object |          | TraitsMap is a map of trait keys to lists of predicate expressions which should evaluate to the desired values for that trait.                                    |
| version           | string |          | Version is the resource version.                                                                                                                                  |

### metadata

Metadata is resource metadata.

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         |          | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### traits_map

TraitsMap is a map of trait keys to lists of predicate expressions which should evaluate to the desired values for that trait.

|  Name  |       Type       | Required | Description |
|--------|------------------|----------|-------------|
| values | array of strings |          |             |

Example:

```
# Teleport Login Rule resource

resource "teleport_login_rule" "example" {
  metadata = {
    description = "Example Login Rule"
    labels = {
      "example" = "yes"
    }
  }

  version  = "v1"
  priority = 0
  traits_map = {
    "logins" = {
      values = [
        "external.logins",
        "external.username",
      ]
    }
    "groups" = {
      values = [
        "external.groups",
      ]
    }
  }
}

```

## teleport_oidc_connector

|   Name   |  Type  | Required |                            Description                            |
|----------|--------|----------|-------------------------------------------------------------------|
| metadata | object |          | Metadata holds resource metadata.                                 |
| spec     | object | *        | Spec is an OIDC connector specification.                          |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources. |
| version  | string |          | Version is a resource version.                                    |

### metadata

Metadata holds resource metadata.

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is an OIDC connector specification.

|            Name            |       Type       | Required |                                                                  Description                                                                  |
|----------------------------|------------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| acr_values                 | string           |          | ACR is an Authentication Context Class Reference value. The meaning of the ACR value is context-specific and varies for identity providers.   |
| allow_unverified_email     | bool             |          | AllowUnverifiedEmail tells the connector to accept OIDC users with unverified emails.                                                         |
| claims_to_roles            | object           |          | ClaimsToRoles specifies a dynamic mapping from claims to roles.                                                                               |
| client_id                  | string           |          | ClientID is the id of the authentication client (Teleport Auth server).                                                                       |
| client_secret              | string           |          | ClientSecret is used to authenticate the client.                                                                                              |
| display                    | string           |          | Display is the friendly name for this provider.                                                                                               |
| google_admin_email         | string           |          | GoogleAdminEmail is the email of a google admin to impersonate.                                                                               |
| google_service_account     | string           |          | GoogleServiceAccount is a string containing google service account credentials.                                                               |
| google_service_account_uri | string           |          | GoogleServiceAccountURI is a path to a google service account uri.                                                                            |
| issuer_url                 | string           |          | IssuerURL is the endpoint of the provider, e.g. https://accounts.google.com.                                                                  |
| prompt                     | string           |          | Prompt is an optional OIDC prompt. An empty string omits prompt. If not specified, it defaults to select_account for backwards compatibility. |
| provider                   | string           |          | Provider is the external identity provider.                                                                                                   |
| redirect_url               | array of strings |          |                                                                                                                                               |
| scope                      | array of strings |          | Scope specifies additional scopes set by provider.                                                                                            |
| username_claim             | string           |          | UsernameClaim specifies the name of the claim from the OIDC connector to be used as the user&#39;s username.                                  |

#### spec.claims_to_roles

ClaimsToRoles specifies a dynamic mapping from claims to roles.

| Name  |       Type       | Required |                    Description                     |
|-------|------------------|----------|----------------------------------------------------|
| claim | string           |          | Claim is a claim name.                             |
| roles | array of strings |          | Roles is a list of static teleport roles to match. |
| value | string           |          | Value is a claim value to match.                   |

Example:

```
# Teleport OIDC connector
# 
# Please note that OIDC connector will work in Enterprise version only. Check the setup docs:
# https://goteleport.com/docs/enterprise/sso/oidc/

variable "oidc_secret" {}

resource "teleport_oidc_connector" "example" {
  metadata = {
    name = "example"
    labels = {
      test = "yes"
    }
  }

  spec = {
    client_id = "client"
    client_secret = var.oidc_secret

    claims_to_roles = [{
      claim = "test"
      roles = ["terraform"]
    }]

    redirect_url = ["https://example.com/redirect"]
  }
}

```

## teleport_provision_token

|   Name   |  Type  | Required |                           Description                            |
|----------|--------|----------|------------------------------------------------------------------|
| metadata | object |          | Metadata is resource metadata                                    |
| spec     | object | *        | Spec is a provisioning token V2 spec                             |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources |
| version  | string |          | Version is version                                               |

### metadata

Metadata is resource metadata

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   | *        | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         |          | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is a provisioning token V2 spec

|              Name              |         Type         | Required |                                                                        Description                                                                         |
|--------------------------------|----------------------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| allow                          | object               |          | Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token.                                                         |
| aws_iid_ttl                    | duration             |          | AWSIIDTTL is the TTL to use for AWS EC2 Instance Identity Documents used to join the cluster with this token.                                              |
| azure                          | object               |          | Azure allows the configuration of options specific to the &#34;azure&#34; join method.                                                                     |
| bot_name                       | string               |          | BotName is the name of the bot this token grants access to, if any                                                                                         |
| circleci                       | object               |          | CircleCI allows the configuration of options specific to the &#34;circleci&#34; join method.                                                               |
| github                         | object               |          | GitHub allows the configuration of options specific to the &#34;github&#34; join method.                                                                   |
| gitlab                         | object               |          | GitLab allows the configuration of options specific to the &#34;gitlab&#34; join method.                                                                   |
| join_method                    | string               |          | JoinMethod is the joining method required in order to use this token. Supported joining methods include &#34;token&#34;, &#34;ec2&#34;, and &#34;iam&#34;. |
| kubernetes                     | object               |          | Kubernetes allows the configuration of options specific to the &#34;kubernetes&#34; join method.                                                           |
| roles                          | array of strings     | *        | Roles is a list of roles associated with the token, that will be converted to metadata in the SSH and X509 certificates issued to the user of the token    |
| suggested_agent_matcher_labels | map of string arrays |          |                                                                                                                                                            |
| suggested_labels               | map of string arrays |          |                                                                                                                                                            |

#### spec.allow

Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token.

|    Name     |       Type       | Required |                                                                  Description                                                                   |
|-------------|------------------|----------|------------------------------------------------------------------------------------------------------------------------------------------------|
| aws_account | string           |          | AWSAccount is the AWS account ID.                                                                                                              |
| aws_arn     | string           |          | AWSARN is used for the IAM join method, the AWS identity of joining nodes must match this ARN. Supports wildcards &#34;*&#34; and &#34;?&#34;. |
| aws_regions | array of strings |          | AWSRegions is used for the EC2 join method and is a list of AWS regions a node is allowed to join from.                                        |
| aws_role    | string           |          | AWSRole is used for the EC2 join method and is the the ARN of the AWS role that the auth server will assume in order to call the ec2 API.      |

#### spec.azure

Azure allows the configuration of options specific to the "azure" join method.

| Name  |  Type  | Required |                                          Description                                          |
|-------|--------|----------|-----------------------------------------------------------------------------------------------|
| allow | object |          | Allow is a list of Rules, nodes using this token must match one allow rule to use this token. |

##### spec.azure.allow

Allow is a list of Rules, nodes using this token must match one allow rule to use this token.

|      Name       |       Type       | Required |                                     Description                                     |
|-----------------|------------------|----------|-------------------------------------------------------------------------------------|
| resource_groups | array of strings |          | ResourceGroups is a list of Azure resource groups the node is allowed to join from. |
| subscription    | string           |          | Subscription is the Azure subscription.                                             |

#### spec.circleci

CircleCI allows the configuration of options specific to the "circleci" join method.

|      Name       |  Type  | Required |                                            Description                                             |
|-----------------|--------|----------|----------------------------------------------------------------------------------------------------|
| allow           | object |          | Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token. |
| organization_id | string |          |                                                                                                    |

##### spec.circleci.allow

Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token.

|    Name    |  Type  | Required | Description |
|------------|--------|----------|-------------|
| context_id | string |          |             |
| project_id | string |          |             |

#### spec.github

GitHub allows the configuration of options specific to the "github" join method.

|          Name          |  Type  | Required |                                                                                                                                                                                                                                             Description                                                                                                                                                                                                                                             |
|------------------------|--------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| allow                  | object |          | Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token.                                                                                                                                                                                                                                                                                                                                                                                                  |
| enterprise_server_host | string |          | EnterpriseServerHost allows joining from runners associated with a GitHub Enterprise Server instance. When unconfigured, tokens will be validated against github.com, but when configured to the host of a GHES instance, then the tokens will be validated against host.  This value should be the hostname of the GHES instance, and should not include the scheme or a path. The instance must be accessible over HTTPS at this hostname and the certificate must be trusted by the Auth Server. |

##### spec.github.allow

Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token.

|       Name       |  Type  | Required |                                                                        Description                                                                         |
|------------------|--------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| actor            | string |          | The personal account that initiated the workflow run.                                                                                                      |
| environment      | string |          | The name of the environment used by the job.                                                                                                               |
| ref              | string |          | The git ref that triggered the workflow run.                                                                                                               |
| ref_type         | string |          | The type of ref, for example: &#34;branch&#34;.                                                                                                            |
| repository       | string |          | The repository from where the workflow is running. This includes the name of the owner e.g `gravitational/teleport`                                        |
| repository_owner | string |          | The name of the organization in which the repository is stored.                                                                                            |
| sub              | string |          | Sub also known as Subject is a string that roughly uniquely identifies the workload. The format of this varies depending on the type of github action run. |
| workflow         | string |          | The name of the workflow.                                                                                                                                  |

#### spec.gitlab

GitLab allows the configuration of options specific to the "gitlab" join method.

|  Name  |  Type  | Required |                                                                             Description                                                                             |
|--------|--------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| allow  | object |          | Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token.                                                                  |
| domain | string |          | Domain is the domain of your GitLab instance. This will default to `gitlab.com` - but can be set to the domain of your self-hosted GitLab e.g `gitlab.example.com`. |

##### spec.gitlab.allow

Allow is a list of TokenRules, nodes using this token must match one allow rule to use this token.

|      Name       |  Type  | Required |                                                                                    Description                                                                                     |
|-----------------|--------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| environment     | string |          | Environment limits access by the environment the job deploys to (if one is associated)                                                                                             |
| namespace_path  | string |          | NamespacePath is used to limit access to jobs in a group or user&#39;s projects. Example: `mygroup`                                                                                |
| pipeline_source | string |          | PipelineSource limits access by the job pipeline source type. https://docs.gitlab.com/ee/ci/jobs/job_control.html#common-if-clauses-for-rules Example: `web`                       |
| project_path    | string |          | ProjectPath is used to limit access to jobs belonging to an individual project. Example: `mygroup/myproject`                                                                       |
| ref             | string |          | Ref allows access to be limited to jobs triggered by a specific git ref. Ensure this is used in combination with ref_type.                                                         |
| ref_type        | string |          | RefType allows access to be limited to jobs triggered by a specific git ref type. Example: `branch` or `tag`                                                                       |
| sub             | string |          | Sub roughly uniquely identifies the workload. Example: `project_path:mygroup/my-project:ref_type:branch:ref:main` project_path:{group}/{project}:ref_type:{type}:ref:{branch_name} |

#### spec.kubernetes

Kubernetes allows the configuration of options specific to the "kubernetes" join method.

| Name  |  Type  | Required |                                          Description                                          |
|-------|--------|----------|-----------------------------------------------------------------------------------------------|
| allow | object |          | Allow is a list of Rules, nodes using this token must match one allow rule to use this token. |

##### spec.kubernetes.allow

Allow is a list of Rules, nodes using this token must match one allow rule to use this token.

|      Name       |  Type  | Required |                                                         Description                                                         |
|-----------------|--------|----------|-----------------------------------------------------------------------------------------------------------------------------|
| service_account | string |          | ServiceAccount is the namespaced name of the Kubernetes service account. Its format is &#34;namespace:service-account&#34;. |

Example:

```
# Teleport Provision Token resource

resource "teleport_provision_token" "example" {
  metadata = {
    expires = "2022-10-12T07:20:51Z"
    description = "Example token"

    labels = {
      example = "yes" 
      "teleport.dev/origin" = "dynamic" // This label is added on Teleport side by default
    }
  }

  spec = {
    roles = ["Node", "Auth"]
  }
}

```

## teleport_role

|   Name   |  Type  | Required |                           Description                            |
|----------|--------|----------|------------------------------------------------------------------|
| metadata | object |          | Metadata is resource metadata                                    |
| spec     | object |          | Spec is a role specification                                     |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources |
| version  | string |          | Version is version                                               |

### metadata

Metadata is resource metadata

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is a role specification

|  Name   |  Type  | Required |                                       Description                                       |
|---------|--------|----------|-----------------------------------------------------------------------------------------|
| allow   | object |          | Allow is the set of conditions evaluated to grant access.                               |
| deny    | object |          | Deny is the set of conditions evaluated to deny access. Deny takes priority over allow. |
| options | object |          | Options is for OpenSSH options like agent forwarding.                                   |

#### spec.allow

Allow is the set of conditions evaluated to grant access.

|          Name          |         Type         | Required |                                                           Description                                                           |
|------------------------|----------------------|----------|---------------------------------------------------------------------------------------------------------------------------------|
| app_labels             | map of string arrays |          |                                                                                                                                 |
| aws_role_arns          | array of strings     |          | AWSRoleARNs is a list of AWS role ARNs this role is allowed to assume.                                                          |
| azure_identities       | array of strings     |          | AzureIdentities is a list of Azure identities this role is allowed to assume.                                                   |
| cluster_labels         | map of string arrays |          |                                                                                                                                 |
| db_labels              | map of string arrays |          |                                                                                                                                 |
| db_names               | array of strings     |          | DatabaseNames is a list of database names this role is allowed to connect to.                                                   |
| db_service_labels      | map of string arrays |          |                                                                                                                                 |
| db_users               | array of strings     |          | DatabaseUsers is a list of databases users this role is allowed to connect as.                                                  |
| desktop_groups         | array of strings     |          | DesktopGroups is a list of groups for created desktop users to be added to                                                      |
| gcp_service_accounts   | array of strings     |          | GCPServiceAccounts is a list of GCP service accounts this role is allowed to assume.                                            |
| group_labels           | map of string arrays |          |                                                                                                                                 |
| host_groups            | array of strings     |          | HostGroups is a list of groups for created users to be added to                                                                 |
| host_sudoers           | array of strings     |          | HostSudoers is a list of entries to include in a users sudoer file                                                              |
| impersonate            | object               |          | Impersonate specifies what users and roles this role is allowed to impersonate by issuing certificates or other possible means. |
| join_sessions          | object               |          | JoinSessions specifies policies to allow users to join other sessions.                                                          |
| kubernetes_groups      | array of strings     |          | KubeGroups is a list of kubernetes groups                                                                                       |
| kubernetes_labels      | map of string arrays |          |                                                                                                                                 |
| kubernetes_resources   | object               |          | KubernetesResources is the Kubernetes Resources this Role grants access to.                                                     |
| kubernetes_users       | array of strings     |          | KubeUsers is an optional kubernetes users to impersonate                                                                        |
| logins                 | array of strings     |          | Logins is a list of *nix system logins.                                                                                         |
| node_labels            | map of string arrays |          |                                                                                                                                 |
| request                | object               |          |                                                                                                                                 |
| require_session_join   | object               |          | RequireSessionJoin specifies policies for required users to start a session.                                                    |
| review_requests        | object               |          | ReviewRequests defines conditions for submitting access reviews.                                                                |
| rules                  | object               |          | Rules is a list of rules and their access levels. Rules are a high level construct used for access control.                     |
| windows_desktop_labels | map of string arrays |          |                                                                                                                                 |
| windows_desktop_logins | array of strings     |          | WindowsDesktopLogins is a list of desktop login names allowed/denied for Windows desktops.                                      |

##### spec.allow.impersonate

Impersonate specifies what users and roles this role is allowed to impersonate by issuing certificates or other possible means.

| Name  |       Type       | Required |                                                  Description                                                   |
|-------|------------------|----------|----------------------------------------------------------------------------------------------------------------|
| roles | array of strings |          | Roles is a list of resources this role is allowed to impersonate                                               |
| users | array of strings |          | Users is a list of resources this role is allowed to impersonate, could be an empty list or a Wildcard pattern |
| where | string           |          | Where specifies optional advanced matcher                                                                      |

##### spec.allow.join_sessions

JoinSessions specifies policies to allow users to join other sessions.

| Name  |       Type       | Required |                           Description                           |
|-------|------------------|----------|-----------------------------------------------------------------|
| kinds | array of strings |          | Kinds are the session kinds this policy applies to.             |
| modes | array of strings |          | Modes is a list of permitted participant modes for this policy. |
| name  | string           |          | Name is the name of the policy.                                 |
| roles | array of strings |          | Roles is a list of roles that you can join the session of.      |

##### spec.allow.kubernetes_resources

KubernetesResources is the Kubernetes Resources this Role grants access to.

|   Name    |  Type  | Required |                                         Description                                         |
|-----------|--------|----------|---------------------------------------------------------------------------------------------|
| kind      | string |          | Kind specifies the Kubernetes Resource type. At the moment only &#34;pod&#34; is supported. |
| name      | string |          | Name is the resource name. It supports wildcards.                                           |
| namespace | string |          | Namespace is the resource namespace. It supports wildcards.                                 |

##### spec.allow.request



|        Name         |         Type         | Required |                                                                                                                    Description                                                                                                                    |
|---------------------|----------------------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| annotations         | map of string arrays |          |                                                                                                                                                                                                                                                   |
| claims_to_roles     | object               |          | ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.                                                                                                                                                                         |
| roles               | array of strings     |          | Roles is the name of roles which will match the request rule.                                                                                                                                                                                     |
| search_as_roles     | array of strings     |          | SearchAsRoles is a list of extra roles which should apply to a user while they are searching for resources as part of a Resource Access Request, and defines the underlying roles which will be requested as part of any Resource Access Request. |
| suggested_reviewers | array of strings     |          | SuggestedReviewers is a list of reviewer suggestions.  These can be teleport usernames, but that is not a requirement.                                                                                                                            |
| thresholds          | object               |          | Thresholds is a list of thresholds, one of which must be met in order for reviews to trigger a state-transition.  If no thresholds are provided, a default threshold of 1 for approval and denial is used.                                        |

###### spec.allow.request.claims_to_roles

ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.

| Name  |       Type       | Required |                    Description                     |
|-------|------------------|----------|----------------------------------------------------|
| claim | string           |          | Claim is a claim name.                             |
| roles | array of strings |          | Roles is a list of static teleport roles to match. |
| value | string           |          | Value is a claim value to match.                   |

###### spec.allow.request.thresholds

Thresholds is a list of thresholds, one of which must be met in order for reviews to trigger a state-transition.  If no thresholds are provided, a default threshold of 1 for approval and denial is used.

|  Name   |  Type  | Required |                                         Description                                          |
|---------|--------|----------|----------------------------------------------------------------------------------------------|
| approve | number |          | Approve is the number of matching approvals needed for state-transition.                     |
| deny    | number |          | Deny is the number of denials needed for state-transition.                                   |
| filter  | string |          | Filter is an optional predicate used to determine which reviews count toward this threshold. |
| name    | string |          | Name is the optional human-readable name of the threshold.                                   |

##### spec.allow.require_session_join

RequireSessionJoin specifies policies for required users to start a session.

|   Name   |       Type       | Required |                                             Description                                             |
|----------|------------------|----------|-----------------------------------------------------------------------------------------------------|
| count    | number           |          | Count is the amount of people that need to be matched for this policy to be fulfilled.              |
| filter   | string           |          | Filter is a predicate that determines what users count towards this policy.                         |
| kinds    | array of strings |          | Kinds are the session kinds this policy applies to.                                                 |
| modes    | array of strings |          | Modes is the list of modes that may be used to fulfill this policy.                                 |
| name     | string           |          | Name is the name of the policy.                                                                     |
| on_leave | string           |          | OnLeave is the behaviour that&#39;s used when the policy is no longer fulfilled for a live session. |

##### spec.allow.review_requests

ReviewRequests defines conditions for submitting access reviews.

|       Name       |       Type       | Required |                                                                                                      Description                                                                                                      |
|------------------|------------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| claims_to_roles  | object           |          | ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.                                                                                                                                             |
| preview_as_roles | array of strings |          | PreviewAsRoles is a list of extra roles which should apply to a reviewer while they are viewing a Resource Access Request for the purposes of viewing details such as the hostname and labels of requested resources. |
| roles            | array of strings |          | Roles is the name of roles which may be reviewed.                                                                                                                                                                     |
| where            | string           |          | Where is an optional predicate which further limits which requests are reviewable.                                                                                                                                    |

###### spec.allow.review_requests.claims_to_roles

ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.

| Name  |       Type       | Required |                    Description                     |
|-------|------------------|----------|----------------------------------------------------|
| claim | string           |          | Claim is a claim name.                             |
| roles | array of strings |          | Roles is a list of static teleport roles to match. |
| value | string           |          | Value is a claim value to match.                   |

##### spec.allow.rules

Rules is a list of rules and their access levels. Rules are a high level construct used for access control.

|   Name    |       Type       | Required |                           Description                           |
|-----------|------------------|----------|-----------------------------------------------------------------|
| actions   | array of strings |          | Actions specifies optional actions taken when this rule matches |
| resources | array of strings |          | Resources is a list of resources                                |
| verbs     | array of strings |          | Verbs is a list of verbs                                        |
| where     | string           |          | Where specifies optional advanced matcher                       |

#### spec.deny

Deny is the set of conditions evaluated to deny access. Deny takes priority over allow.

|          Name          |         Type         | Required |                                                           Description                                                           |
|------------------------|----------------------|----------|---------------------------------------------------------------------------------------------------------------------------------|
| app_labels             | map of string arrays |          |                                                                                                                                 |
| aws_role_arns          | array of strings     |          | AWSRoleARNs is a list of AWS role ARNs this role is allowed to assume.                                                          |
| azure_identities       | array of strings     |          | AzureIdentities is a list of Azure identities this role is allowed to assume.                                                   |
| cluster_labels         | map of string arrays |          |                                                                                                                                 |
| db_labels              | map of string arrays |          |                                                                                                                                 |
| db_names               | array of strings     |          | DatabaseNames is a list of database names this role is allowed to connect to.                                                   |
| db_service_labels      | map of string arrays |          |                                                                                                                                 |
| db_users               | array of strings     |          | DatabaseUsers is a list of databases users this role is allowed to connect as.                                                  |
| desktop_groups         | array of strings     |          | DesktopGroups is a list of groups for created desktop users to be added to                                                      |
| gcp_service_accounts   | array of strings     |          | GCPServiceAccounts is a list of GCP service accounts this role is allowed to assume.                                            |
| group_labels           | map of string arrays |          |                                                                                                                                 |
| host_groups            | array of strings     |          | HostGroups is a list of groups for created users to be added to                                                                 |
| host_sudoers           | array of strings     |          | HostSudoers is a list of entries to include in a users sudoer file                                                              |
| impersonate            | object               |          | Impersonate specifies what users and roles this role is allowed to impersonate by issuing certificates or other possible means. |
| join_sessions          | object               |          | JoinSessions specifies policies to allow users to join other sessions.                                                          |
| kubernetes_groups      | array of strings     |          | KubeGroups is a list of kubernetes groups                                                                                       |
| kubernetes_labels      | map of string arrays |          |                                                                                                                                 |
| kubernetes_resources   | object               |          | KubernetesResources is the Kubernetes Resources this Role grants access to.                                                     |
| kubernetes_users       | array of strings     |          | KubeUsers is an optional kubernetes users to impersonate                                                                        |
| logins                 | array of strings     |          | Logins is a list of *nix system logins.                                                                                         |
| node_labels            | map of string arrays |          |                                                                                                                                 |
| request                | object               |          |                                                                                                                                 |
| require_session_join   | object               |          | RequireSessionJoin specifies policies for required users to start a session.                                                    |
| review_requests        | object               |          | ReviewRequests defines conditions for submitting access reviews.                                                                |
| rules                  | object               |          | Rules is a list of rules and their access levels. Rules are a high level construct used for access control.                     |
| windows_desktop_labels | map of string arrays |          |                                                                                                                                 |
| windows_desktop_logins | array of strings     |          | WindowsDesktopLogins is a list of desktop login names allowed/denied for Windows desktops.                                      |

##### spec.deny.impersonate

Impersonate specifies what users and roles this role is allowed to impersonate by issuing certificates or other possible means.

| Name  |       Type       | Required |                                                  Description                                                   |
|-------|------------------|----------|----------------------------------------------------------------------------------------------------------------|
| roles | array of strings |          | Roles is a list of resources this role is allowed to impersonate                                               |
| users | array of strings |          | Users is a list of resources this role is allowed to impersonate, could be an empty list or a Wildcard pattern |
| where | string           |          | Where specifies optional advanced matcher                                                                      |

##### spec.deny.join_sessions

JoinSessions specifies policies to allow users to join other sessions.

| Name  |       Type       | Required |                           Description                           |
|-------|------------------|----------|-----------------------------------------------------------------|
| kinds | array of strings |          | Kinds are the session kinds this policy applies to.             |
| modes | array of strings |          | Modes is a list of permitted participant modes for this policy. |
| name  | string           |          | Name is the name of the policy.                                 |
| roles | array of strings |          | Roles is a list of roles that you can join the session of.      |

##### spec.deny.kubernetes_resources

KubernetesResources is the Kubernetes Resources this Role grants access to.

|   Name    |  Type  | Required |                                         Description                                         |
|-----------|--------|----------|---------------------------------------------------------------------------------------------|
| kind      | string |          | Kind specifies the Kubernetes Resource type. At the moment only &#34;pod&#34; is supported. |
| name      | string |          | Name is the resource name. It supports wildcards.                                           |
| namespace | string |          | Namespace is the resource namespace. It supports wildcards.                                 |

##### spec.deny.request



|        Name         |         Type         | Required |                                                                                                                    Description                                                                                                                    |
|---------------------|----------------------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| annotations         | map of string arrays |          |                                                                                                                                                                                                                                                   |
| claims_to_roles     | object               |          | ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.                                                                                                                                                                         |
| roles               | array of strings     |          | Roles is the name of roles which will match the request rule.                                                                                                                                                                                     |
| search_as_roles     | array of strings     |          | SearchAsRoles is a list of extra roles which should apply to a user while they are searching for resources as part of a Resource Access Request, and defines the underlying roles which will be requested as part of any Resource Access Request. |
| suggested_reviewers | array of strings     |          | SuggestedReviewers is a list of reviewer suggestions.  These can be teleport usernames, but that is not a requirement.                                                                                                                            |
| thresholds          | object               |          | Thresholds is a list of thresholds, one of which must be met in order for reviews to trigger a state-transition.  If no thresholds are provided, a default threshold of 1 for approval and denial is used.                                        |

###### spec.deny.request.claims_to_roles

ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.

| Name  |       Type       | Required |                    Description                     |
|-------|------------------|----------|----------------------------------------------------|
| claim | string           |          | Claim is a claim name.                             |
| roles | array of strings |          | Roles is a list of static teleport roles to match. |
| value | string           |          | Value is a claim value to match.                   |

###### spec.deny.request.thresholds

Thresholds is a list of thresholds, one of which must be met in order for reviews to trigger a state-transition.  If no thresholds are provided, a default threshold of 1 for approval and denial is used.

|  Name   |  Type  | Required |                                         Description                                          |
|---------|--------|----------|----------------------------------------------------------------------------------------------|
| approve | number |          | Approve is the number of matching approvals needed for state-transition.                     |
| deny    | number |          | Deny is the number of denials needed for state-transition.                                   |
| filter  | string |          | Filter is an optional predicate used to determine which reviews count toward this threshold. |
| name    | string |          | Name is the optional human-readable name of the threshold.                                   |

##### spec.deny.require_session_join

RequireSessionJoin specifies policies for required users to start a session.

|   Name   |       Type       | Required |                                             Description                                             |
|----------|------------------|----------|-----------------------------------------------------------------------------------------------------|
| count    | number           |          | Count is the amount of people that need to be matched for this policy to be fulfilled.              |
| filter   | string           |          | Filter is a predicate that determines what users count towards this policy.                         |
| kinds    | array of strings |          | Kinds are the session kinds this policy applies to.                                                 |
| modes    | array of strings |          | Modes is the list of modes that may be used to fulfill this policy.                                 |
| name     | string           |          | Name is the name of the policy.                                                                     |
| on_leave | string           |          | OnLeave is the behaviour that&#39;s used when the policy is no longer fulfilled for a live session. |

##### spec.deny.review_requests

ReviewRequests defines conditions for submitting access reviews.

|       Name       |       Type       | Required |                                                                                                      Description                                                                                                      |
|------------------|------------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| claims_to_roles  | object           |          | ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.                                                                                                                                             |
| preview_as_roles | array of strings |          | PreviewAsRoles is a list of extra roles which should apply to a reviewer while they are viewing a Resource Access Request for the purposes of viewing details such as the hostname and labels of requested resources. |
| roles            | array of strings |          | Roles is the name of roles which may be reviewed.                                                                                                                                                                     |
| where            | string           |          | Where is an optional predicate which further limits which requests are reviewable.                                                                                                                                    |

###### spec.deny.review_requests.claims_to_roles

ClaimsToRoles specifies a mapping from claims (traits) to teleport roles.

| Name  |       Type       | Required |                    Description                     |
|-------|------------------|----------|----------------------------------------------------|
| claim | string           |          | Claim is a claim name.                             |
| roles | array of strings |          | Roles is a list of static teleport roles to match. |
| value | string           |          | Value is a claim value to match.                   |

##### spec.deny.rules

Rules is a list of rules and their access levels. Rules are a high level construct used for access control.

|   Name    |       Type       | Required |                           Description                           |
|-----------|------------------|----------|-----------------------------------------------------------------|
| actions   | array of strings |          | Actions specifies optional actions taken when this rule matches |
| resources | array of strings |          | Resources is a list of resources                                |
| verbs     | array of strings |          | Verbs is a list of verbs                                        |
| where     | string           |          | Where specifies optional advanced matcher                       |

#### spec.options

Options is for OpenSSH options like agent forwarding.

|            Name            |       Type       | Required |                                                                                Description                                                                                 |
|----------------------------|------------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| cert_extensions            | object           |          | CertExtensions specifies the key/values                                                                                                                                    |
| cert_format                | string           |          | CertificateFormat defines the format of the user certificate to allow compatibility with older versions of OpenSSH.                                                        |
| client_idle_timeout        | duration         |          | ClientIdleTimeout sets disconnect clients on idle timeout behavior, if set to 0 means do not disconnect, otherwise is set to the idle duration.                            |
| create_desktop_user        | bool             |          |                                                                                                                                                                            |
| create_host_user           | bool             |          |                                                                                                                                                                            |
| desktop_clipboard          | bool             |          |                                                                                                                                                                            |
| desktop_directory_sharing  | bool             |          |                                                                                                                                                                            |
| device_trust_mode          | string           |          | DeviceTrustMode is the device authorization mode used for the resources associated with the role. See DeviceTrust.Mode. Reserved for future use, not yet used by Teleport. |
| disconnect_expired_cert    | bool             |          | DisconnectExpiredCert sets disconnect clients on expired certificates.                                                                                                     |
| enhanced_recording         | array of strings |          | BPF defines what events to record for the BPF-based session recorder.                                                                                                      |
| forward_agent              | bool             |          | ForwardAgent is SSH agent forwarding.                                                                                                                                      |
| idp                        | object           |          | IDP is a set of options related to accessing IdPs within Teleport. Requires Teleport Enterprise.                                                                           |
| lock                       | string           |          | Lock specifies the locking mode (strict|best_effort) to be applied with the role.                                                                                          |
| max_connections            | number           |          | MaxConnections defines the maximum number of concurrent connections a user may hold.                                                                                       |
| max_kubernetes_connections | number           |          | MaxKubernetesConnections defines the maximum number of concurrent Kubernetes sessions a user may hold.                                                                     |
| max_session_ttl            | duration         |          | MaxSessionTTL defines how long a SSH session can last for.                                                                                                                 |
| max_sessions               | number           |          | MaxSessions defines the maximum number of concurrent sessions per connection.                                                                                              |
| permit_x11_forwarding      | bool             |          | PermitX11Forwarding authorizes use of X11 forwarding.                                                                                                                      |
| pin_source_ip              | bool             |          | PinSourceIP forces the same client IP for certificate generation and usage                                                                                                 |
| port_forwarding            | bool             |          |                                                                                                                                                                            |
| record_session             | object           |          | RecordDesktopSession indicates whether desktop access sessions should be recorded. It defaults to true unless explicitly set to false.                                     |
| request_access             | string           |          | RequestAccess defines the access request strategy (optional|note|always) where optional is the default.                                                                    |
| request_prompt             | string           |          | RequestPrompt is an optional message which tells users what they aught to                                                                                                  |
| require_session_mfa        | number           |          | RequireMFAType is the type of MFA requirement enforced for this role: 0:Off, 1:Session, 2:SessionAndHardwareKey, 3:HardwareKeyTouch                                        |
| ssh_file_copy              | bool             |          |                                                                                                                                                                            |

##### spec.options.cert_extensions

CertExtensions specifies the key/values

| Name  |  Type  | Required |                                       Description                                        |
|-------|--------|----------|------------------------------------------------------------------------------------------|
| mode  | number |          | Mode is the type of extension to be used -- currently critical-option is not supported   |
| name  | string |          | Name specifies the key to be used in the cert extension.                                 |
| type  | number |          | Type represents the certificate type being extended, only ssh is supported at this time. |
| value | string |          | Value specifies the value to be used in the cert extension.                              |

##### spec.options.idp

IDP is a set of options related to accessing IdPs within Teleport. Requires Teleport Enterprise.

| Name |  Type  | Required |                    Description                     |
|------|--------|----------|----------------------------------------------------|
| saml | object |          | SAML are options related to the Teleport SAML IdP. |

###### spec.options.idp.saml

SAML are options related to the Teleport SAML IdP.

|  Name   | Type | Required | Description |
|---------|------|----------|-------------|
| enabled | bool |          |             |

##### spec.options.record_session

RecordDesktopSession indicates whether desktop access sessions should be recorded. It defaults to true unless explicitly set to false.

|  Name   |  Type  | Required |                      Description                      |
|---------|--------|----------|-------------------------------------------------------|
| default | string |          | Default indicates the default value for the services. |
| desktop | bool   |          |                                                       |
| ssh     | string |          | SSH indicates the session mode used on SSH sessions.  |

Example:

```
# Teleport Role resource

resource "teleport_role" "example" {
  metadata = {
    name        = "example"
    description = "Example Teleport Role"
    expires     = "2022-10-12T07:20:51Z"
    labels = {
      example  = "yes"      
    }
  }
  
  spec = {
    options = {
      forward_agent           = false
      max_session_ttl         = "7m"
      port_forwarding         = false
      client_idle_timeout     = "1h"
      disconnect_expired_cert = true
      permit_x11_forwarding   = false
      request_access          = "denied"
    }

    allow = {
      logins = ["example"]

      rules = [{
        resources = ["user", "role"]
        verbs = ["list"]
      }]

      request = {
        roles = ["example"]
        claims_to_roles = [{
          claim = "example"
          value = "example"
          roles = ["example"]
        }]
      }

      node_labels = {
        example = ["yes"]
      }
    }

    deny = {
      logins = ["anonymous"]
    }
  }
}
```

## teleport_saml_connector

|   Name   |  Type  | Required |                            Description                            |
|----------|--------|----------|-------------------------------------------------------------------|
| metadata | object |          | Metadata holds resource metadata.                                 |
| spec     | object | *        | Spec is an SAML connector specification.                          |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources. |
| version  | string |          | Version is a resource version.                                    |

### metadata

Metadata holds resource metadata.

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is an SAML connector specification.

|          Name           |  Type  | Required |                                                                            Description                                                                            |
|-------------------------|--------|----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| acs                     | string | *        | AssertionConsumerService is a URL for assertion consumer service on the service provider (Teleport&#39;s side).                                                   |
| allow_idp_initiated     | bool   |          | AllowIDPInitiated is a flag that indicates if the connector can be used for IdP-initiated logins.                                                                 |
| assertion_key_pair      | object |          | EncryptionKeyPair is a key pair used for decrypting SAML assertions.                                                                                              |
| attributes_to_roles     | object | *        | AttributesToRoles is a list of mappings of attribute statements to roles.                                                                                         |
| audience                | string |          | Audience uniquely identifies our service provider.                                                                                                                |
| cert                    | string |          | Cert is the identity provider certificate PEM. IDP signs &lt;Response&gt; responses using this certificate.                                                       |
| display                 | string |          | Display controls how this connector is displayed.                                                                                                                 |
| entity_descriptor       | string |          | EntityDescriptor is XML with descriptor. It can be used to supply configuration parameters in one XML file rather than supplying them in the individual elements. |
| entity_descriptor_url   | string |          | EntityDescriptorURL is a URL that supplies a configuration XML.                                                                                                   |
| issuer                  | string |          | Issuer is the identity provider issuer.                                                                                                                           |
| provider                | string |          | Provider is the external identity provider.                                                                                                                       |
| service_provider_issuer | string |          | ServiceProviderIssuer is the issuer of the service provider (Teleport).                                                                                           |
| signing_key_pair        | object |          | SigningKeyPair is an x509 key pair used to sign AuthnRequest.                                                                                                     |
| sso                     | string |          | SSO is the URL of the identity provider&#39;s SSO service.                                                                                                        |

#### spec.assertion_key_pair

EncryptionKeyPair is a key pair used for decrypting SAML assertions.

|    Name     |  Type  | Required |                  Description                  |
|-------------|--------|----------|-----------------------------------------------|
| cert        | string |          | Cert is a PEM-encoded x509 certificate.       |
| private_key | string |          | PrivateKey is a PEM encoded x509 private key. |

#### spec.attributes_to_roles

AttributesToRoles is a list of mappings of attribute statements to roles.

| Name  |       Type       | Required |                     Description                     |
|-------|------------------|----------|-----------------------------------------------------|
| name  | string           |          | Name is an attribute statement name.                |
| roles | array of strings |          | Roles is a list of static teleport roles to map to. |
| value | string           |          | Value is an attribute statement value to match.     |

#### spec.signing_key_pair

SigningKeyPair is an x509 key pair used to sign AuthnRequest.

|    Name     |  Type  | Required |                  Description                  |
|-------------|--------|----------|-----------------------------------------------|
| cert        | string |          | Cert is a PEM-encoded x509 certificate.       |
| private_key | string |          | PrivateKey is a PEM encoded x509 private key. |

Example:

```
# Teleport SAML connector
# 
# Please note that SAML connector will work in Enterprise version only. Check the setup docs:
# https://goteleport.com/docs/enterprise/sso/okta/

resource "teleport_saml_connector" "example" {
  # This block will tell Terraform to never update private key from our side if a keys are managed 
  # from an outside of Terraform.

  # lifecycle {
  #   ignore_changes = [
  #     spec[0].signing_key_pair[0].cert,
  #     spec[0].signing_key_pair[0].private_key,
  #     spec[0].assertion_key_pair[0].cert,
  #     spec[0].assertion_key_pair[0].private_key,
  #   ]
  # }

  # This section tells Terraform that role example must be created before the SAML connector
  depends_on = [
    teleport_role.example
  ]

  metadata = {
    name = "example"
  }

  spec = {
    attributes_to_roles = [{
      name  = "groups"
      roles = ["example"]
      value = "okta-admin"
    },
    {
      name  = "groups"
      roles = ["example"]
      value = "okta-dev"
    }]

    acs               = "https://localhost:3025/v1/webapi/saml/acs"
    entity_descriptor = ""
  }
}
```

## teleport_session_recording_config

|   Name   |  Type  | Required |                           Description                            |
|----------|--------|----------|------------------------------------------------------------------|
| metadata | object |          | Metadata is resource metadata                                    |
| spec     | object |          | Spec is a SessionRecordingConfig specification                   |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources |
| version  | string |          | Version is a resource version                                    |

### metadata

Metadata is resource metadata

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is a SessionRecordingConfig specification

|          Name          |  Type  | Required |                     Description                      |
|------------------------|--------|----------|------------------------------------------------------|
| mode                   | string |          | Mode controls where (or if) the session is recorded. |
| proxy_checks_host_keys | bool   |          |                                                      |

Example:

```
# Teleport session recording config

resource "teleport_session_recording_config" "example" {
  metadata = {
    description = "Session recording config"
    labels = {
      "example" = "yes"
      "teleport.dev/origin" = "dynamic" // This label is added on Teleport side by default
    }
  }

  spec = {
    proxy_checks_host_keys = true
  }
}
```

## teleport_trusted_cluster

|   Name   |  Type  | Required |                            Description                            |
|----------|--------|----------|-------------------------------------------------------------------|
| metadata | object |          | Metadata holds resource metadata.                                 |
| spec     | object | *        | Spec is a Trusted Cluster specification.                          |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources. |
| version  | string |          | Version is a resource version.                                    |

### metadata

Metadata holds resource metadata.

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is a Trusted Cluster specification.

|      Name      |       Type       | Required |                                                                                     Description                                                                                     |
|----------------|------------------|----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| enabled        | bool             |          | Enabled is a bool that indicates if the TrustedCluster is enabled or disabled. Setting Enabled to false has a side effect of deleting the user and host certificate authority (CA). |
| role_map       | object           |          | RoleMap specifies role mappings to remote roles.                                                                                                                                    |
| roles          | array of strings |          | Roles is a list of roles that users will be assuming when connecting to this cluster.                                                                                               |
| token          | string           |          | Token is the authorization token provided by another cluster needed by this cluster to join.                                                                                        |
| tunnel_addr    | string           |          | ReverseTunnelAddress is the address of the SSH proxy server of the cluster to join. If not set, it is derived from &lt;metadata.name&gt;:&lt;default reverse tunnel port&gt;.       |
| web_proxy_addr | string           |          | ProxyAddress is the address of the web proxy server of the cluster to join. If not set, it is derived from &lt;metadata.name&gt;:&lt;default web proxy server port&gt;.             |

#### spec.role_map

RoleMap specifies role mappings to remote roles.

|  Name  |       Type       | Required |                  Description                  |
|--------|------------------|----------|-----------------------------------------------|
| local  | array of strings |          | Local specifies local roles to map to         |
| remote | string           |          | Remote specifies remote role name to map from |

Example:

```
# Teleport trusted cluster
#
# https://goteleport.com/docs/setup/admin/trustedclusters/

resource "teleport_trusted_cluster" "cluster" {
  metadata = {
    name = "primary"
    labels = {
      test = "yes"
    }
  }

  spec = {
    enabled = false
    role_map = [{
      remote = "test"
      local = ["admin"]
    }]
    proxy_addr = "localhost:3080"
    token = "salami"
  }
}

```

## teleport_user

|   Name   |  Type  | Required |                           Description                            |
|----------|--------|----------|------------------------------------------------------------------|
| metadata | object |          | Metadata is resource metadata                                    |
| spec     | object |          | Spec is a user specification                                     |
| sub_kind | string |          | SubKind is an optional resource sub kind, used in some resources |
| version  | string |          | Version is version                                               |

### metadata

Metadata is resource metadata

|    Name     |      Type      | Required |                                                  Description                                                   |
|-------------|----------------|----------|----------------------------------------------------------------------------------------------------------------|
| description | string         |          | Description is object description                                                                              |
| expires     | RFC3339 time   |          | Expires is a global expiry time header can be set on any resource in the system.                               |
| labels      | map of strings |          | Labels is a set of labels                                                                                      |
| name        | string         | *        | Name is an object name                                                                                         |
| namespace   | string         |          | Namespace is object namespace. The field should be called &#34;namespace&#34; when it returns in Teleport 2.4. |

### spec

Spec is a user specification

|       Name        |         Type         | Required |                                                    Description                                                    |
|-------------------|----------------------|----------|-------------------------------------------------------------------------------------------------------------------|
| github_identities | object               |          | GithubIdentities list associated Github OAuth2 identities that let user log in using externally verified identity |
| oidc_identities   | object               |          | OIDCIdentities lists associated OpenID Connect identities that let user log in using externally verified identity |
| roles             | array of strings     |          | Roles is a list of roles assigned to user                                                                         |
| saml_identities   | object               |          | SAMLIdentities lists associated SAML identities that let user log in using externally verified identity           |
| traits            | map of string arrays |          |                                                                                                                   |

#### spec.github_identities

GithubIdentities list associated Github OAuth2 identities that let user log in using externally verified identity

|     Name     |  Type  | Required |                                    Description                                    |
|--------------|--------|----------|-----------------------------------------------------------------------------------|
| connector_id | string |          | ConnectorID is id of registered OIDC connector, e.g. &#39;google-example.com&#39; |
| username     | string |          | Username is username supplied by external identity provider                       |

#### spec.oidc_identities

OIDCIdentities lists associated OpenID Connect identities that let user log in using externally verified identity

|     Name     |  Type  | Required |                                    Description                                    |
|--------------|--------|----------|-----------------------------------------------------------------------------------|
| connector_id | string |          | ConnectorID is id of registered OIDC connector, e.g. &#39;google-example.com&#39; |
| username     | string |          | Username is username supplied by external identity provider                       |

#### spec.saml_identities

SAMLIdentities lists associated SAML identities that let user log in using externally verified identity

|     Name     |  Type  | Required |                                    Description                                    |
|--------------|--------|----------|-----------------------------------------------------------------------------------|
| connector_id | string |          | ConnectorID is id of registered OIDC connector, e.g. &#39;google-example.com&#39; |
| username     | string |          | Username is username supplied by external identity provider                       |

Example:

```
# Teleport User resource

resource "teleport_user" "example" {
  # Tells Terraform that the role could not be destroyed while this user exists
  depends_on = [
    teleport_role.example
  ]

  metadata = {
    name        = "example"
    description = "Example Teleport User"

    expires = "2022-10-12T07:20:50Z"

    labels = {
      example = "yes"
    }
  }

  spec = {
    roles = ["example"]

    oidc_identities = [{
      connector_id = "oidc1"
      username     = "example"
    }]

    traits = {
      "logins1" = ["example"]
      "logins2" = ["example"]
    }

    github_identities = [{
      connector_id = "github"
      username     = "example"
    }]

    saml_identities = [{
      connector_id = "example-saml"
      username     = "example"
    }]
  }
}
```
