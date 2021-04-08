/*
Copyright 2017 Gravitational, Inc.

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

package srv

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/gravitational/teleport"
	apisshutils "github.com/gravitational/teleport/api/utils/sshutils"
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/sshutils"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"

	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

// AuthHandlers are common authorization and authentication related handlers
// used by the regular and forwarding server.
type AuthHandlers struct {
	*log.Entry

	// Server is the services.Server in the backend.
	Server Server

	// Component is the type of SSH server (node, proxy, or recording proxy).
	Component string

	// Emitter is event emitter
	Emitter events.Emitter

	// AccessPoint is used to access the Auth Server.
	AccessPoint auth.AccessPoint

	// FIPS mode means Teleport started in a FedRAMP/FIPS 140-2 compliant
	// configuration.
	FIPS bool

	// Clock specifies the time provider. Will be used to override the time anchor
	// for TLS certificate verification.
	// Defaults to real clock if unspecified
	Clock clockwork.Clock
}

// CreateIdentityContext returns an IdentityContext populated with information
// about the logged in user on the connection.
func (h *AuthHandlers) CreateIdentityContext(sconn *ssh.ServerConn) (IdentityContext, error) {
	identity := IdentityContext{
		TeleportUser: sconn.Permissions.Extensions[utils.CertTeleportUser],
		Login:        sconn.User(),
	}

	clusterName, err := h.AccessPoint.GetClusterName()
	if err != nil {
		return IdentityContext{}, trace.Wrap(err)
	}

	certRaw := []byte(sconn.Permissions.Extensions[utils.CertTeleportUserCertificate])
	certificate, err := sshutils.ParseCertificate(certRaw)
	if err != nil {
		return IdentityContext{}, trace.Wrap(err)
	}
	identity.Certificate = certificate
	identity.RouteToCluster = certificate.Extensions[teleport.CertExtensionTeleportRouteToCluster]
	if certificate.ValidBefore != 0 {
		identity.CertValidBefore = time.Unix(int64(certificate.ValidBefore), 0)
	}
	certAuthority, err := h.authorityForCert(services.UserCA, certificate.SignatureKey)
	if err != nil {
		return IdentityContext{}, trace.Wrap(err)
	}
	identity.CertAuthority = certAuthority

	roleSet, err := h.fetchRoleSet(certificate, certAuthority, identity.TeleportUser, clusterName.GetClusterName())
	if err != nil {
		return IdentityContext{}, trace.Wrap(err)
	}
	identity.RoleSet = roleSet
	identity.Impersonator = certificate.Extensions[teleport.CertExtensionImpersonator]

	return identity, nil
}

// CheckAgentForward checks if agent forwarding is allowed for the users RoleSet.
func (h *AuthHandlers) CheckAgentForward(ctx *ServerContext) error {
	if err := ctx.Identity.RoleSet.CheckAgentForward(ctx.Identity.Login); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// CheckPortForward checks if port forwarding is allowed for the users RoleSet.
func (h *AuthHandlers) CheckPortForward(addr string, ctx *ServerContext) error {
	if ok := ctx.Identity.RoleSet.CanPortForward(); !ok {
		systemErrorMessage := fmt.Sprintf("port forwarding not allowed by role set: %v", ctx.Identity.RoleSet)
		userErrorMessage := "port forwarding not allowed"

		// Emit port forward failure event
		if err := h.Emitter.EmitAuditEvent(h.Server.Context(), &events.PortForward{
			Metadata: events.Metadata{
				Type: events.PortForwardEvent,
				Code: events.PortForwardFailureCode,
			},
			UserMetadata: events.UserMetadata{
				Login:        ctx.Identity.Login,
				User:         ctx.Identity.TeleportUser,
				Impersonator: ctx.Identity.Impersonator,
			},
			ConnectionMetadata: events.ConnectionMetadata{
				LocalAddr:  ctx.ServerConn.LocalAddr().String(),
				RemoteAddr: ctx.ServerConn.RemoteAddr().String(),
			},
			Addr: addr,
			Status: events.Status{
				Success: false,
				Error:   systemErrorMessage,
			},
		}); err != nil {
			h.WithError(err).Warn("Failed to emit port forward deny audit event.")
		}

		h.Warnf("Port forwarding request denied: %v.", systemErrorMessage)

		return trace.AccessDenied(userErrorMessage)
	}

	return nil
}

// UserKeyAuth implements SSH client authentication using public keys and is
// called by the server every time the client connects.
func (h *AuthHandlers) UserKeyAuth(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	fingerprint := fmt.Sprintf("%v %v", key.Type(), sshutils.Fingerprint(key))

	// create a new logging entry with info specific to this login attempt
	log := h.Entry.WithField(trace.ComponentFields, log.Fields{
		"local":       conn.LocalAddr(),
		"remote":      conn.RemoteAddr(),
		"user":        conn.User(),
		"fingerprint": fingerprint,
	})

	cid := fmt.Sprintf("conn(%v->%v, user=%v)", conn.RemoteAddr(), conn.LocalAddr(), conn.User())
	log.Debugf("%v auth attempt", cid)

	cert, ok := key.(*ssh.Certificate)
	log.Debugf("%v auth attempt with key %v, %#v", cid, fingerprint, cert)
	if !ok {
		log.Debugf("auth attempt, unsupported key type")
		return nil, trace.BadParameter("unsupported key type: %v", fingerprint)
	}
	if len(cert.ValidPrincipals) == 0 {
		log.Debugf("need a valid principal for key")
		return nil, trace.BadParameter("need a valid principal for key %v", fingerprint)
	}
	if len(cert.KeyId) == 0 {
		log.Debugf("need a valid key ID for key")
		return nil, trace.BadParameter("need a valid key for key %v", fingerprint)
	}
	teleportUser := cert.KeyId

	// only failed attempts are logged right now
	recordFailedLogin := func(err error) {
		if err := h.Emitter.EmitAuditEvent(h.Server.Context(), &events.AuthAttempt{
			Metadata: events.Metadata{
				Type: events.AuthAttemptEvent,
				Code: events.AuthAttemptFailureCode,
			},
			UserMetadata: events.UserMetadata{
				Login: conn.User(),
				User:  teleportUser,
			},
			ConnectionMetadata: events.ConnectionMetadata{
				LocalAddr:  conn.LocalAddr().String(),
				RemoteAddr: conn.RemoteAddr().String(),
			},
			Status: events.Status{
				Success: false,
				Error:   err.Error(),
			},
		}); err != nil {
			h.WithError(err).Warn("Failed to emit failed login audit event.")
		}
	}

	// Check that the user certificate uses supported public key algorithms, was
	// issued by Teleport, and check the certificate metadata (principals,
	// timestamp, etc). Fallback to keys is not supported.
	clock := time.Now
	if h.Clock != nil {
		clock = h.Clock.Now
	}
	certChecker := utils.CertChecker{
		CertChecker: ssh.CertChecker{
			IsUserAuthority: h.IsUserAuthority,
			Clock:           clock,
		},
		FIPS: h.FIPS,
	}
	permissions, err := certChecker.Authenticate(conn, key)
	if err != nil {
		recordFailedLogin(err)
		return nil, trace.Wrap(err)
	}
	log.Debugf("Successfully authenticated")

	clusterName, err := h.AccessPoint.GetClusterName()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// this is the only way we know of to pass valid additional data about the
	// connection to the handlers
	permissions.Extensions[utils.CertTeleportUser] = teleportUser
	permissions.Extensions[utils.CertTeleportClusterName] = clusterName.GetClusterName()
	permissions.Extensions[utils.CertTeleportUserCertificate] = string(ssh.MarshalAuthorizedKey(cert))

	if h.isProxy() {
		return permissions, nil
	}

	// check if the user has permission to log into the node.
	switch {
	case h.Component == teleport.ComponentForwardingNode:
		err = h.canLoginWithoutRBAC(cert, clusterName.GetClusterName(), teleportUser, conn.User())
	default:
		err = h.canLoginWithRBAC(cert, clusterName.GetClusterName(), teleportUser, conn.User())
	}
	if err != nil {
		log.Errorf("Permission denied: %v", err)
		recordFailedLogin(err)
		return nil, trace.Wrap(err)
	}

	return permissions, nil
}

// HostKeyAuth implements host key verification and is called by the client
// to validate the certificate presented by the target server. If the target
// server presents a SSH certificate, we validate that it was Teleport that
// generated the certificate. If the target server presents a public key, if
// we are strictly checking keys, we reject the target server. If we are not
// we take whatever.
func (h *AuthHandlers) HostKeyAuth(addr string, remote net.Addr, key ssh.PublicKey) error {
	// Check if the given host key was signed by a Teleport certificate
	// authority (CA) or fallback to host key checking if it's allowed.
	certChecker := utils.CertChecker{
		CertChecker: ssh.CertChecker{
			IsHostAuthority: h.IsHostAuthority,
			HostKeyFallback: h.hostKeyCallback,
		},
		FIPS: h.FIPS,
	}
	err := certChecker.CheckHostKey(addr, remote, key)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// hostKeyCallback allows connections to hosts that present keys only if
// strict host key checking is disabled.
func (h *AuthHandlers) hostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	// If strict host key checking is enabled, reject host key fallback.
	clusterConfig, err := h.AccessPoint.GetClusterConfig()
	if err != nil {
		return trace.Wrap(err)
	}
	if clusterConfig.GetProxyChecksHostKeys() == services.HostKeyCheckYes {
		return trace.AccessDenied("remote host presented a public key, expected a host certificate")
	}

	// If strict host key checking is not enabled, log that Teleport trusted an
	// insecure key, but allow the request to go through.
	h.Warnf("Insecure configuration! Strict host key checking disabled, allowing login without checking host key of type %v.", key.Type())
	return nil
}

// IsUserAuthority is called during checking the client key, to see if the
// key used to sign the certificate was a Teleport CA.
func (h *AuthHandlers) IsUserAuthority(cert ssh.PublicKey) bool {
	if _, err := h.authorityForCert(services.UserCA, cert); err != nil {
		return false
	}

	return true
}

// IsHostAuthority is called when checking the host certificate a server
// presents. It make sure that the key used to sign the host certificate was a
// Teleport CA.
func (h *AuthHandlers) IsHostAuthority(cert ssh.PublicKey, address string) bool {
	if _, err := h.authorityForCert(services.HostCA, cert); err != nil {
		h.Entry.Debugf("Unable to find SSH host CA: %v.", err)
		return false
	}
	return true
}

// canLoginWithoutRBAC checks the given certificate (supplied by a connected
// client) to see if this certificate can be allowed to login as user:login
// pair to requested server.
func (h *AuthHandlers) canLoginWithoutRBAC(cert *ssh.Certificate, clusterName string, teleportUser, osUser string) error {
	h.Debugf("Checking permissions for (%v,%v) to login to node without RBAC checks.", teleportUser, osUser)

	// check if the ca that signed the certificate is known to the cluster
	_, err := h.authorityForCert(services.UserCA, cert.SignatureKey)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// canLoginWithRBAC checks the given certificate (supplied by a connected
// client) to see if this certificate can be allowed to login as user:login
// pair to requested server and if RBAC rules allow login.
func (h *AuthHandlers) canLoginWithRBAC(cert *ssh.Certificate, clusterName string, teleportUser, osUser string) error {
	h.Debugf("Checking permissions for (%v,%v) to login to node with RBAC checks.", teleportUser, osUser)

	// get the ca that signd the users certificate
	ca, err := h.authorityForCert(services.UserCA, cert.SignatureKey)
	if err != nil {
		return trace.Wrap(err)
	}

	// get roles assigned to this user
	roles, err := h.fetchRoleSet(cert, ca, teleportUser, clusterName)
	if err != nil {
		return trace.Wrap(err)
	}

	ap, err := h.AccessPoint.GetAuthPreference()
	if err != nil {
		return trace.Wrap(err)
	}
	_, mfaVerified := cert.Extensions[teleport.CertExtensionMFAVerified]
	mfaParams := services.AccessMFAParams{
		Verified:       mfaVerified,
		AlwaysRequired: ap.GetRequireSessionMFA(),
	}

	// check if roles allow access to server
	if err := roles.CheckAccessToServer(osUser, h.Server.GetInfo(), mfaParams); err != nil {
		return trace.AccessDenied("user %s@%s is not authorized to login as %v@%s: %v",
			teleportUser, ca.GetClusterName(), osUser, clusterName, err)
	}

	return nil
}

// fetchRoleSet fetches the services.RoleSet assigned to a Teleport user.
func (h *AuthHandlers) fetchRoleSet(cert *ssh.Certificate, ca services.CertAuthority, teleportUser string, clusterName string) (services.RoleSet, error) {
	// for local users, go and check their individual permissions
	var roleset services.RoleSet
	if clusterName == ca.GetClusterName() {
		// Extract roles and traits either from the certificate or from
		// services.User and create a services.RoleSet with all runtime roles.
		roles, traits, err := services.ExtractFromCertificate(h.AccessPoint, cert)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		roleset, err = services.FetchRoles(roles, h.AccessPoint, traits)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	} else {
		roles, err := extractRolesFromCert(cert)
		if err != nil {
			return nil, trace.AccessDenied("failed to parse certificate roles")
		}
		roleNames, err := services.MapRoles(ca.CombinedMapping(), roles)
		if err != nil {
			return nil, trace.AccessDenied("failed to map roles")
		}
		// Pass the principals on the certificate along as the login traits
		// to the remote cluster.
		traits := map[string][]string{
			teleport.TraitLogins: cert.ValidPrincipals,
		}
		roleset, err = services.FetchRoles(roleNames, h.AccessPoint, traits)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	return roleset, nil
}

// authorityForCert checks if the certificate was signed by a Teleport
// Certificate Authority and returns it.
func (h *AuthHandlers) authorityForCert(caType services.CertAuthType, key ssh.PublicKey) (services.CertAuthority, error) {
	// get all certificate authorities for given type
	cas, err := h.AccessPoint.GetCertAuthorities(caType, false)
	if err != nil {
		h.Warnf("%v", trace.DebugReport(err))
		return nil, trace.Wrap(err)
	}

	// find the one that signed our certificate
	var ca services.CertAuthority
	for i := range cas {
		checkers, err := sshutils.GetCheckers(cas[i])
		if err != nil {
			h.Warnf("%v", err)
			return nil, trace.Wrap(err)
		}
		for _, checker := range checkers {
			// if we have a certificate, compare the certificate signing key against
			// the ca key. otherwise check the public key that was passed in. this is
			// due to the differences in how this function is called by the user and
			// host checkers.
			switch v := key.(type) {
			case *ssh.Certificate:
				if apisshutils.KeysEqual(v.SignatureKey, checker) {
					ca = cas[i]
					break
				}
			default:
				if apisshutils.KeysEqual(key, checker) {
					ca = cas[i]
					break
				}
			}
		}
	}

	// the certificate was signed by unknown authority
	if ca == nil {
		return nil, trace.AccessDenied("the certificate signed by untrusted CA")
	}

	return ca, nil
}

// isProxy returns true if it's a regular SSH proxy.
func (h *AuthHandlers) isProxy() bool {
	return h.Component == teleport.ComponentProxy
}

// extractRolesFromCert extracts roles from certificate metadata extensions.
func extractRolesFromCert(cert *ssh.Certificate) ([]string, error) {
	data, ok := cert.Extensions[teleport.CertExtensionTeleportRoles]
	if !ok {
		// it's ok to not have any roles in the metadata
		return nil, nil
	}
	return services.UnmarshalCertRoles(data)
}
