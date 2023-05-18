// Copyright 2023 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"text/template"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/exp/slices"

	"github.com/gravitational/teleport-plugins/terraform/provider"
	"github.com/gravitational/teleport-plugins/terraform/tfschema"
	loginruleSchema "github.com/gravitational/teleport-plugins/terraform/tfschema/loginrule/v1"
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
	// HasStaticID states whether this particular resource has a static (usually 0) Metadata.ID
	// This is relevant to cache enabled clusters: we use Metadata.ID to check if the resource was updated
	// Currently, the resources that don't have a dynamic Metadata.ID are strong consistent: oidc, github and saml connectors
	HasStaticID bool
	// ProtoPackagePath is the path of the package where the protobuf type of
	// the resource is defined.
	ProtoPackagePath string
	// ProtoPackagePath is the name of the package where the protobuf type of
	// the resource is defined.
	ProtoPackage string
	// SchemaPackagePath is the path of the package where the resource schema
	// definitions are defined.
	SchemaPackagePath string
	// SchemaPackagePath is the name of the package where the resource schema
	// definitions are defined.
	SchemaPackage string
	// IsPlainStruct states whether the resource type used by the API methods
	// for this resource is a plain struct, rather than an interface.
	IsPlainStruct bool
	// ExtraImports contains a list of imports that are being used.
	ExtraImports []string
	// TerraformResourceType represents the resource type in Terraform code.
	// e.g. `terraform import <resource_type>.<resource_name> identifier`.
	// This is also used to name the generated files.
	TerraformResourceType string
}

func (p *payload) CheckAndSetDefaults() error {
	if p.ProtoPackage == "" {
		p.ProtoPackage = "apitypes"
	}
	if p.ProtoPackagePath == "" {
		p.ProtoPackagePath = "github.com/gravitational/teleport/api/types"
	}
	if p.SchemaPackage == "" {
		p.SchemaPackage = "tfschema"
	}
	if p.SchemaPackagePath == "" {
		p.SchemaPackagePath = "github.com/gravitational/teleport-plugins/terraform/tfschema"
	}
	return nil
}

const (
	pluralResource          = "plural_resource.go.tpl"
	pluralDataSource        = "plural_data_source.go.tpl"
	singularResource        = "singular_resource.go.tpl"
	singularDataSource      = "singular_data_source.go.tpl"
	outFileResourceFormat   = "provider/resource_%s.go"
	outFileDataSourceFormat = "provider/data_source_%s.go"

	referenceDocsTemplate = "referencedocs.go.tpl"
)

