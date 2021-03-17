/*
Copyright 2015 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ui

import (
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/utils"
)

type access struct {
	List   bool `json:"list"`
	Read   bool `json:"read"`
	Edit   bool `json:"edit"`
	Create bool `json:"create"`
	Delete bool `json:"remove"`
}

type accessStrategy struct {
	// Type determines how a user should access teleport resources.
	// ie: does the user require a request to access resources?
	Type services.RequestStrategy `json:"type"`
	// Prompt is the optional dialogue shown to user,
	// when the access strategy type requires a reason.
	Prompt string `json:"prompt"`
}

type userACL struct {
	// Sessions defines access to recorded sessions
	Sessions access `json:"sessions"`
	// AuthConnectors defines access to auth.connectors
	AuthConnectors access `json:"authConnectors"`
	// Roles defines access to roles
	Roles access `json:"roles"`
	// Users defines access to users.
	Users access `json:"users"`
	// TrustedClusters defines access to trusted clusters
	TrustedClusters access `json:"trustedClusters"`
	// Events defines access to audit logs
	Events access `json:"events"`
	// Tokens defines access to tokens.
	Tokens access `json:"tokens"`
	// Nodes defines access to nodes.
	Nodes access `json:"nodes"`
	// AppServers defines access to application servers.
	AppServers access `json:"appServers"`
	// SSH defines access to servers
	SSHLogins []string `json:"sshLogins"`
	// AccessRequests defines access to access requests.
	AccessRequests access `json:"accessRequests"`
}

type authType string

const (
	authLocal authType = "local"
	authSSO   authType = "sso"
)

// UserContext describes a users settings to various resources.
type UserContext struct {
	// AuthType is auth method of this user.
	AuthType authType `json:"authType"`
	// Name is this user name.
	Name string `json:"userName"`
	// ACL contains user access control list.
	ACL userACL `json:"userAcl"`
	// Cluster contains cluster detail for this user's context.
	Cluster *Cluster `json:"cluster"`
	// AccessStrategy describes how a user should access teleport resources.
	AccessStrategy accessStrategy `json:"accessStrategy"`
	// RequestableRoles are roles that the user can assume when requesting access.
	RequestableRoles []string `json:"requestableRoles"`
}

func getLogins(roleSet services.RoleSet) []string {
	allowed := []string{}
	denied := []string{}
	for _, role := range roleSet {
		denied = append(denied, role.GetLogins(services.Deny)...)
		allowed = append(allowed, role.GetLogins(services.Allow)...)
	}

	allowed = utils.Deduplicate(allowed)
	denied = utils.Deduplicate(denied)
	userLogins := []string{}
	for _, login := range allowed {
		loginMatch, _ := services.MatchLogin(denied, login)
		if !loginMatch {
			userLogins = append(userLogins, login)
		}
	}

	return userLogins
}

func hasAccess(roleSet services.RoleSet, ctx *services.Context, kind string, verbs ...string) bool {
	for _, verb := range verbs {
		// Since this check occurs often and it does not imply the caller is trying
		// to access any resource, silence any logging done on the proxy.
		err := roleSet.CheckAccessToRule(ctx, defaults.Namespace, kind, verb, true)
		if err != nil {
			return false
		}
	}

	return true
}

func newAccess(roleSet services.RoleSet, ctx *services.Context, kind string) access {
	return access{
		List:   hasAccess(roleSet, ctx, kind, services.VerbList),
		Read:   hasAccess(roleSet, ctx, kind, services.VerbRead),
		Edit:   hasAccess(roleSet, ctx, kind, services.VerbUpdate),
		Create: hasAccess(roleSet, ctx, kind, services.VerbCreate),
		Delete: hasAccess(roleSet, ctx, kind, services.VerbDelete),
	}
}

func getAccessStrategy(roleset services.RoleSet) accessStrategy {
	strategy := services.RequestStrategyOptional
	prompt := ""

	for _, role := range roleset {
		options := role.GetOptions()

		if options.RequestAccess == services.RequestStrategyReason {
			strategy = services.RequestStrategyReason
			prompt = options.RequestPrompt
			break
		}

		if options.RequestAccess == services.RequestStrategyAlways {
			strategy = services.RequestStrategyAlways
		}
	}

	return accessStrategy{
		Type:   strategy,
		Prompt: prompt,
	}
}

// NewUserContext returns user context
func NewUserContext(user services.User, userRoles services.RoleSet) (*UserContext, error) {
	ctx := &services.Context{User: user}
	sessionAccess := newAccess(userRoles, ctx, services.KindSession)
	roleAccess := newAccess(userRoles, ctx, services.KindRole)
	authConnectors := newAccess(userRoles, ctx, services.KindAuthConnector)
	trustedClusterAccess := newAccess(userRoles, ctx, services.KindTrustedCluster)
	eventAccess := newAccess(userRoles, ctx, services.KindEvent)
	userAccess := newAccess(userRoles, ctx, services.KindUser)
	tokenAccess := newAccess(userRoles, ctx, services.KindToken)
	nodeAccess := newAccess(userRoles, ctx, services.KindNode)
	appServerAccess := newAccess(userRoles, ctx, services.KindAppServer)
	requestAccess := newAccess(userRoles, ctx, services.KindAccessRequest)

	logins := getLogins(userRoles)
	accessStrategy := getAccessStrategy(userRoles)

	acl := userACL{
		AccessRequests:  requestAccess,
		AppServers:      appServerAccess,
		AuthConnectors:  authConnectors,
		TrustedClusters: trustedClusterAccess,
		Sessions:        sessionAccess,
		Roles:           roleAccess,
		Events:          eventAccess,
		SSHLogins:       logins,
		Users:           userAccess,
		Tokens:          tokenAccess,
		Nodes:           nodeAccess,
	}

	// local user
	authType := authLocal

	// check for any SSO identities
	isSSO := len(user.GetOIDCIdentities()) > 0 ||
		len(user.GetGithubIdentities()) > 0 ||
		len(user.GetSAMLIdentities()) > 0

	if isSSO {
		// SSO user
		authType = authSSO
	}

	return &UserContext{
		Name:           user.GetName(),
		ACL:            acl,
		AuthType:       authType,
		AccessStrategy: accessStrategy,
	}, nil
}
