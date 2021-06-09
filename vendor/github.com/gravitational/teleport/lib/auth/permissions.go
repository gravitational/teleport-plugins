/*
Copyright 2015-2018 Gravitational, Inc.

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

package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/wrappers"
	"github.com/gravitational/teleport/api/utils"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/tlsca"

	"github.com/gravitational/trace"
	"github.com/vulcand/predicate/builder"
)

// NewAdminContext returns new admin auth context
func NewAdminContext() (*Context, error) {
	return NewBuiltinRoleContext(types.RoleAdmin)
}

// NewBuiltinRoleContext create auth context for the provided builtin role.
func NewBuiltinRoleContext(role types.SystemRole) (*Context, error) {
	authContext, err := contextForBuiltinRole(BuiltinRole{Role: role, Username: fmt.Sprintf("%v", role)}, nil)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return authContext, nil
}

// NewAuthorizer returns new authorizer using backends
func NewAuthorizer(clusterName string, access services.Access, identity services.UserGetter, trust services.Trust) (Authorizer, error) {
	if clusterName == "" {
		return nil, trace.BadParameter("missing parameter clusterName")
	}
	if access == nil {
		return nil, trace.BadParameter("missing parameter access")
	}
	if identity == nil {
		return nil, trace.BadParameter("missing parameter identity")
	}
	if trust == nil {
		return nil, trace.BadParameter("missing parameter trust")
	}
	return &authorizer{clusterName: clusterName, access: access, identity: identity, trust: trust}, nil
}

// Authorizer authorizes identity and returns auth context
type Authorizer interface {
	// Authorize authorizes user based on identity supplied via context
	Authorize(ctx context.Context) (*Context, error)
}

// authorizer creates new local authorizer
type authorizer struct {
	clusterName string
	access      services.Access
	identity    services.UserGetter
	trust       services.Trust
}

// AuthContext is authorization context
type Context struct {
	// User is the user name
	User services.User
	// Checker is access checker
	Checker services.AccessChecker
	// Identity holds the caller identity:
	// 1. If caller is a user
	//   a. local user identity
	//   b. remote user identity remapped to local identity based on trusted
	//      cluster role mapping.
	// 2. If caller is a teleport instance, Identity holds their identity as-is
	//    (because there's no role mapping for non-human roles)
	Identity IdentityGetter
	// UnmappedIdentity holds the original caller identity. If this is a remote
	// user, UnmappedIdentity holds the data before role mapping. Otherwise,
	// it's identical to Identity.
	UnmappedIdentity IdentityGetter
}

// Authorize authorizes user based on identity supplied via context
func (a *authorizer) Authorize(ctx context.Context) (*Context, error) {
	if ctx == nil {
		return nil, trace.AccessDenied("missing authentication context")
	}
	userI := ctx.Value(ContextUser)
	authContext, err := a.fromUser(userI)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return authContext, nil
}

func (a *authorizer) fromUser(userI interface{}) (*Context, error) {
	switch user := userI.(type) {
	case LocalUser:
		return a.authorizeLocalUser(user)
	case RemoteUser:
		return a.authorizeRemoteUser(user)
	case BuiltinRole:
		return a.authorizeBuiltinRole(user)
	case RemoteBuiltinRole:
		return a.authorizeRemoteBuiltinRole(user)
	default:
		return nil, trace.AccessDenied("unsupported context type %T", userI)
	}
}

// authorizeLocalUser returns authz context based on the username
func (a *authorizer) authorizeLocalUser(u LocalUser) (*Context, error) {
	return contextForLocalUser(u, a.identity, a.access)
}

// authorizeRemoteUser returns checker based on cert authority roles
func (a *authorizer) authorizeRemoteUser(u RemoteUser) (*Context, error) {
	ca, err := a.trust.GetCertAuthority(services.CertAuthID{Type: services.UserCA, DomainName: u.ClusterName}, false)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	roleNames, err := services.MapRoles(ca.CombinedMapping(), u.RemoteRoles)
	if err != nil {
		return nil, trace.AccessDenied("failed to map roles for remote user %q from cluster %q with remote roles %v", u.Username, u.ClusterName, u.RemoteRoles)
	}
	if len(roleNames) == 0 {
		return nil, trace.AccessDenied("no roles mapped for remote user %q from cluster %q with remote roles %v", u.Username, u.ClusterName, u.RemoteRoles)
	}
	// Set internal traits for the remote user. This allows Teleport to work by
	// passing exact logins, Kubernetes users/groups and database users/names
	// to the remote cluster.
	traits := map[string][]string{
		teleport.TraitLogins:     u.Principals,
		teleport.TraitKubeGroups: u.KubernetesGroups,
		teleport.TraitKubeUsers:  u.KubernetesUsers,
		teleport.TraitDBNames:    u.DatabaseNames,
		teleport.TraitDBUsers:    u.DatabaseUsers,
	}
	// Prior to Teleport 6.2 no user traits were passed to remote clusters
	// except for the internal ones specified above.
	//
	// To preserve backwards compatible behavior, when applying traits from user
	// identity, make sure to filter out those already present in the map above.
	//
	// This ensures that if e.g. there's a "logins" trait in the root user's
	// identity, it won't overwrite the internal "logins" trait set above
	// causing behavior change.
	for k, v := range u.Identity.Traits {
		if _, ok := traits[k]; !ok {
			traits[k] = v
		}
	}
	log.Debugf("Mapped roles %v of remote user %q to local roles %v and traits %v.",
		u.RemoteRoles, u.Username, roleNames, traits)
	checker, err := services.FetchRoles(roleNames, a.access, traits)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// The user is prefixed with "remote-" and suffixed with cluster name with
	// the hope that it does not match a real local user.
	user, err := services.NewUser(fmt.Sprintf("remote-%v-%v", u.Username, u.ClusterName))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user.SetTraits(traits)

	// Set the list of roles this user has in the remote cluster.
	user.SetRoles(roleNames)

	// Adjust expiry based on locally mapped roles.
	ttl := time.Until(u.Identity.Expires)
	ttl = checker.AdjustSessionTTL(ttl)

	kubeUsers, kubeGroups, err := checker.CheckKubeGroupsAndUsers(ttl, false)
	// IsNotFound means that the user has no k8s users or groups, which is fine
	// in many cases. The downstream k8s handler will ensure that users/groups
	// are set if this is a k8s request.
	if err != nil && !trace.IsNotFound(err) {
		return nil, trace.Wrap(err)
	}
	principals, err := checker.CheckLoginDuration(ttl)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// Convert u.Identity into the mapped local identity.
	//
	// This prevents downstream users from accidentally using the unmapped
	// identity information and confusing who's accessing a resource.
	identity := tlsca.Identity{
		Username:         user.GetName(),
		Groups:           user.GetRoles(),
		Traits:           wrappers.Traits(traits),
		Principals:       principals,
		KubernetesGroups: kubeGroups,
		KubernetesUsers:  kubeUsers,
		TeleportCluster:  a.clusterName,
		Expires:          time.Now().Add(ttl),

		// These fields are for routing and restrictions, safe to re-use from
		// unmapped identity.
		Usage:             u.Identity.Usage,
		RouteToCluster:    u.Identity.RouteToCluster,
		KubernetesCluster: u.Identity.KubernetesCluster,
		RouteToApp:        u.Identity.RouteToApp,
		RouteToDatabase:   u.Identity.RouteToDatabase,
		MFAVerified:       u.Identity.MFAVerified,
		ClientIP:          u.Identity.ClientIP,
	}

	return &Context{
		User:             user,
		Checker:          RemoteUserRoleSet{checker},
		Identity:         WrapIdentity(identity),
		UnmappedIdentity: u,
	}, nil
}

// authorizeBuiltinRole authorizes builtin role
func (a *authorizer) authorizeBuiltinRole(r BuiltinRole) (*Context, error) {
	config, err := r.GetClusterConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return contextForBuiltinRole(r, config)
}

func (a *authorizer) authorizeRemoteBuiltinRole(r RemoteBuiltinRole) (*Context, error) {
	if r.Role != teleport.RoleProxy {
		return nil, trace.AccessDenied("access denied for remote %v connecting to cluster", r.Role)
	}
	roles, err := services.FromSpec(
		string(teleport.RoleRemoteProxy),
		services.RoleSpecV3{
			Allow: services.RoleConditions{
				Namespaces: []string{services.Wildcard},
				Rules: []services.Rule{
					services.NewRule(services.KindNode, services.RO()),
					services.NewRule(services.KindProxy, services.RO()),
					services.NewRule(services.KindCertAuthority, services.ReadNoSecrets()),
					services.NewRule(services.KindNamespace, services.RO()),
					services.NewRule(services.KindUser, services.RO()),
					services.NewRule(services.KindRole, services.RO()),
					services.NewRule(services.KindAuthServer, services.RO()),
					services.NewRule(services.KindReverseTunnel, services.RO()),
					services.NewRule(services.KindTunnelConnection, services.RO()),
					services.NewRule(services.KindClusterConfig, services.RO()),
					services.NewRule(services.KindKubeService, services.RO()),
					// this rule allows remote proxy to update the cluster's certificate authorities
					// during certificates renewal
					{
						Resources: []string{services.KindCertAuthority},
						// It is important that remote proxy can only rotate
						// existing certificate authority, and not create or update new ones
						Verbs: []string{services.VerbRead, services.VerbRotate},
						// allow administrative access to the certificate authority names
						// matching the cluster name only
						Where: builder.Equals(services.ResourceNameExpr, builder.String(r.ClusterName)).String(),
					},
				},
			},
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user, err := services.NewUser(r.Username)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user.SetRoles([]string{string(teleport.RoleRemoteProxy)})
	return &Context{
		User:             user,
		Checker:          RemoteBuiltinRoleSet{roles},
		Identity:         r,
		UnmappedIdentity: r,
	}, nil
}

// GetCheckerForBuiltinRole returns checkers for embedded builtin role
func GetCheckerForBuiltinRole(clusterName string, clusterConfig services.ClusterConfig, role teleport.Role) (services.RoleSet, error) {
	switch role {
	case teleport.RoleAuth:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{services.Wildcard},
					Rules: []services.Rule{
						services.NewRule(services.KindAuthServer, services.RW()),
					},
				},
			})
	case teleport.RoleProvisionToken:
		return services.FromSpec(role.String(), services.RoleSpecV3{})
	case teleport.RoleNode:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{services.Wildcard},
					Rules: []services.Rule{
						services.NewRule(services.KindNode, services.RW()),
						services.NewRule(services.KindSSHSession, services.RW()),
						services.NewRule(services.KindEvent, services.RW()),
						services.NewRule(services.KindProxy, services.RO()),
						services.NewRule(services.KindCertAuthority, services.ReadNoSecrets()),
						services.NewRule(services.KindUser, services.RO()),
						services.NewRule(services.KindNamespace, services.RO()),
						services.NewRule(services.KindRole, services.RO()),
						services.NewRule(services.KindAuthServer, services.RO()),
						services.NewRule(services.KindReverseTunnel, services.RW()),
						services.NewRule(services.KindTunnelConnection, services.RO()),
						services.NewRule(services.KindClusterConfig, services.RO()),
						services.NewRule(services.KindClusterAuthPreference, services.RO()),
						services.NewRule(services.KindSemaphore, services.RW()),
					},
				},
			})
	case teleport.RoleApp:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{services.Wildcard},
					Rules: []services.Rule{
						services.NewRule(services.KindEvent, services.RW()),
						services.NewRule(services.KindProxy, services.RO()),
						services.NewRule(services.KindCertAuthority, services.ReadNoSecrets()),
						services.NewRule(services.KindUser, services.RO()),
						services.NewRule(services.KindNamespace, services.RO()),
						services.NewRule(services.KindRole, services.RO()),
						services.NewRule(services.KindAuthServer, services.RO()),
						services.NewRule(services.KindReverseTunnel, services.RW()),
						services.NewRule(services.KindTunnelConnection, services.RO()),
						services.NewRule(services.KindClusterConfig, services.RO()),
						services.NewRule(services.KindClusterAuthPreference, services.RO()),
						services.NewRule(services.KindAppServer, services.RW()),
						services.NewRule(services.KindWebSession, services.RO()),
						services.NewRule(services.KindWebToken, services.RO()),
						services.NewRule(services.KindJWT, services.RW()),
					},
				},
			})
	case teleport.RoleDatabase:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{services.Wildcard},
					Rules: []services.Rule{
						services.NewRule(services.KindEvent, services.RW()),
						services.NewRule(services.KindProxy, services.RO()),
						services.NewRule(services.KindCertAuthority, services.ReadNoSecrets()),
						services.NewRule(services.KindUser, services.RO()),
						services.NewRule(services.KindNamespace, services.RO()),
						services.NewRule(services.KindRole, services.RO()),
						services.NewRule(services.KindAuthServer, services.RO()),
						services.NewRule(services.KindReverseTunnel, services.RW()),
						services.NewRule(services.KindTunnelConnection, services.RO()),
						services.NewRule(services.KindClusterConfig, services.RO()),
						services.NewRule(services.KindClusterAuthPreference, services.RO()),
						services.NewRule(types.KindDatabaseServer, services.RW()),
					},
				},
			})
	case teleport.RoleProxy:
		// if in recording mode, return a different set of permissions than regular
		// mode. recording proxy needs to be able to generate host certificates.
		if services.IsRecordAtProxy(clusterConfig.GetSessionRecording()) {
			return services.FromSpec(
				role.String(),
				services.RoleSpecV3{
					Allow: services.RoleConditions{
						Namespaces:    []string{services.Wildcard},
						ClusterLabels: services.Labels{services.Wildcard: []string{services.Wildcard}},
						Rules: []services.Rule{
							services.NewRule(services.KindProxy, services.RW()),
							services.NewRule(services.KindOIDCRequest, services.RW()),
							services.NewRule(services.KindSSHSession, services.RW()),
							services.NewRule(services.KindSession, services.RO()),
							services.NewRule(services.KindEvent, services.RW()),
							services.NewRule(services.KindSAMLRequest, services.RW()),
							services.NewRule(services.KindOIDC, services.ReadNoSecrets()),
							services.NewRule(services.KindSAML, services.ReadNoSecrets()),
							services.NewRule(services.KindGithub, services.ReadNoSecrets()),
							services.NewRule(services.KindGithubRequest, services.RW()),
							services.NewRule(services.KindNamespace, services.RO()),
							services.NewRule(services.KindNode, services.RO()),
							services.NewRule(services.KindAuthServer, services.RO()),
							services.NewRule(services.KindReverseTunnel, services.RO()),
							services.NewRule(services.KindCertAuthority, services.ReadNoSecrets()),
							services.NewRule(services.KindUser, services.RO()),
							services.NewRule(services.KindRole, services.RO()),
							services.NewRule(services.KindClusterAuthPreference, services.RO()),
							services.NewRule(services.KindClusterConfig, services.RO()),
							services.NewRule(services.KindClusterName, services.RO()),
							services.NewRule(services.KindStaticTokens, services.RO()),
							services.NewRule(services.KindTunnelConnection, services.RW()),
							services.NewRule(services.KindHostCert, services.RW()),
							services.NewRule(services.KindRemoteCluster, services.RO()),
							services.NewRule(services.KindSemaphore, services.RW()),
							services.NewRule(services.KindAppServer, services.RO()),
							services.NewRule(services.KindWebSession, services.RW()),
							services.NewRule(services.KindWebToken, services.RW()),
							services.NewRule(services.KindKubeService, services.RW()),
							services.NewRule(types.KindDatabaseServer, services.RO()),
							// this rule allows local proxy to update the remote cluster's host certificate authorities
							// during certificates renewal
							{
								Resources: []string{services.KindCertAuthority},
								Verbs:     []string{services.VerbCreate, services.VerbRead, services.VerbUpdate},
								// allow administrative access to the host certificate authorities
								// matching any cluster name except local
								Where: builder.And(
									builder.Equals(services.CertAuthorityTypeExpr, builder.String(string(services.HostCA))),
									builder.Not(
										builder.Equals(
											services.ResourceNameExpr,
											builder.String(clusterName),
										),
									),
								).String(),
							},
						},
					},
				})
		}
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces:    []string{services.Wildcard},
					ClusterLabels: services.Labels{services.Wildcard: []string{services.Wildcard}},
					Rules: []services.Rule{
						services.NewRule(services.KindProxy, services.RW()),
						services.NewRule(services.KindOIDCRequest, services.RW()),
						services.NewRule(services.KindSSHSession, services.RW()),
						services.NewRule(services.KindSession, services.RO()),
						services.NewRule(services.KindEvent, services.RW()),
						services.NewRule(services.KindSAMLRequest, services.RW()),
						services.NewRule(services.KindOIDC, services.ReadNoSecrets()),
						services.NewRule(services.KindSAML, services.ReadNoSecrets()),
						services.NewRule(services.KindGithub, services.ReadNoSecrets()),
						services.NewRule(services.KindGithubRequest, services.RW()),
						services.NewRule(services.KindNamespace, services.RO()),
						services.NewRule(services.KindNode, services.RO()),
						services.NewRule(services.KindAuthServer, services.RO()),
						services.NewRule(services.KindReverseTunnel, services.RO()),
						services.NewRule(services.KindCertAuthority, services.ReadNoSecrets()),
						services.NewRule(services.KindUser, services.RO()),
						services.NewRule(services.KindRole, services.RO()),
						services.NewRule(services.KindClusterAuthPreference, services.RO()),
						services.NewRule(services.KindClusterConfig, services.RO()),
						services.NewRule(services.KindClusterName, services.RO()),
						services.NewRule(services.KindStaticTokens, services.RO()),
						services.NewRule(services.KindTunnelConnection, services.RW()),
						services.NewRule(services.KindRemoteCluster, services.RO()),
						services.NewRule(services.KindSemaphore, services.RW()),
						services.NewRule(services.KindAppServer, services.RO()),
						services.NewRule(services.KindWebSession, services.RW()),
						services.NewRule(services.KindWebToken, services.RW()),
						services.NewRule(services.KindKubeService, services.RW()),
						services.NewRule(types.KindDatabaseServer, services.RO()),
						// this rule allows local proxy to update the remote cluster's host certificate authorities
						// during certificates renewal
						{
							Resources: []string{services.KindCertAuthority},
							Verbs:     []string{services.VerbCreate, services.VerbRead, services.VerbUpdate},
							// allow administrative access to the certificate authority names
							// matching any cluster name except local
							Where: builder.And(
								builder.Equals(services.CertAuthorityTypeExpr, builder.String(string(services.HostCA))),
								builder.Not(
									builder.Equals(
										services.ResourceNameExpr,
										builder.String(clusterName),
									),
								),
							).String(),
						},
					},
				},
			})
	case teleport.RoleWeb:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{services.Wildcard},
					Rules: []services.Rule{
						services.NewRule(services.KindWebSession, services.RW()),
						services.NewRule(services.KindWebToken, services.RW()),
						services.NewRule(services.KindSSHSession, services.RW()),
						services.NewRule(services.KindAuthServer, services.RO()),
						services.NewRule(services.KindUser, services.RO()),
						services.NewRule(services.KindRole, services.RO()),
						services.NewRule(services.KindNamespace, services.RO()),
						services.NewRule(services.KindTrustedCluster, services.RO()),
					},
				},
			})
	case teleport.RoleSignup:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{services.Wildcard},
					Rules: []services.Rule{
						services.NewRule(services.KindAuthServer, services.RO()),
						services.NewRule(services.KindClusterAuthPreference, services.RO()),
					},
				},
			})
	case teleport.RoleAdmin:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Options: services.RoleOptions{
					MaxSessionTTL: services.MaxDuration(),
				},
				Allow: services.RoleConditions{
					Namespaces:    []string{services.Wildcard},
					Logins:        []string{},
					NodeLabels:    services.Labels{services.Wildcard: []string{services.Wildcard}},
					ClusterLabels: services.Labels{services.Wildcard: []string{services.Wildcard}},
					Rules: []services.Rule{
						services.NewRule(services.Wildcard, services.RW()),
					},
				},
			})
	case teleport.RoleNop:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{},
					Rules:      []services.Rule{},
				},
			})
	case teleport.RoleKube:
		return services.FromSpec(
			role.String(),
			services.RoleSpecV3{
				Allow: services.RoleConditions{
					Namespaces: []string{services.Wildcard},
					Rules: []services.Rule{
						services.NewRule(services.KindKubeService, services.RW()),
						services.NewRule(services.KindEvent, services.RW()),
						services.NewRule(services.KindCertAuthority, services.ReadNoSecrets()),
						services.NewRule(services.KindClusterConfig, services.RO()),
						services.NewRule(services.KindClusterAuthPreference, services.RO()),
						services.NewRule(services.KindUser, services.RO()),
						services.NewRule(services.KindRole, services.RO()),
						services.NewRule(services.KindNamespace, services.RO()),
					},
				},
			})
	}

	return nil, trace.NotFound("%q is not recognized", role.String())
}

func contextForBuiltinRole(r BuiltinRole, clusterConfig services.ClusterConfig) (*Context, error) {
	checker, err := GetCheckerForBuiltinRole(r.ClusterName, clusterConfig, r.Role)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user, err := services.NewUser(r.Username)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user.SetRoles([]string{string(r.Role)})
	return &Context{
		User:             user,
		Checker:          BuiltinRoleSet{checker},
		Identity:         r,
		UnmappedIdentity: r,
	}, nil
}

func contextForLocalUser(u LocalUser, identity services.UserGetter, access services.Access) (*Context, error) {
	// User has to be fetched to check if it's a blocked username
	user, err := identity.GetUser(u.Username, false)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	roles, traits, err := services.ExtractFromIdentity(identity, u.Identity)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	checker, err := services.FetchRoles(roles, access, traits)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// Override roles and traits from the local user based on the identity roles
	// and traits, this is done to prevent potential conflict. Imagine a scenairo
	// when SSO user has left the company, but local user entry remained with old
	// privileged roles. New user with the same name has been onboarded and would
	// have derived the roles from the stale user entry. This code prevents
	// that by extracting up to date identity traits and roles from the user's
	// certificate metadata.
	user.SetRoles(roles)
	user.SetTraits(traits)

	return &Context{
		User:             user,
		Checker:          LocalUserRoleSet{checker},
		Identity:         u,
		UnmappedIdentity: u,
	}, nil
}

type contextKey string

const (
	// ContextUser is a user set in the context of the request
	ContextUser contextKey = "teleport-user"
	// ContextClientAddr is a client address set in the context of the request
	ContextClientAddr contextKey = "client-addr"
)

// WithDelegator alias for backwards compatibility
var WithDelegator = utils.WithDelegator

// ClientUsername returns the username of a remote HTTP client making the call.
// If ctx didn't pass through auth middleware or did not come from an HTTP
// request, teleport.UserSystem is returned.
func ClientUsername(ctx context.Context) string {
	userI := ctx.Value(ContextUser)
	userWithIdentity, ok := userI.(IdentityGetter)
	if !ok {
		return teleport.UserSystem
	}
	identity := userWithIdentity.GetIdentity()
	if identity.Username == "" {
		return teleport.UserSystem
	}
	return identity.Username
}

// ClientImpersonator returns the impersonator username of a remote client
// making the call. If not present, returns an empty string
func ClientImpersonator(ctx context.Context) string {
	userI := ctx.Value(ContextUser)
	userWithIdentity, ok := userI.(IdentityGetter)
	if !ok {
		return ""
	}
	identity := userWithIdentity.GetIdentity()
	return identity.Impersonator
}

// LocalUser is a local user
type LocalUser struct {
	// Username is local username
	Username string
	// Identity is x509-derived identity used to build this user
	Identity tlsca.Identity
}

// GetIdentity returns client identity
func (l LocalUser) GetIdentity() tlsca.Identity {
	return l.Identity
}

// IdentityGetter returns the unmapped client identity.
//
// Unmapped means that if the client is a remote cluster user, the returned
// tlsca.Identity contains data from the remote cluster before role mapping is
// applied.
type IdentityGetter interface {
	// GetIdentity  returns x509-derived identity of the user
	GetIdentity() tlsca.Identity
}

// WrapIdentity wraps identity to return identity getter function
type WrapIdentity tlsca.Identity

// GetIdentity returns identity
func (i WrapIdentity) GetIdentity() tlsca.Identity {
	return tlsca.Identity(i)
}

// BuiltinRole is the role of the Teleport service.
type BuiltinRole struct {
	// GetClusterConfig fetches cluster configuration.
	GetClusterConfig GetClusterConfigFunc

	// Role is the builtin role this username is associated with
	Role teleport.Role

	// Username is for authentication tracking purposes
	Username string

	// ClusterName is the name of the local cluster
	ClusterName string

	// Identity is source x509 used to build this role
	Identity tlsca.Identity
}

// IsServer returns true if the role is one of the builtin server roles.
func (r BuiltinRole) IsServer() bool {
	return r.Role == teleport.RoleProxy ||
		r.Role == teleport.RoleNode ||
		r.Role == teleport.RoleAuth ||
		r.Role == teleport.RoleApp ||
		r.Role == teleport.RoleKube ||
		r.Role == teleport.RoleDatabase
}

// GetServerID extracts the identity from the full name. The username
// extracted from the node's identity (x.509 certificate) is expected to
// consist of "<server-id>.<cluster-name>" so strip the cluster name suffix
// to get the server id.
//
// Note that as of right now Teleport expects server id to be a UUID4 but
// older Gravity clusters used to override it with strings like
// "192_168_1_1.<cluster-name>" so this code can't rely on it being
// UUID4 to account for clusters upgraded from older versions.
func (r BuiltinRole) GetServerID() string {
	return strings.TrimSuffix(r.Identity.Username, "."+r.ClusterName)
}

// GetIdentity returns client identity
func (r BuiltinRole) GetIdentity() tlsca.Identity {
	return r.Identity
}

// BuiltinRoleSet wraps a services.RoleSet. The type is used to determine if
// the role is builtin or not.
type BuiltinRoleSet struct {
	services.RoleSet
}

// RemoteBuiltinRoleSet wraps a services.RoleSet. The type is used to determine if
// the role is a remote builtin or not.
type RemoteBuiltinRoleSet struct {
	services.RoleSet
}

// LocalUserRoleSet wraps a services.RoleSet. This type is used to determine
// if the role is a local user or not.
type LocalUserRoleSet struct {
	services.RoleSet
}

// RemoteUserRoleSet wraps a services.RoleSet. This type is used to determine
// if the role is a remote user or not.
type RemoteUserRoleSet struct {
	services.RoleSet
}

// RemoteBuiltinRole is the role of the remote (service connecting via trusted cluster link)
// Teleport service.
type RemoteBuiltinRole struct {
	// Role is the builtin role of the user
	Role teleport.Role

	// Username is for authentication tracking purposes
	Username string

	// ClusterName is the name of the remote cluster.
	ClusterName string

	// Identity is source x509 used to build this role
	Identity tlsca.Identity
}

// GetIdentity returns client identity
func (r RemoteBuiltinRole) GetIdentity() tlsca.Identity {
	return r.Identity
}

// RemoteUser defines encoded remote user.
type RemoteUser struct {
	// Username is a name of the remote user
	Username string `json:"username"`

	// ClusterName is the name of the remote cluster
	// of the user.
	ClusterName string `json:"cluster_name"`

	// RemoteRoles is optional list of remote roles
	RemoteRoles []string `json:"remote_roles"`

	// Principals is a list of Unix logins.
	Principals []string `json:"principals"`

	// KubernetesGroups is a list of Kubernetes groups
	KubernetesGroups []string `json:"kubernetes_groups"`

	// KubernetesUsers is a list of Kubernetes users
	KubernetesUsers []string `json:"kubernetes_users"`

	// DatabaseNames is a list of database names a user can connect to.
	DatabaseNames []string `json:"database_names"`

	// DatabaseUsers is a list of database users a user can connect as.
	DatabaseUsers []string `json:"database_users"`

	// Identity is source x509 used to build this role
	Identity tlsca.Identity
}

// GetIdentity returns client identity
func (r RemoteUser) GetIdentity() tlsca.Identity {
	return r.Identity
}

// GetClusterConfigFunc returns a cached services.ClusterConfig.
type GetClusterConfigFunc func(opts ...services.MarshalOption) (services.ClusterConfig, error)