var (
	app = payload{
		Name:                  "App",
		TypeName:              "AppV3",
		VarName:               "app",
		IfaceName:             "Application",
		GetMethod:             "GetApp",
		CreateMethod:          "CreateApp",
		UpdateMethod:          "UpdateApp",
		DeleteMethod:          "DeleteApp",
		ID:                    `app.Metadata.Name`,
		Kind:                  "app",
		HasStaticID:           false,
		TerraformResourceType: "teleport_app",
	}

	authPreference = payload{
		Name:                  "AuthPreference",
		TypeName:              "AuthPreferenceV2",
		VarName:               "authPreference",
		GetMethod:             "GetAuthPreference",
		CreateMethod:          "SetAuthPreference",
		UpdateMethod:          "SetAuthPreference",
		DeleteMethod:          "ResetAuthPreference",
		ID:                    `"auth_preference"`,
		Kind:                  "cluster_auth_preference",
		HasStaticID:           false,
		TerraformResourceType: "teleport_auth_preference",
	}

	clusterNetworking = payload{
		Name:                  "ClusterNetworkingConfig",
		TypeName:              "ClusterNetworkingConfigV2",
		VarName:               "clusterNetworkingConfig",
		GetMethod:             "GetClusterNetworkingConfig",
		CreateMethod:          "SetClusterNetworkingConfig",
		UpdateMethod:          "SetClusterNetworkingConfig",
		DeleteMethod:          "ResetClusterNetworkingConfig",
		ID:                    `"cluster_networking_config"`,
		Kind:                  "cluster_networking_config",
		HasStaticID:           false,
		TerraformResourceType: "teleport_cluster_networking_config",
	}

	database = payload{
		Name:                  "Database",
		TypeName:              "DatabaseV3",
		VarName:               "database",
		GetMethod:             "GetDatabase",
		CreateMethod:          "CreateDatabase",
		UpdateMethod:          "UpdateDatabase",
		DeleteMethod:          "DeleteDatabase",
		ID:                    `database.Metadata.Name`,
		Kind:                  "db",
		HasStaticID:           false,
		TerraformResourceType: "teleport_database",
	}

	githubConnector = payload{
		Name:                  "GithubConnector",
		TypeName:              "GithubConnectorV3",
		VarName:               "githubConnector",
		GetMethod:             "GetGithubConnector",
		CreateMethod:          "UpsertGithubConnector",
		UpdateMethod:          "UpsertGithubConnector",
		DeleteMethod:          "DeleteGithubConnector",
		WithSecrets:           "true",
		ID:                    "githubConnector.Metadata.Name",
		Kind:                  "github",
		HasStaticID:           true,
		TerraformResourceType: "teleport_github_connector",
	}

	oidcConnector = payload{
		Name:                  "OIDCConnector",
		TypeName:              "OIDCConnectorV3",
		VarName:               "oidcConnector",
		GetMethod:             "GetOIDCConnector",
		CreateMethod:          "UpsertOIDCConnector",
		UpdateMethod:          "UpsertOIDCConnector",
		DeleteMethod:          "DeleteOIDCConnector",
		WithSecrets:           "true",
		ID:                    "oidcConnector.Metadata.Name",
		Kind:                  "oidc",
		HasStaticID:           true,
		TerraformResourceType: "teleport_oidc_connector",
	}

	samlConnector = payload{
		Name:                  "SAMLConnector",
		TypeName:              "SAMLConnectorV2",
		VarName:               "samlConnector",
		GetMethod:             "GetSAMLConnector",
		CreateMethod:          "UpsertSAMLConnector",
		UpdateMethod:          "UpsertSAMLConnector",
		DeleteMethod:          "DeleteSAMLConnector",
		WithSecrets:           "true",
		ID:                    "samlConnector.Metadata.Name",
		Kind:                  "saml",
		HasStaticID:           true,
		TerraformResourceType: "teleport_saml_connector",
	}

	provisionToken = payload{
		Name:                  "ProvisionToken",
		TypeName:              "ProvisionTokenV2",
		VarName:               "provisionToken",
		GetMethod:             "GetToken",
		CreateMethod:          "UpsertToken",
		UpdateMethod:          "UpsertToken",
		DeleteMethod:          "DeleteToken",
		ID:                    "strconv.FormatInt(provisionToken.Metadata.ID, 10)", // must be a string
		RandomMetadataName:    true,
		Kind:                  "token",
		HasStaticID:           false,
		ExtraImports:          []string{"strconv"},
		TerraformResourceType: "teleport_provision_token",
	}

	role = payload{
		Name:                  "Role",
		TypeName:              "RoleV6",
		VarName:               "role",
		GetMethod:             "GetRole",
		CreateMethod:          "UpsertRole",
		UpdateMethod:          "UpsertRole",
		DeleteMethod:          "DeleteRole",
		ID:                    "role.Metadata.Name",
		Kind:                  "role",
		HasStaticID:           false,
		TerraformResourceType: "teleport_role",
	}

	sessionRecording = payload{
		Name:                  "SessionRecordingConfig",
		TypeName:              "SessionRecordingConfigV2",
		VarName:               "sessionRecordingConfig",
		GetMethod:             "GetSessionRecordingConfig",
		CreateMethod:          "SetSessionRecordingConfig",
		UpdateMethod:          "SetSessionRecordingConfig",
		DeleteMethod:          "ResetSessionRecordingConfig",
		ID:                    `"session_recording_config"`,
		Kind:                  "session_recording_config",
		HasStaticID:           false,
		TerraformResourceType: "teleport_session_recording_config",
	}

	trustedCluster = payload{
		Name:                  "TrustedCluster",
		TypeName:              "TrustedClusterV2",
		VarName:               "trustedCluster",
		GetMethod:             "GetTrustedCluster",
		CreateMethod:          "UpsertTrustedCluster",
		UpdateMethod:          "UpsertTrustedCluster",
		DeleteMethod:          "DeleteTrustedCluster",
		UpsertMethodArity:     2,
		ID:                    "trustedCluster.Metadata.Name",
		Kind:                  "trusted_cluster",
		HasStaticID:           false,
		TerraformResourceType: "teleport_trusted_cluster",
	}

	user = payload{
		Name:                  "User",
		TypeName:              "UserV2",
		VarName:               "user",
		GetMethod:             "GetUser",
		CreateMethod:          "CreateUser",
		UpdateMethod:          "UpdateUser",
		DeleteMethod:          "DeleteUser",
		WithSecrets:           "false",
		GetWithoutContext:     true,
		ID:                    "user.Metadata.Name",
		Kind:                  "user",
		HasStaticID:           false,
		TerraformResourceType: "teleport_user",
	}

	loginRule = payload{
		Name:                  "LoginRule",
		TypeName:              "LoginRule",
		VarName:               "loginRule",
		GetMethod:             "GetLoginRule",
		CreateMethod:          "UpsertLoginRule",
		UpsertMethodArity:     2,
		UpdateMethod:          "UpsertLoginRule",
		DeleteMethod:          "DeleteLoginRule",
		ID:                    "loginRule.Metadata.Name",
		Kind:                  "login_rule",
		HasStaticID:           false,
		ProtoPackage:          "loginrulev1",
		ProtoPackagePath:      "github.com/gravitational/teleport/api/gen/proto/go/teleport/loginrule/v1",
		SchemaPackage:         "schemav1",
		SchemaPackagePath:     "github.com/gravitational/teleport-plugins/terraform/tfschema/loginrule/v1",
		IsPlainStruct:         true,
		TerraformResourceType: "teleport_login_rule",
	}
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "docs" {
		genReferenceDocs()
	} else {
		genTFSchema()
	}
}

