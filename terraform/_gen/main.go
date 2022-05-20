package main

import (
	"bytes"
	"log"
	"os"
	"path"
	"text/template"
)

// payload represents template payload
type payload struct {
	// Name represents resource name (capitalized)
	Name string
	// VarName represents resource variable name (underscored)
	VarName string
	// TypeName represents api/types resource type name
	TypeName string
	// IfaceName represents api/types interface for the (usually this is the same as Name)
	IfaceName string
	// GetMethod represents API get method name
	GetMethod string
	// CreateMethod represents API create method name
	CreateMethod string
	// CreateMethod represents API update method name
	UpdateMethod string
	// DeleteMethod represents API reset method used in singular resources
	DeleteMethod string
	// UpsertMethodArity represents Create/Update method arity, if it's 2, then the call signature would be "_, err :="
	UpsertMethodArity int
	// WithSecrets value for a withSecrets param of Get method (empty means no param used)
	WithSecrets string
	// GetWithoutContext indicates that get method has no context parameter (workaround for the User)
	GetWithoutContext bool
	// ID id value on create and import
	ID string
	// RandomMetadataName indicates that Metadata.Name must be generated (supported by plural resources only)
	RandomMetadataName bool
	// Kind Teleport kind for a resource
	Kind string
	// DefaultVersion represents the default resource version on create
	DefaultVersion string
	// StaticID states whether this particular resource has a static (usually 0) Metadata.ID
	// This is relevant to cache enabled clusters: we use Metadata.ID to check if the resource was updated
	// Currently, the resources that don't have a dynamic Metadata.ID are strong consistent: oidc, github and saml connectors
	StaticID bool
}

const (
	pluralResource     = "plural_resource.go.tpl"
	pluralDataSource   = "plural_data_source.go.tpl"
	singularResource   = "singular_resource.go.tpl"
	singularDataSource = "singular_data_source.go.tpl"
)

var (
	app = payload{
		Name:         "App",
		TypeName:     "AppV3",
		VarName:      "app",
		IfaceName:    "Application",
		GetMethod:    "GetApp",
		CreateMethod: "CreateApp",
		UpdateMethod: "UpdateApp",
		DeleteMethod: "DeleteApp",
		ID:           `app.Metadata.Name`,
		Kind:         "app",
		StaticID:     false,
	}

	authPreference = payload{
		Name:         "AuthPreference",
		TypeName:     "AuthPreferenceV2",
		VarName:      "authPreference",
		GetMethod:    "GetAuthPreference",
		CreateMethod: "SetAuthPreference",
		UpdateMethod: "SetAuthPreference",
		DeleteMethod: "ResetAuthPreference",
		ID:           `"auth_preference"`,
		Kind:         "cluster_auth_preference",
		StaticID:     false,
	}

	clusterNetworking = payload{
		Name:         "ClusterNetworkingConfig",
		TypeName:     "ClusterNetworkingConfigV2",
		VarName:      "clusterNetworkingConfig",
		GetMethod:    "GetClusterNetworkingConfig",
		CreateMethod: "SetClusterNetworkingConfig",
		UpdateMethod: "SetClusterNetworkingConfig",
		DeleteMethod: "ResetClusterNetworkingConfig",
		ID:           `"cluster_networking_config"`,
		Kind:         "cluster_networking_config",
		StaticID:     false,
	}

	database = payload{
		Name:         "Database",
		TypeName:     "DatabaseV3",
		VarName:      "database",
		GetMethod:    "GetDatabase",
		CreateMethod: "CreateDatabase",
		UpdateMethod: "UpdateDatabase",
		DeleteMethod: "DeleteDatabase",
		ID:           `database.Metadata.Name`,
		Kind:         "db",
		StaticID:     false,
	}

	githubConnector = payload{
		Name:         "GithubConnector",
		TypeName:     "GithubConnectorV3",
		VarName:      "githubConnector",
		GetMethod:    "GetGithubConnector",
		CreateMethod: "UpsertGithubConnector",
		UpdateMethod: "UpsertGithubConnector",
		DeleteMethod: "DeleteGithubConnector",
		WithSecrets:  "true",
		ID:           "githubConnector.Metadata.Name",
		Kind:         "github",
		StaticID:     true,
	}

	oidcConnector = payload{
		Name:         "OIDCConnector",
		TypeName:     "OIDCConnectorV3",
		VarName:      "oidcConnector",
		GetMethod:    "GetOIDCConnector",
		CreateMethod: "UpsertOIDCConnector",
		UpdateMethod: "UpsertOIDCConnector",
		DeleteMethod: "DeleteOIDCConnector",
		WithSecrets:  "true",
		ID:           "oidcConnector.Metadata.Name",
		Kind:         "oidc",
		StaticID:     true,
	}

	samlConnector = payload{
		Name:         "SAMLConnector",
		TypeName:     "SAMLConnectorV2",
		VarName:      "samlConnector",
		GetMethod:    "GetSAMLConnector",
		CreateMethod: "UpsertSAMLConnector",
		UpdateMethod: "UpsertSAMLConnector",
		DeleteMethod: "DeleteSAMLConnector",
		WithSecrets:  "true",
		ID:           "samlConnector.Metadata.Name",
		Kind:         "saml",
		StaticID:     true,
	}

	provisionToken = payload{
		Name:               "ProvisionToken",
		TypeName:           "ProvisionTokenV2",
		VarName:            "provisionToken",
		GetMethod:          "GetToken",
		CreateMethod:       "UpsertToken",
		UpdateMethod:       "UpsertToken",
		DeleteMethod:       "DeleteToken",
		ID:                 "provisionToken.Metadata.Name",
		RandomMetadataName: true,
		Kind:               "token",
		StaticID:           false,
	}

	role = payload{
		Name:           "Role",
		TypeName:       "RoleV5",
		VarName:        "role",
		GetMethod:      "GetRole",
		CreateMethod:   "UpsertRole",
		UpdateMethod:   "UpsertRole",
		DeleteMethod:   "DeleteRole",
		ID:             "role.Metadata.Name",
		Kind:           "role",
		DefaultVersion: "v4", // TODO (Joerger)
		StaticID:       false,
	}

	sessionRecording = payload{
		Name:         "SessionRecordingConfig",
		TypeName:     "SessionRecordingConfigV2",
		VarName:      "sessionRecordingConfig",
		GetMethod:    "GetSessionRecordingConfig",
		CreateMethod: "SetSessionRecordingConfig",
		UpdateMethod: "SetSessionRecordingConfig",
		DeleteMethod: "ResetSessionRecordingConfig",
		ID:           `"session_recording_config"`,
		Kind:         "session_recording_config",
		StaticID:     false,
	}

	trustedCluster = payload{
		Name:              "TrustedCluster",
		TypeName:          "TrustedClusterV2",
		VarName:           "trustedCluster",
		GetMethod:         "GetTrustedCluster",
		CreateMethod:      "UpsertTrustedCluster",
		UpdateMethod:      "UpsertTrustedCluster",
		DeleteMethod:      "DeleteTrustedCluster",
		UpsertMethodArity: 2,
		ID:                "trustedCluster.Metadata.Name",
		Kind:              "trusted_cluster",
		StaticID:          false,
	}

	user = payload{
		Name:              "User",
		TypeName:          "UserV2",
		VarName:           "user",
		GetMethod:         "GetUser",
		CreateMethod:      "CreateUser",
		UpdateMethod:      "UpdateUser",
		DeleteMethod:      "DeleteUser",
		WithSecrets:       "false",
		GetWithoutContext: true,
		ID:                "user.Metadata.Name",
		Kind:              "user",
		StaticID:          false,
	}
)