func genTFSchema() {
	generateResource(app, pluralResource)
	generateDataSource(app, pluralDataSource)
	generateResource(authPreference, singularResource)
	generateDataSource(authPreference, singularDataSource)
	generateResource(clusterNetworking, singularResource)
	generateDataSource(clusterNetworking, singularDataSource)
	generateResource(database, pluralResource)
	generateDataSource(database, pluralDataSource)
	generateResource(githubConnector, pluralResource)
	generateDataSource(githubConnector, pluralDataSource)
	generateResource(oidcConnector, pluralResource)
	generateDataSource(oidcConnector, pluralDataSource)
	generateResource(samlConnector, pluralResource)
	generateDataSource(samlConnector, pluralDataSource)
	generateResource(provisionToken, pluralResource)
	generateDataSource(provisionToken, pluralDataSource)
	generateResource(role, pluralResource)
	generateDataSource(role, pluralDataSource)
	generateResource(trustedCluster, pluralResource)
	generateDataSource(trustedCluster, pluralDataSource)
	generateResource(sessionRecording, singularResource)
	generateDataSource(sessionRecording, singularDataSource)
	generateResource(user, pluralResource)
	generateDataSource(user, pluralDataSource)
	generateResource(loginRule, pluralResource)
	generateDataSource(loginRule, pluralDataSource)
}

func generateResource(p payload, tpl string) {
	outFile := fmt.Sprintf(outFileResourceFormat, p.TerraformResourceType)
	generate(p, tpl, outFile)
}
func generateDataSource(p payload, tpl string) {
	outFile := fmt.Sprintf(outFileDataSourceFormat, p.TerraformResourceType)
	generate(p, tpl, outFile)
}

func generate(p payload, tpl, outFile string) {
	if err := p.CheckAndSetDefaults(); err != nil {
		log.Fatal(err)
	}

	t, err := template.ParseFiles(path.Join("gen", tpl))
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

// Create Docs Markdown
var (
	mapResourceSchema = map[string]func(context.Context) (tfsdk.Schema, diag.Diagnostics){
		"app":                       tfschema.GenSchemaAppV3,
		"auth_preference":           tfschema.GenSchemaAuthPreferenceV2,
		"bot":                       provider.GenSchemaBot,
		"cluster_networking_config": tfschema.GenSchemaClusterNetworkingConfigV2,
		"database":                  tfschema.GenSchemaDatabaseV3,
		"github_connector":          tfschema.GenSchemaGithubConnectorV3,
		"login_rule":                loginruleSchema.GenSchemaLoginRule,
		"oidc_connector":            tfschema.GenSchemaOIDCConnectorV3,
		"provision_token":           tfschema.GenSchemaProvisionTokenV2,
		"role":                      tfschema.GenSchemaRoleV6,
		"saml_connector":            tfschema.GenSchemaSAMLConnectorV2,
		"session_recording_config":  tfschema.GenSchemaSessionRecordingConfigV2,
		"trusted_cluster":           tfschema.GenSchemaTrustedClusterV2,
		"user":                      tfschema.GenSchemaUserV2,
	}

	// hiddenFields are fields that are not outputted to the reference doc.
	// It supports non-top level fields by adding its prefix. Eg: metadata.namespace
	hiddenFields = []string{
		"id",   // read only field
		"kind", // each resource already defines its kind so this is redundant
	}

	// fieldComments is used to define specific descriptions for the given fields.
	// Typical usage is for enums which we don't have comments yet.
	fieldComments = map[string]string{
		"teleport_auth_preference.spec.require_session_mfa": "RequireMFAType is the type of MFA requirement enforced for this cluster: 0:Off, 1:Session, 2:SessionAndHardwareKey, 3:HardwareKeyTouch",
		"teleport_role.spec.options.require_session_mfa":    "RequireMFAType is the type of MFA requirement enforced for this role: 0:Off, 1:Session, 2:SessionAndHardwareKey, 3:HardwareKeyTouch",
	}
)

func genReferenceDocs() {
	sortedNames := make([]string, 0, len(mapResourceSchema))
	for k := range mapResourceSchema {
		sortedNames = append(sortedNames, k)
	}
	sort.Strings(sortedNames)

	t, err := template.ParseFiles(path.Join("gen", referenceDocsTemplate))
	if err != nil {
		log.Fatal(err)
	}

	referenceDocsResource := make([]referenceDocResource, 0, len(mapResourceSchema))
	for _, name := range sortedNames {
		resourceName := "teleport_" + name

		schemaFn := mapResourceSchema[name]
		schema, diags := schemaFn(context.Background())
		if diags.HasError() {
			log.Fatalf("%v", diags)
		}

		fieldDescBuilder := strings.Builder{}
		dumpAttributes(&fieldDescBuilder, 0, resourceName, "", schema.Attributes)

		exampleFileName := fmt.Sprintf("example/%s.tf.example", name)
		exampleBytes, err := os.ReadFile(exampleFileName)
		if err != nil {
			log.Fatalf("error loading %q file: %v", exampleFileName, err)
		}

		referenceDocsResource = append(referenceDocsResource, referenceDocResource{
			Name:       resourceName,
			FieldsDesc: fieldDescBuilder.String(),
			Example:    string(exampleBytes),
		})
	}

	var b bytes.Buffer
	err = t.ExecuteTemplate(&b, referenceDocsTemplate, map[string]any{
		"resourceList": sortedNames,
		"resourcesDoc": referenceDocsResource,
	})
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("reference.mdx", b.Bytes(), 0777)
	if err != nil {
		log.Fatal(err)
	}
}

type referenceDocResource struct {
	Name       string
	FieldsDesc string
	Example    string
}

func dumpAttributes(fp io.Writer, level int, resourceName string, prefix string, attrs map[string]tfsdk.Attribute) {
	sortedAttrKeys := make([]string, 0, len(attrs))
	for k := range attrs {
		sortedAttrKeys = append(sortedAttrKeys, k)
	}
	sort.Strings(sortedAttrKeys)

	table := tablewriter.NewWriter(fp)
	table.SetHeader([]string{"Name", "Type", "Required", "Description"})
	table.SetAutoWrapText(false)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.SetAutoFormatHeaders(false)

	for _, name := range sortedAttrKeys {
		fullFieldPath := resourceName + "." + prefix + name
		attr := attrs[name]

		if slices.Contains(hiddenFields, prefix+name) {
			continue
		}

		description := attr.Description
		if d, found := fieldComments[fullFieldPath]; found {
			description = d
		}
		table.Append([]string{name, typ(attr.Type), requiredString(attr.Required), html.EscapeString(description)})
	}
	table.Render()
	fmt.Fprintln(fp)

	for _, name := range sortedAttrKeys {
		attr := attrs[name]
		if attr.Attributes != nil {
			fmt.Fprintf(fp, "%s %s\n", strings.Repeat("#", 3+level), prefix+name)
			fmt.Fprintln(fp)
			fmt.Fprintln(fp, attr.Description)
			fmt.Fprintln(fp)

			dumpAttributes(fp, level+1, resourceName, prefix+name+".", attr.Attributes.GetAttributes())
		}
	}
}

func typ(typ attr.Type) string {
	if typ == nil {
		return "object"
	}

	switch typ.String() {
	case "types.StringType":
		return "string"
	case "TimeType(2006-01-02T15:04:05Z07:00)":
		return "RFC3339 time"
	case "DurationType":
		return "duration"
	case "types.BoolType":
		return "bool"
	case "types.MapType[types.StringType]":
		return "map of strings"
	case "types.ListType[types.StringType]":
		return "array of strings"
	case "types.MapType[types.ListType[types.StringType]]":
		return "map of string arrays"
	case "types.Int64Type":
		return "number"
	default:
		return typ.String()
	}
}

func requiredString(r bool) string {
	if r {
		return "*"
	}
	return " "
}