func main() {
	generate(app, pluralResource, "provider/resource_teleport_app.go")
	generate(app, pluralDataSource, "provider/data_source_teleport_app.go")
	generate(authPreference, singularResource, "provider/resource_teleport_auth_preference.go")
	generate(authPreference, singularDataSource, "provider/data_source_teleport_auth_preference.go")
	generate(clusterNetworking, singularResource, "provider/resource_teleport_cluster_networking_config.go")
	generate(clusterNetworking, singularDataSource, "provider/data_source_teleport_cluster_networking_config.go")
	generate(database, pluralResource, "provider/resource_teleport_database.go")
	generate(database, pluralDataSource, "provider/data_source_teleport_database.go")
	generate(githubConnector, pluralResource, "provider/resource_teleport_github_connector.go")
	generate(githubConnector, pluralDataSource, "provider/data_source_teleport_github_connector.go")
	generate(oidcConnector, pluralResource, "provider/resource_teleport_oidc_connector.go")
	generate(oidcConnector, pluralDataSource, "provider/data_source_teleport_oidc_connector.go")
	generate(samlConnector, pluralResource, "provider/resource_teleport_saml_connector.go")
	generate(samlConnector, pluralDataSource, "provider/data_source_teleport_saml_connector.go")
	// Provision Token code is an exception because it requires custom id generation TODO: generalize
	generate(provisionToken, pluralResource, "provider/resource_teleport_provision_token.go")
	generate(provisionToken, pluralDataSource, "provider/data_source_teleport_provision_token.go")
	generate(role, pluralResource, "provider/resource_teleport_role.go")
	generate(role, pluralDataSource, "provider/data_source_teleport_role.go")
	generate(trustedCluster, pluralResource, "provider/resource_teleport_trusted_cluster.go")
	generate(trustedCluster, pluralDataSource, "provider/data_source_teleport_trusted_cluster.go")
	generate(sessionRecording, singularResource, "provider/resource_teleport_session_recording_config.go")
	generate(sessionRecording, singularDataSource, "provider/data_source_teleport_session_recording_config.go")
	generate(user, pluralResource, "provider/resource_teleport_user.go")
	generate(user, pluralDataSource, "provider/data_source_teleport_user.go")
}

func generate(p payload, tpl, outFile string) {
	t, err := template.ParseFiles(path.Join("_gen", tpl))
	if err != nil {
		log.Fatal(err)
	}

	var b bytes.Buffer
	err = t.ExecuteTemplate(&b, tpl, p)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(outFile, b.Bytes(), 0777)
	if err != nil {
		log.Fatal(err)
	}
}
