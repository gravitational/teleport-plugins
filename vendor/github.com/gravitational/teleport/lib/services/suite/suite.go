/*
Copyright 2015-2019 Gravitational, Inc.

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

package suite

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/fixtures"
	"github.com/gravitational/teleport/lib/jwt"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/tlsca"

	"github.com/gravitational/trace"

	"github.com/jonboulle/clockwork"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/tstranex/u2f"
	"gopkg.in/check.v1"
)

var _ = fmt.Printf

// NewTestCA returns new test authority with a test key as a public and
// signing key
func NewTestCA(caType services.CertAuthType, clusterName string, privateKeys ...[]byte) *services.CertAuthorityV2 {
	// privateKeys is to specify another RSA private key
	if len(privateKeys) == 0 {
		privateKeys = [][]byte{fixtures.PEMBytes["rsa"]}
	}
	keyBytes := privateKeys[0]
	rsaKey, err := ssh.ParseRawPrivateKey(keyBytes)
	if err != nil {
		panic(err)
	}

	signer, err := ssh.NewSignerFromKey(rsaKey)
	if err != nil {
		panic(err)
	}

	key, cert, err := tlsca.GenerateSelfSignedCAWithPrivateKey(rsaKey.(*rsa.PrivateKey), pkix.Name{
		CommonName:   clusterName,
		Organization: []string{clusterName},
	}, nil, defaults.CATTL)
	if err != nil {
		panic(err)
	}

	publicKey, privateKey, err := jwt.GenerateKeyPair()
	if err != nil {
		panic(err)
	}

	return &services.CertAuthorityV2{
		Kind:    services.KindCertAuthority,
		SubKind: string(caType),
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      clusterName,
			Namespace: defaults.Namespace,
		},
		Spec: services.CertAuthoritySpecV2{
			Type:         caType,
			ClusterName:  clusterName,
			CheckingKeys: [][]byte{ssh.MarshalAuthorizedKey(signer.PublicKey())},
			SigningKeys:  [][]byte{keyBytes},
			TLSKeyPairs:  []services.TLSKeyPair{{Cert: cert, Key: key}},
			JWTKeyPairs: []services.JWTKeyPair{
				{
					PublicKey:  publicKey,
					PrivateKey: privateKey,
				},
			},
		},
	}
}

// ServicesTestSuite is an acceptance test suite
// for services. It is used for local implementations and implementations
// using GRPC to guarantee consistency between local and remote services
type ServicesTestSuite struct {
	Access          services.Access
	CAS             services.Trust
	PresenceS       services.Presence
	ProvisioningS   services.Provisioner
	WebS            services.Identity
	ConfigS         services.ClusterConfiguration
	EventsS         services.Events
	UsersS          services.UsersService
	ChangesC        chan interface{}
	Clock           clockwork.FakeClock
	NewProxyWatcher services.NewProxyWatcherFunc
}

func (s *ServicesTestSuite) Users() services.UsersService {
	if s.WebS != nil {
		return s.WebS
	}
	return s.UsersS
}

func userSlicesEqual(c *check.C, a []services.User, b []services.User) {
	comment := check.Commentf("a: %#v b: %#v", a, b)
	c.Assert(len(a), check.Equals, len(b), comment)
	sort.Sort(services.Users(a))
	sort.Sort(services.Users(b))
	for i := range a {
		usersEqual(c, a[i], b[i])
	}
}

func usersEqual(c *check.C, a services.User, b services.User) {
	comment := check.Commentf("a: %#v b: %#v", a, b)
	c.Assert(a.Equals(b), check.Equals, true, comment)
}

func newUser(name string, roles []string) services.User {
	return &services.UserV2{
		Kind:    services.KindUser,
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      name,
			Namespace: defaults.Namespace,
		},
		Spec: services.UserSpecV2{
			Roles: roles,
		},
	}
}

func (s *ServicesTestSuite) UsersCRUD(c *check.C) {
	u, err := s.WebS.GetUsers(false)
	c.Assert(err, check.IsNil)
	c.Assert(len(u), check.Equals, 0)

	c.Assert(s.WebS.UpsertPasswordHash("user1", []byte("hash")), check.IsNil)
	c.Assert(s.WebS.UpsertPasswordHash("user2", []byte("hash2")), check.IsNil)

	u, err = s.WebS.GetUsers(false)
	c.Assert(err, check.IsNil)
	userSlicesEqual(c, u, []services.User{newUser("user1", nil), newUser("user2", nil)})

	out, err := s.WebS.GetUser("user1", false)
	c.Assert(err, check.IsNil)
	usersEqual(c, out, u[0])

	user := newUser("user1", []string{"admin", "user"})
	c.Assert(s.WebS.UpsertUser(user), check.IsNil)

	out, err = s.WebS.GetUser("user1", false)
	c.Assert(err, check.IsNil)
	usersEqual(c, out, user)

	out, err = s.WebS.GetUser("user1", false)
	c.Assert(err, check.IsNil)
	usersEqual(c, out, user)

	c.Assert(s.WebS.DeleteUser(context.TODO(), "user1"), check.IsNil)

	u, err = s.WebS.GetUsers(false)
	c.Assert(err, check.IsNil)
	userSlicesEqual(c, u, []services.User{newUser("user2", nil)})

	err = s.WebS.DeleteUser(context.TODO(), "user1")
	fixtures.ExpectNotFound(c, err)

	// bad username
	err = s.WebS.UpsertUser(newUser("", nil))
	fixtures.ExpectBadParameter(c, err)
}

func (s *ServicesTestSuite) UsersExpiry(c *check.C) {
	expiresAt := s.Clock.Now().Add(1 * time.Minute)

	err := s.WebS.UpsertUser(&services.UserV2{
		Kind:    services.KindUser,
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      "foo",
			Namespace: defaults.Namespace,
			Expires:   &expiresAt,
		},
		Spec: services.UserSpecV2{},
	})
	c.Assert(err, check.IsNil)

	// Make sure the user exists.
	u, err := s.WebS.GetUser("foo", false)
	c.Assert(err, check.IsNil)
	c.Assert(u.GetName(), check.Equals, "foo")

	s.Clock.Advance(2 * time.Minute)

	// Make sure the user is now gone.
	_, err = s.WebS.GetUser("foo", false)
	c.Assert(err, check.NotNil)
}

func (s *ServicesTestSuite) LoginAttempts(c *check.C) {
	user := newUser("user1", []string{"admin", "user"})
	c.Assert(s.WebS.UpsertUser(user), check.IsNil)

	attempts, err := s.WebS.GetUserLoginAttempts(user.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(len(attempts), check.Equals, 0)

	clock := clockwork.NewFakeClock()
	attempt1 := services.LoginAttempt{Time: clock.Now().UTC(), Success: false}
	err = s.WebS.AddUserLoginAttempt(user.GetName(), attempt1, defaults.AttemptTTL)
	c.Assert(err, check.IsNil)

	attempt2 := services.LoginAttempt{Time: clock.Now().UTC(), Success: false}
	err = s.WebS.AddUserLoginAttempt(user.GetName(), attempt2, defaults.AttemptTTL)
	c.Assert(err, check.IsNil)

	attempts, err = s.WebS.GetUserLoginAttempts(user.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(attempts, check.DeepEquals, []services.LoginAttempt{attempt1, attempt2})
	c.Assert(services.LastFailed(3, attempts), check.Equals, false)
	c.Assert(services.LastFailed(2, attempts), check.Equals, true)
}

func (s *ServicesTestSuite) CertAuthCRUD(c *check.C) {
	ca := NewTestCA(services.UserCA, "example.com")
	c.Assert(s.CAS.UpsertCertAuthority(ca), check.IsNil)

	out, err := s.CAS.GetCertAuthority(ca.GetID(), true)
	c.Assert(err, check.IsNil)
	ca.SetResourceID(out.GetResourceID())
	fixtures.DeepCompare(c, out, ca)

	cas, err := s.CAS.GetCertAuthorities(services.UserCA, false)
	c.Assert(err, check.IsNil)
	ca2 := *ca
	ca2.Spec.SigningKeys = nil
	ca2.Spec.TLSKeyPairs = []services.TLSKeyPair{{Cert: ca2.Spec.TLSKeyPairs[0].Cert}}
	ca2.Spec.JWTKeyPairs = []services.JWTKeyPair{{PublicKey: ca2.Spec.JWTKeyPairs[0].PublicKey}}
	fixtures.DeepCompare(c, cas[0], &ca2)

	cas, err = s.CAS.GetCertAuthorities(services.UserCA, true)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, cas[0], ca)

	cas, err = s.CAS.GetCertAuthorities(services.UserCA, true, services.SkipValidation())
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, cas[0], ca)

	err = s.CAS.DeleteCertAuthority(*ca.ID())
	c.Assert(err, check.IsNil)

	// test compare and swap
	ca = NewTestCA(services.UserCA, "example.com")
	c.Assert(s.CAS.CreateCertAuthority(ca), check.IsNil)

	clock := clockwork.NewFakeClock()
	newCA := *ca
	rotation := services.Rotation{
		State:       services.RotationStateInProgress,
		CurrentID:   "id1",
		GracePeriod: services.NewDuration(time.Hour),
		Started:     clock.Now(),
	}
	newCA.SetRotation(rotation)

	err = s.CAS.CompareAndSwapCertAuthority(&newCA, ca)
	c.Assert(err, check.IsNil)

	out, err = s.CAS.GetCertAuthority(ca.GetID(), true)
	c.Assert(err, check.IsNil)
	newCA.SetResourceID(out.GetResourceID())
	fixtures.DeepCompare(c, &newCA, out)
}

// NewServer creates a new server resource
func NewServer(kind, name, addr, namespace string) *services.ServerV2 {
	return &services.ServerV2{
		Kind:    kind,
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		Spec: services.ServerSpecV2{
			Addr:       addr,
			PublicAddr: addr,
		},
	}
}

func (s *ServicesTestSuite) ServerCRUD(c *check.C) {
	ctx := context.TODO()
	// SSH service.
	out, err := s.PresenceS.GetNodes(defaults.Namespace)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	srv := NewServer(services.KindNode, "srv1", "127.0.0.1:2022", defaults.Namespace)
	_, err = s.PresenceS.UpsertNode(srv)
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetNodes(srv.Metadata.Namespace)
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 1)
	srv.SetResourceID(out[0].GetResourceID())
	fixtures.DeepCompare(c, out, []services.Server{srv})

	err = s.PresenceS.DeleteNode(srv.Metadata.Namespace, srv.GetName())
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetNodes(srv.Metadata.Namespace)
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 0)

	// Proxy service.
	out, err = s.PresenceS.GetProxies()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	proxy := NewServer(services.KindProxy, "proxy1", "127.0.0.1:2023", defaults.Namespace)
	c.Assert(s.PresenceS.UpsertProxy(proxy), check.IsNil)

	out, err = s.PresenceS.GetProxies()
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 1)
	proxy.SetResourceID(out[0].GetResourceID())
	c.Assert(out, check.DeepEquals, []services.Server{proxy})

	err = s.PresenceS.DeleteProxy(proxy.GetName())
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetProxies()
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 0)

	// Auth service.
	out, err = s.PresenceS.GetAuthServers()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	auth := NewServer(services.KindAuthServer, "auth1", "127.0.0.1:2025", defaults.Namespace)
	c.Assert(s.PresenceS.UpsertAuthServer(auth), check.IsNil)

	out, err = s.PresenceS.GetAuthServers()
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 1)
	auth.SetResourceID(out[0].GetResourceID())
	c.Assert(out, check.DeepEquals, []services.Server{auth})

	// Kubernetes service.
	out, err = s.PresenceS.GetKubeServices(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	kube1 := NewServer(services.KindKubeService, "kube1", "10.0.0.1:3026", defaults.Namespace)
	c.Assert(s.PresenceS.UpsertKubeService(ctx, kube1), check.IsNil)
	kube2 := NewServer(services.KindKubeService, "kube2", "10.0.0.2:3026", defaults.Namespace)
	c.Assert(s.PresenceS.UpsertKubeService(ctx, kube2), check.IsNil)

	out, err = s.PresenceS.GetKubeServices(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 2)
	kube1.SetResourceID(out[0].GetResourceID())
	kube2.SetResourceID(out[1].GetResourceID())
	c.Assert(out, check.DeepEquals, []services.Server{kube1, kube2})

	c.Assert(s.PresenceS.DeleteKubeService(ctx, kube1.GetName()), check.IsNil)
	out, err = s.PresenceS.GetKubeServices(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 1)
	c.Assert(out, check.DeepEquals, []services.Server{kube2})

	c.Assert(s.PresenceS.DeleteAllKubeServices(ctx), check.IsNil)
	out, err = s.PresenceS.GetKubeServices(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 0)
}

// NewAppServer creates a new application server resource.
func NewAppServer(name string, internalAddr string, publicAddr string) *services.ServerV2 {
	return &services.ServerV2{
		Kind:    services.KindAppServer,
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      uuid.New(),
			Namespace: defaults.Namespace,
		},
		Spec: services.ServerSpecV2{
			Apps: []*services.App{
				&services.App{
					Name:       name,
					URI:        internalAddr,
					PublicAddr: publicAddr,
				},
			},
		},
	}
}

// AppServerCRUD tests CRUD functionality for services.Server.
func (s *ServicesTestSuite) AppServerCRUD(c *check.C) {
	ctx := context.Background()

	// Create application.
	server := NewAppServer("foo", "http://127.0.0.1:8080", "foo.example.com")

	// Expect not to be returned any applications and trace.NotFound.
	out, err := s.PresenceS.GetAppServers(ctx, defaults.Namespace)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	// Upsert application.
	_, err = s.PresenceS.UpsertAppServer(ctx, server)
	c.Assert(err, check.IsNil)

	// Check again, expect a single application to be found.
	out, err = s.PresenceS.GetAppServers(ctx, server.GetNamespace())
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 1)
	server.SetResourceID(out[0].GetResourceID())
	fixtures.DeepCompare(c, out, []services.Server{server})

	// Remove the application.
	err = s.PresenceS.DeleteAppServer(ctx, server.Metadata.Namespace, server.GetName())
	c.Assert(err, check.IsNil)

	// Now expect no applications to be returned.
	out, err = s.PresenceS.GetAppServers(ctx, server.Metadata.Namespace)
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 0)
}

func newReverseTunnel(clusterName string, dialAddrs []string) *services.ReverseTunnelV2 {
	return &services.ReverseTunnelV2{
		Kind:    services.KindReverseTunnel,
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      clusterName,
			Namespace: defaults.Namespace,
		},
		Spec: services.ReverseTunnelSpecV2{
			ClusterName: clusterName,
			DialAddrs:   dialAddrs,
		},
	}
}

func (s *ServicesTestSuite) ReverseTunnelsCRUD(c *check.C) {
	out, err := s.PresenceS.GetReverseTunnels()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	tunnel := newReverseTunnel("example.com", []string{"example.com:2023"})
	c.Assert(s.PresenceS.UpsertReverseTunnel(tunnel), check.IsNil)

	out, err = s.PresenceS.GetReverseTunnels()
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 1)
	tunnel.SetResourceID(out[0].GetResourceID())
	fixtures.DeepCompare(c, out, []services.ReverseTunnel{tunnel})

	err = s.PresenceS.DeleteReverseTunnel(tunnel.Spec.ClusterName)
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetReverseTunnels()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	err = s.PresenceS.UpsertReverseTunnel(newReverseTunnel("", []string{"127.0.0.1:1234"}))
	fixtures.ExpectBadParameter(c, err)

	err = s.PresenceS.UpsertReverseTunnel(newReverseTunnel("example.com", []string{""}))
	fixtures.ExpectBadParameter(c, err)

	err = s.PresenceS.UpsertReverseTunnel(newReverseTunnel("example.com", []string{}))
	fixtures.ExpectBadParameter(c, err)
}

func (s *ServicesTestSuite) PasswordHashCRUD(c *check.C) {
	_, err := s.WebS.GetPasswordHash("user1")
	c.Assert(trace.IsNotFound(err), check.Equals, true, check.Commentf("%#v", err))

	err = s.WebS.UpsertPasswordHash("user1", []byte("hello123"))
	c.Assert(err, check.IsNil)

	hash, err := s.WebS.GetPasswordHash("user1")
	c.Assert(err, check.IsNil)
	c.Assert(hash, check.DeepEquals, []byte("hello123"))

	err = s.WebS.UpsertPasswordHash("user1", []byte("hello321"))
	c.Assert(err, check.IsNil)

	hash, err = s.WebS.GetPasswordHash("user1")
	c.Assert(err, check.IsNil)
	c.Assert(hash, check.DeepEquals, []byte("hello321"))
}

func (s *ServicesTestSuite) WebSessionCRUD(c *check.C) {
	_, err := s.WebS.GetWebSession("user1", "sid1")
	c.Assert(trace.IsNotFound(err), check.Equals, true, check.Commentf("%#v", err))

	dt := time.Date(2015, 6, 5, 4, 3, 2, 1, time.UTC).UTC()
	ws := services.NewWebSession("sid1", services.KindWebSession, services.KindWebSession,
		services.WebSessionSpecV2{
			Pub:     []byte("pub123"),
			Priv:    []byte("priv123"),
			Expires: dt,
		})
	err = s.WebS.UpsertWebSession("user1", "sid1", ws)
	c.Assert(err, check.IsNil)

	out, err := s.WebS.GetWebSession("user1", "sid1")
	c.Assert(err, check.IsNil)
	c.Assert(out, check.DeepEquals, ws)

	ws1 := services.NewWebSession("sid1", services.KindWebSession, services.KindWebSession,
		services.WebSessionSpecV2{
			Pub:     []byte("pub321"),
			Priv:    []byte("priv321"),
			Expires: dt,
		})
	err = s.WebS.UpsertWebSession("user1", "sid1", ws1)
	c.Assert(err, check.IsNil)

	out2, err := s.WebS.GetWebSession("user1", "sid1")
	c.Assert(err, check.IsNil)
	c.Assert(out2, check.DeepEquals, ws1)

	c.Assert(s.WebS.DeleteWebSession("user1", "sid1"), check.IsNil)

	_, err = s.WebS.GetWebSession("user1", "sid1")
	fixtures.ExpectNotFound(c, err)
}

func (s *ServicesTestSuite) TokenCRUD(c *check.C) {
	_, err := s.ProvisioningS.GetToken("token")
	fixtures.ExpectNotFound(c, err)

	t, err := services.NewProvisionToken("token", teleport.Roles{teleport.RoleAuth, teleport.RoleNode}, time.Time{})
	c.Assert(err, check.IsNil)

	c.Assert(s.ProvisioningS.UpsertToken(t), check.IsNil)

	token, err := s.ProvisioningS.GetToken("token")
	c.Assert(err, check.IsNil)
	c.Assert(token.GetRoles().Include(teleport.RoleAuth), check.Equals, true)
	c.Assert(token.GetRoles().Include(teleport.RoleNode), check.Equals, true)
	c.Assert(token.GetRoles().Include(teleport.RoleProxy), check.Equals, false)
	diff := time.Now().UTC().Add(defaults.ProvisioningTokenTTL).Second() - token.Expiry().Second()
	if diff > 1 {
		c.Fatalf("expected diff to be within one second, got %v instead", diff)
	}

	c.Assert(s.ProvisioningS.DeleteToken("token"), check.IsNil)

	_, err = s.ProvisioningS.GetToken("token")
	fixtures.ExpectNotFound(c, err)

	// check tokens backwards compatibility and marshal/unmarshal
	expiry := time.Now().UTC().Add(time.Hour)
	v1 := &services.ProvisionTokenV1{
		Token:   "old",
		Roles:   teleport.Roles{teleport.RoleNode, teleport.RoleProxy},
		Expires: expiry,
	}
	v2, err := services.NewProvisionToken(v1.Token, v1.Roles, expiry)
	c.Assert(err, check.IsNil)

	// Tokens in different version formats are backwards and forwards
	// compatible
	fixtures.DeepCompare(c, v1.V2(), v2)
	fixtures.DeepCompare(c, v2.V1(), v1)

	// Marshal V1, unmarshal V2
	data, err := services.MarshalProvisionToken(v2, services.WithVersion(services.V1))
	c.Assert(err, check.IsNil)

	out, err := services.UnmarshalProvisionToken(data)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, out, v2)

	// Test delete all tokens
	t, err = services.NewProvisionToken("token1", teleport.Roles{teleport.RoleAuth, teleport.RoleNode}, time.Time{})
	c.Assert(err, check.IsNil)
	c.Assert(s.ProvisioningS.UpsertToken(t), check.IsNil)

	t, err = services.NewProvisionToken("token2", teleport.Roles{teleport.RoleAuth, teleport.RoleNode}, time.Time{})
	c.Assert(err, check.IsNil)
	c.Assert(s.ProvisioningS.UpsertToken(t), check.IsNil)

	tokens, err := s.ProvisioningS.GetTokens()
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 2)

	err = s.ProvisioningS.DeleteAllTokens()
	c.Assert(err, check.IsNil)

	tokens, err = s.ProvisioningS.GetTokens()
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 0)
}

func (s *ServicesTestSuite) RolesCRUD(c *check.C) {
	out, err := s.Access.GetRoles()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	role := services.RoleV3{
		Kind:    services.KindRole,
		Version: services.V3,
		Metadata: services.Metadata{
			Name:      "role1",
			Namespace: defaults.Namespace,
		},
		Spec: services.RoleSpecV3{
			Options: services.RoleOptions{
				MaxSessionTTL:     services.Duration(time.Hour),
				PortForwarding:    services.NewBoolOption(true),
				CertificateFormat: teleport.CertificateFormatStandard,
				BPF:               defaults.EnhancedEvents(),
			},
			Allow: services.RoleConditions{
				Logins:           []string{"root", "bob"},
				NodeLabels:       services.Labels{services.Wildcard: []string{services.Wildcard}},
				AppLabels:        services.Labels{services.Wildcard: []string{services.Wildcard}},
				KubernetesLabels: services.Labels{services.Wildcard: []string{services.Wildcard}},
				Namespaces:       []string{defaults.Namespace},
				Rules: []services.Rule{
					services.NewRule(services.KindRole, services.RO()),
				},
			},
			Deny: services.RoleConditions{
				Namespaces: []string{defaults.Namespace},
			},
		},
	}
	ctx := context.Background()
	err = s.Access.UpsertRole(ctx, &role)
	c.Assert(err, check.IsNil)
	rout, err := s.Access.GetRole(role.Metadata.Name)
	c.Assert(err, check.IsNil)
	role.SetResourceID(rout.GetResourceID())
	fixtures.DeepCompare(c, rout, &role)

	role.Spec.Allow.Logins = []string{"bob"}
	err = s.Access.UpsertRole(ctx, &role)
	c.Assert(err, check.IsNil)
	rout, err = s.Access.GetRole(role.Metadata.Name)
	c.Assert(err, check.IsNil)
	role.SetResourceID(rout.GetResourceID())
	c.Assert(rout, check.DeepEquals, &role)

	err = s.Access.DeleteRole(ctx, role.Metadata.Name)
	c.Assert(err, check.IsNil)

	_, err = s.Access.GetRole(role.Metadata.Name)
	fixtures.ExpectNotFound(c, err)
}

func (s *ServicesTestSuite) NamespacesCRUD(c *check.C) {
	out, err := s.PresenceS.GetNamespaces()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	ns := services.Namespace{
		Kind:    services.KindNamespace,
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      defaults.Namespace,
			Namespace: defaults.Namespace,
		},
	}
	err = s.PresenceS.UpsertNamespace(ns)
	c.Assert(err, check.IsNil)
	nsout, err := s.PresenceS.GetNamespace(ns.Metadata.Name)
	c.Assert(err, check.IsNil)
	c.Assert(nsout, check.DeepEquals, &ns)

	err = s.PresenceS.DeleteNamespace(ns.Metadata.Name)
	c.Assert(err, check.IsNil)

	_, err = s.PresenceS.GetNamespace(ns.Metadata.Name)
	fixtures.ExpectNotFound(c, err)
}

func (s *ServicesTestSuite) U2FCRUD(c *check.C) {
	token := "tok1"
	appID := "https://localhost"
	user1 := "user1"

	challenge, err := u2f.NewChallenge(appID, []string{appID})
	c.Assert(err, check.IsNil)

	err = s.WebS.UpsertU2FRegisterChallenge(token, challenge)
	c.Assert(err, check.IsNil)

	challengeOut, err := s.WebS.GetU2FRegisterChallenge(token)
	c.Assert(err, check.IsNil)
	c.Assert(challenge.Challenge, check.DeepEquals, challengeOut.Challenge)
	c.Assert(challenge.Timestamp.Unix(), check.Equals, challengeOut.Timestamp.Unix())
	c.Assert(challenge.AppID, check.Equals, challengeOut.AppID)
	c.Assert(challenge.TrustedFacets, check.DeepEquals, challengeOut.TrustedFacets)

	err = s.WebS.UpsertU2FSignChallenge(user1, challenge)
	c.Assert(err, check.IsNil)

	challengeOut, err = s.WebS.GetU2FSignChallenge(user1)
	c.Assert(err, check.IsNil)
	c.Assert(challenge.Challenge, check.DeepEquals, challengeOut.Challenge)
	c.Assert(challenge.Timestamp.Unix(), check.Equals, challengeOut.Timestamp.Unix())
	c.Assert(challenge.AppID, check.Equals, challengeOut.AppID)
	c.Assert(challenge.TrustedFacets, check.DeepEquals, challengeOut.TrustedFacets)

	derKey, err := base64.StdEncoding.DecodeString("MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEGOi54Eun0r3Xrj8PjyOGYzJObENYI/t/Lr9g9PsHTHnp1qI2ysIhsdMPd7x/vpsL6cr+2EPVik7921OSsVjEMw==")
	c.Assert(err, check.IsNil)
	pubkeyInterface, err := x509.ParsePKIXPublicKey(derKey)
	c.Assert(err, check.IsNil)

	pubkey, ok := pubkeyInterface.(*ecdsa.PublicKey)
	c.Assert(ok, check.Equals, true)

	registration := u2f.Registration{
		Raw:       []byte("BQQY6LngS6fSvdeuPw+PI4ZjMk5sQ1gj+38uv2D0+wdMeenWojbKwiGx0w93vH++mwvpyv7YQ9WKTv3bU5KxWMQzQIJ+PVFsYjEa0Xgnx+siQaxdlku+U+J2W55U5NrN1iGIc0Amh+0HwhbV2W90G79cxIYS2SVIFAdqTTDXvPXJbeAwggE8MIHkoAMCAQICChWIR0AwlYJZQHcwCgYIKoZIzj0EAwIwFzEVMBMGA1UEAxMMRlQgRklETyAwMTAwMB4XDTE0MDgxNDE4MjkzMloXDTI0MDgxNDE4MjkzMlowMTEvMC0GA1UEAxMmUGlsb3RHbnViYnktMC40LjEtMTU4ODQ3NDAzMDk1ODI1OTQwNzcwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQY6LngS6fSvdeuPw+PI4ZjMk5sQ1gj+38uv2D0+wdMeenWojbKwiGx0w93vH++mwvpyv7YQ9WKTv3bU5KxWMQzMAoGCCqGSM49BAMCA0cAMEQCIIbmYKu6I2L4pgZCBms9NIo9yo5EO9f2irp0ahvLlZudAiC8RN/N+WHAFdq8Z+CBBOMsRBFDDJy3l5EDR83B5GAfrjBEAiBl6R6gAmlbudVpW2jSn3gfjmA8EcWq0JsGZX9oFM/RJwIgb9b01avBY5jBeVIqw5KzClLzbRDMY4K+Ds6uprHyA1Y="),
		KeyHandle: []byte("gn49UWxiMRrReCfH6yJBrF2WS75T4nZbnlTk2s3WIYhzQCaH7QfCFtXZb3Qbv1zEhhLZJUgUB2pNMNe89clt4A=="),
		PubKey:    *pubkey,
	}
	err = s.WebS.UpsertU2FRegistration(user1, &registration)
	c.Assert(err, check.IsNil)

	registrationOut, err := s.WebS.GetU2FRegistration(user1)
	c.Assert(err, check.IsNil)
	c.Assert(&registration, check.DeepEquals, registrationOut)
}

func (s *ServicesTestSuite) SAMLCRUD(c *check.C) {
	connector := &services.SAMLConnectorV2{
		Kind:    services.KindSAML,
		Version: services.V2,
		Metadata: services.Metadata{
			Name:      "saml1",
			Namespace: defaults.Namespace,
		},
		Spec: services.SAMLConnectorSpecV2{
			Issuer:                   "http://example.com",
			SSO:                      "https://example.com/saml/sso",
			AssertionConsumerService: "https://localhost/acs",
			Audience:                 "https://localhost/aud",
			ServiceProviderIssuer:    "https://localhost/iss",
			AttributesToRoles: []services.AttributeMapping{
				{Name: "groups", Value: "admin", Roles: []string{"admin"}},
			},
			Cert: fixtures.SigningCertPEM,
			SigningKeyPair: &services.SigningKeyPair{
				PrivateKey: fixtures.SigningKeyPEM,
				Cert:       fixtures.SigningCertPEM,
			},
		},
	}
	err := connector.CheckAndSetDefaults()
	c.Assert(err, check.IsNil)
	err = s.WebS.UpsertSAMLConnector(connector)
	c.Assert(err, check.IsNil)
	out, err := s.WebS.GetSAMLConnector(connector.GetName(), true)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, out, connector)

	connectors, err := s.WebS.GetSAMLConnectors(true)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, []services.SAMLConnector{connector}, connectors)

	out2, err := s.WebS.GetSAMLConnector(connector.GetName(), false)
	c.Assert(err, check.IsNil)
	connectorNoSecrets := *connector
	connectorNoSecrets.Spec.SigningKeyPair.PrivateKey = ""
	fixtures.DeepCompare(c, out2, &connectorNoSecrets)

	connectorsNoSecrets, err := s.WebS.GetSAMLConnectors(false)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, []services.SAMLConnector{&connectorNoSecrets}, connectorsNoSecrets)

	err = s.WebS.DeleteSAMLConnector(connector.GetName())
	c.Assert(err, check.IsNil)

	err = s.WebS.DeleteSAMLConnector(connector.GetName())
	c.Assert(trace.IsNotFound(err), check.Equals, true, check.Commentf("expected not found, got %T", err))

	_, err = s.WebS.GetSAMLConnector(connector.GetName(), true)
	c.Assert(trace.IsNotFound(err), check.Equals, true, check.Commentf("expected not found, got %T", err))
}

func (s *ServicesTestSuite) TunnelConnectionsCRUD(c *check.C) {
	clusterName := "example.com"
	out, err := s.PresenceS.GetTunnelConnections(clusterName)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	dt := time.Date(2015, 6, 5, 4, 3, 2, 1, time.UTC).UTC()
	conn, err := services.NewTunnelConnection("conn1", services.TunnelConnectionSpecV2{
		ClusterName:   clusterName,
		ProxyName:     "p1",
		LastHeartbeat: dt,
	})
	c.Assert(err, check.IsNil)

	err = s.PresenceS.UpsertTunnelConnection(conn)
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetTunnelConnections(clusterName)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 1)
	conn.SetResourceID(out[0].GetResourceID())
	fixtures.DeepCompare(c, out[0], conn)

	out, err = s.PresenceS.GetAllTunnelConnections()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 1)
	fixtures.DeepCompare(c, out[0], conn)

	dt = dt.Add(time.Hour)
	conn.SetLastHeartbeat(dt)

	err = s.PresenceS.UpsertTunnelConnection(conn)
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetTunnelConnections(clusterName)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 1)
	conn.SetResourceID(out[0].GetResourceID())
	fixtures.DeepCompare(c, out[0], conn)

	err = s.PresenceS.DeleteAllTunnelConnections()
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetTunnelConnections(clusterName)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	err = s.PresenceS.DeleteAllTunnelConnections()
	c.Assert(err, check.IsNil)

	// test delete individual connection
	err = s.PresenceS.UpsertTunnelConnection(conn)
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetTunnelConnections(clusterName)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 1)
	conn.SetResourceID(out[0].GetResourceID())
	fixtures.DeepCompare(c, out[0], conn)

	err = s.PresenceS.DeleteTunnelConnection(clusterName, conn.GetName())
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetTunnelConnections(clusterName)
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)
}

func (s *ServicesTestSuite) GithubConnectorCRUD(c *check.C) {
	connector := &services.GithubConnectorV3{
		Kind:    services.KindGithubConnector,
		Version: services.V3,
		Metadata: services.Metadata{
			Name:      "github",
			Namespace: defaults.Namespace,
		},
		Spec: services.GithubConnectorSpecV3{
			ClientID:     "aaa",
			ClientSecret: "bbb",
			RedirectURL:  "https://localhost:3080/v1/webapi/github/callback",
			Display:      "Github",
			TeamsToLogins: []services.TeamMapping{
				{
					Organization: "gravitational",
					Team:         "admins",
					Logins:       []string{"admin"},
					KubeGroups:   []string{"system:masters"},
				},
			},
		},
	}
	err := connector.CheckAndSetDefaults()
	c.Assert(err, check.IsNil)
	err = s.WebS.UpsertGithubConnector(connector)
	c.Assert(err, check.IsNil)
	out, err := s.WebS.GetGithubConnector(connector.GetName(), true)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, out, connector)

	connectors, err := s.WebS.GetGithubConnectors(true)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, []services.GithubConnector{connector}, connectors)

	out2, err := s.WebS.GetGithubConnector(connector.GetName(), false)
	c.Assert(err, check.IsNil)
	connectorNoSecrets := *connector
	connectorNoSecrets.Spec.ClientSecret = ""
	fixtures.DeepCompare(c, out2, &connectorNoSecrets)

	connectorsNoSecrets, err := s.WebS.GetGithubConnectors(false)
	c.Assert(err, check.IsNil)
	fixtures.DeepCompare(c, []services.GithubConnector{&connectorNoSecrets}, connectorsNoSecrets)

	err = s.WebS.DeleteGithubConnector(connector.GetName())
	c.Assert(err, check.IsNil)

	err = s.WebS.DeleteGithubConnector(connector.GetName())
	c.Assert(trace.IsNotFound(err), check.Equals, true, check.Commentf("expected not found, got %T", err))

	_, err = s.WebS.GetGithubConnector(connector.GetName(), true)
	c.Assert(trace.IsNotFound(err), check.Equals, true, check.Commentf("expected not found, got %T", err))
}

func (s *ServicesTestSuite) RemoteClustersCRUD(c *check.C) {
	clusterName := "example.com"
	out, err := s.PresenceS.GetRemoteClusters()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	rc, err := services.NewRemoteCluster(clusterName)
	c.Assert(err, check.IsNil)

	rc.SetConnectionStatus(teleport.RemoteClusterStatusOffline)

	err = s.PresenceS.CreateRemoteCluster(rc)
	c.Assert(err, check.IsNil)

	err = s.PresenceS.CreateRemoteCluster(rc)
	fixtures.ExpectAlreadyExists(c, err)

	out, err = s.PresenceS.GetRemoteClusters()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 1)
	rc.SetResourceID(out[0].GetResourceID())
	fixtures.DeepCompare(c, out[0], rc)

	err = s.PresenceS.DeleteAllRemoteClusters()
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetRemoteClusters()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 0)

	// test delete individual connection
	err = s.PresenceS.CreateRemoteCluster(rc)
	c.Assert(err, check.IsNil)

	out, err = s.PresenceS.GetRemoteClusters()
	c.Assert(err, check.IsNil)
	c.Assert(len(out), check.Equals, 1)
	fixtures.DeepCompare(c, out[0], rc)

	err = s.PresenceS.DeleteRemoteCluster(clusterName)
	c.Assert(err, check.IsNil)

	err = s.PresenceS.DeleteRemoteCluster(clusterName)
	fixtures.ExpectNotFound(c, err)
}

// AuthPreference tests authentication preference service
func (s *ServicesTestSuite) AuthPreference(c *check.C) {
	ap, err := services.NewAuthPreference(services.AuthPreferenceSpecV2{
		Type:         "local",
		SecondFactor: "otp",
	})
	c.Assert(err, check.IsNil)

	err = s.ConfigS.SetAuthPreference(ap)
	c.Assert(err, check.IsNil)

	gotAP, err := s.ConfigS.GetAuthPreference()
	c.Assert(err, check.IsNil)

	c.Assert(gotAP.GetType(), check.Equals, "local")
	c.Assert(gotAP.GetSecondFactor(), check.Equals, "otp")
}

func (s *ServicesTestSuite) StaticTokens(c *check.C) {
	// set static tokens
	staticTokens, err := services.NewStaticTokens(services.StaticTokensSpecV2{
		StaticTokens: []services.ProvisionTokenV1{
			{
				Token:   "tok1",
				Roles:   teleport.Roles{teleport.RoleNode},
				Expires: time.Now().UTC().Add(time.Hour),
			},
		},
	})
	c.Assert(err, check.IsNil)

	err = s.ConfigS.SetStaticTokens(staticTokens)
	c.Assert(err, check.IsNil)

	out, err := s.ConfigS.GetStaticTokens()
	c.Assert(err, check.IsNil)
	staticTokens.SetResourceID(out.GetResourceID())
	fixtures.DeepCompare(c, staticTokens, out)

	err = s.ConfigS.DeleteStaticTokens()
	c.Assert(err, check.IsNil)

	_, err = s.ConfigS.GetStaticTokens()
	fixtures.ExpectNotFound(c, err)
}

// Options provides functional arguments
// to turn certain parts of the test suite off
type Options struct {
	// SkipDelete turns off deletes in tests
	SkipDelete bool
}

// Option is a functional suite option
type Option func(s *Options)

// SkipDelete instructs tests to skip testing delete features
func SkipDelete() Option {
	return func(s *Options) {
		s.SkipDelete = true
	}
}

// CollectOptions collects suite options
func CollectOptions(opts ...Option) Options {
	var suiteOpts Options
	for _, o := range opts {
		o(&suiteOpts)
	}
	return suiteOpts
}

// ClusterConfig tests cluster configuration
func (s *ServicesTestSuite) ClusterConfig(c *check.C, opts ...Option) {
	config, err := services.NewClusterConfig(services.ClusterConfigSpecV3{
		ClientIdleTimeout:     services.NewDuration(17 * time.Second),
		DisconnectExpiredCert: services.NewBool(true),
		ClusterID:             "27",
		SessionRecording:      services.RecordAtProxy,
		Audit: services.AuditConfig{
			Region:           "us-west-1",
			Type:             "dynamodb",
			AuditSessionsURI: "file:///home/log",
			AuditTableName:   "audit_table_name",
			AuditEventsURI:   []string{"dynamodb://audit_table_name", "file:///home/log"},
		},
	})
	c.Assert(err, check.IsNil)

	err = s.ConfigS.SetClusterConfig(config)
	c.Assert(err, check.IsNil)

	gotConfig, err := s.ConfigS.GetClusterConfig()
	c.Assert(err, check.IsNil)
	config.SetResourceID(gotConfig.GetResourceID())
	fixtures.DeepCompare(c, config, gotConfig)

	// Some parts (e.g. auth server) will not function
	// without cluster name or cluster config
	if CollectOptions(opts...).SkipDelete {
		return
	}
	err = s.ConfigS.DeleteClusterConfig()
	c.Assert(err, check.IsNil)

	_, err = s.ConfigS.GetClusterConfig()
	fixtures.ExpectNotFound(c, err)

	clusterName, err := services.NewClusterName(services.ClusterNameSpecV2{
		ClusterName: "example.com",
	})
	c.Assert(err, check.IsNil)

	err = s.ConfigS.SetClusterName(clusterName)
	c.Assert(err, check.IsNil)

	gotName, err := s.ConfigS.GetClusterName()
	c.Assert(err, check.IsNil)
	clusterName.SetResourceID(gotName.GetResourceID())
	fixtures.DeepCompare(c, clusterName, gotName)

	err = s.ConfigS.DeleteClusterName()
	c.Assert(err, check.IsNil)

	_, err = s.ConfigS.GetClusterName()
	fixtures.ExpectNotFound(c, err)

	err = s.ConfigS.UpsertClusterName(clusterName)
	c.Assert(err, check.IsNil)

	gotName, err = s.ConfigS.GetClusterName()
	c.Assert(err, check.IsNil)
	clusterName.SetResourceID(gotName.GetResourceID())
	fixtures.DeepCompare(c, clusterName, gotName)
}

// sem wrapper is a helper for overriding the keepalive
// method on the semaphore service.
type semWrapper struct {
	services.Semaphores
	keepAlive func(context.Context, services.SemaphoreLease) error
}

func (w *semWrapper) KeepAliveSemaphoreLease(ctx context.Context, lease services.SemaphoreLease) error {
	return w.keepAlive(ctx, lease)
}

func (s *ServicesTestSuite) SemaphoreFlakiness(c *check.C) {
	const renewals = 3
	// wrap our services.Semaphores instance to cause two out of three lease
	// keepalive attempts fail with a meaningless error.  Locks should make
	// at *least* three attempts before their expiry, so locks should not
	// fail under these conditions.
	keepAlives := new(uint64)
	wrapper := &semWrapper{
		Semaphores: s.PresenceS,
		keepAlive: func(ctx context.Context, lease services.SemaphoreLease) error {
			kn := atomic.AddUint64(keepAlives, 1)
			if kn%3 == 0 {
				return s.PresenceS.KeepAliveSemaphoreLease(ctx, lease)
			}
			return trace.Errorf("uh-oh!")
		},
	}

	cfg := services.SemaphoreLockConfig{
		Service:  wrapper,
		Expiry:   time.Second,
		TickRate: time.Millisecond * 50,
		Params: services.AcquireSemaphoreRequest{
			SemaphoreKind: services.SemaphoreKindConnection,
			SemaphoreName: "alice",
			MaxLeases:     1,
		},
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	lock, err := services.AcquireSemaphoreLock(ctx, cfg)
	c.Assert(err, check.IsNil)
	go lock.KeepAlive(ctx)

	for i := 0; i < renewals; i++ {
		select {
		case <-lock.Renewed():
			continue
		case <-lock.Done():
			c.Fatalf("Lost semaphore lock: %v", lock.Wait())
		case <-time.After(time.Second):
			c.Fatalf("Timeout waiting for renewals")
		}
	}
}

// SemaphoreContention checks that a large number of concurrent acquisitions
// all succeed if MaxLeases is sufficiently high.  Note that we do not test
// the same property holds for releasing semaphores; release operations are
// best-effort and allowed to fail.  Also, "large" in this context is still
// fairly small.  Semaphores aren't cheap and the auth server is expected
// to start returning "too much contention" errors at around 100 concurrent
// attempts.
func (s *ServicesTestSuite) SemaphoreContention(c *check.C) {
	const locks int64 = 50
	const iters = 5
	for i := 0; i < iters; i++ {
		cfg := services.SemaphoreLockConfig{
			Service: s.PresenceS,
			Expiry:  time.Hour,
			Params: services.AcquireSemaphoreRequest{
				SemaphoreKind: services.SemaphoreKindConnection,
				SemaphoreName: "alice",
				MaxLeases:     locks,
			},
		}
		// we leak lock handles in the spawned goroutines, so
		// context-based cancellation is needed to cleanup the
		// background keepalive activity.
		ctx, cancel := context.WithCancel(context.TODO())
		var wg sync.WaitGroup
		for i := int64(0); i < locks; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				lock, err := services.AcquireSemaphoreLock(ctx, cfg)
				c.Assert(err, check.IsNil)
				go lock.KeepAlive(ctx)
			}()
		}
		wg.Wait()
		cancel()
		c.Assert(s.PresenceS.DeleteSemaphore(context.TODO(), services.SemaphoreFilter{
			SemaphoreKind: cfg.Params.SemaphoreKind,
			SemaphoreName: cfg.Params.SemaphoreName,
		}), check.IsNil)
	}
}

// SemaphoreConcurrency verifies that a large number of concurrent
// acquisitions result in the correct number of successful acquisitions.
func (s *ServicesTestSuite) SemaphoreConcurrency(c *check.C) {
	const maxLeases int64 = 20
	const attempts int64 = 200
	cfg := services.SemaphoreLockConfig{
		Service: s.PresenceS,
		Expiry:  time.Hour,
		Params: services.AcquireSemaphoreRequest{
			SemaphoreKind: services.SemaphoreKindConnection,
			SemaphoreName: "alice",
			MaxLeases:     maxLeases,
		},
	}
	// we leak lock handles in the spawned goroutines, so
	// context-based cancellation is needed to cleanup the
	// background keepalive activity.
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	var success int64
	var failure int64
	var wg sync.WaitGroup
	for i := int64(0); i < attempts; i++ {
		wg.Add(1)
		go func() {
			lock, err := services.AcquireSemaphoreLock(ctx, cfg)
			if err == nil {
				go lock.KeepAlive(ctx)
				atomic.AddInt64(&success, 1)
			} else {
				atomic.AddInt64(&failure, 1)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	c.Assert(atomic.LoadInt64(&success), check.Equals, maxLeases)
	c.Assert(atomic.LoadInt64(&failure), check.Equals, attempts-maxLeases)
}

// SemaphoreLock verifies correct functionality of the basic
// semaphore lock scenarios.
func (s *ServicesTestSuite) SemaphoreLock(c *check.C) {
	cfg := services.SemaphoreLockConfig{
		Service: s.PresenceS,
		Expiry:  time.Hour,
		Params: services.AcquireSemaphoreRequest{
			SemaphoreKind: services.SemaphoreKindConnection,
			SemaphoreName: "alice",
			MaxLeases:     1,
		},
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	lock, err := services.AcquireSemaphoreLock(ctx, cfg)
	c.Assert(err, check.IsNil)
	go lock.KeepAlive(ctx)

	// MaxLeases is 1, so second acquire op fails.
	_, err = services.AcquireSemaphoreLock(ctx, cfg)
	fixtures.ExpectLimitExceeded(c, err)

	// Lock is successfully released.
	lock.Stop()
	c.Assert(lock.Wait(), check.IsNil)

	// Acquire new lock with short expiry
	// and high tick rate to verify renewals.
	cfg.Expiry = time.Second
	cfg.TickRate = time.Millisecond * 50
	lock, err = services.AcquireSemaphoreLock(ctx, cfg)
	c.Assert(err, check.IsNil)
	go lock.KeepAlive(ctx)

	timeout := time.After(time.Second)

	for i := 0; i < 3; i++ {
		select {
		case <-lock.Done():
			c.Fatalf("Unexpected lock failure: %v", lock.Wait())
		case <-timeout:
			c.Fatalf("Timeout waiting for lock renewal %d", i)
		case <-lock.Renewed():
		}
	}

	// forcibly delete the semaphore
	c.Assert(s.PresenceS.DeleteSemaphore(context.TODO(), services.SemaphoreFilter{
		SemaphoreKind: cfg.Params.SemaphoreKind,
		SemaphoreName: cfg.Params.SemaphoreName,
	}), check.IsNil)

	select {
	case <-lock.Done():
		fixtures.ExpectNotFound(c, lock.Wait())
	case <-time.After(time.Millisecond * 1500):
		c.Errorf("timeout waiting for semaphore lock failure")
	}
}

// Events tests various events variations
func (s *ServicesTestSuite) Events(c *check.C) {
	ctx := context.Background()
	testCases := []eventTest{
		{
			name: "Cert authority with secrets",
			kind: services.WatchKind{
				Kind:        services.KindCertAuthority,
				LoadSecrets: true,
			},
			crud: func() services.Resource {
				ca := NewTestCA(services.UserCA, "example.com")
				c.Assert(s.CAS.UpsertCertAuthority(ca), check.IsNil)

				out, err := s.CAS.GetCertAuthority(*ca.ID(), true)
				c.Assert(err, check.IsNil)

				c.Assert(s.CAS.DeleteCertAuthority(*ca.ID()), check.IsNil)
				return out
			},
		},
	}
	s.runEventsTests(c, testCases)

	testCases = []eventTest{
		{
			name: "Cert authority without secrets",
			kind: services.WatchKind{
				Kind:        services.KindCertAuthority,
				LoadSecrets: false,
			},
			crud: func() services.Resource {
				ca := NewTestCA(services.UserCA, "example.com")
				c.Assert(s.CAS.UpsertCertAuthority(ca), check.IsNil)

				out, err := s.CAS.GetCertAuthority(*ca.ID(), false)
				c.Assert(err, check.IsNil)

				c.Assert(s.CAS.DeleteCertAuthority(*ca.ID()), check.IsNil)
				return out
			},
		},
	}
	s.runEventsTests(c, testCases)

	testCases = []eventTest{
		{
			name: "Token",
			kind: services.WatchKind{
				Kind: services.KindToken,
			},
			crud: func() services.Resource {
				expires := time.Now().UTC().Add(time.Hour)
				t, err := services.NewProvisionToken("token",
					teleport.Roles{teleport.RoleAuth, teleport.RoleNode}, expires)
				c.Assert(err, check.IsNil)

				c.Assert(s.ProvisioningS.UpsertToken(t), check.IsNil)

				token, err := s.ProvisioningS.GetToken("token")
				c.Assert(err, check.IsNil)

				c.Assert(s.ProvisioningS.DeleteToken("token"), check.IsNil)
				return token
			},
		},
		{
			name: "Namespace",
			kind: services.WatchKind{
				Kind: services.KindNamespace,
			},
			crud: func() services.Resource {
				ns := services.Namespace{
					Kind:    services.KindNamespace,
					Version: services.V2,
					Metadata: services.Metadata{
						Name:      "testnamespace",
						Namespace: defaults.Namespace,
					},
				}
				err := s.PresenceS.UpsertNamespace(ns)
				c.Assert(err, check.IsNil)

				out, err := s.PresenceS.GetNamespace(ns.Metadata.Name)
				c.Assert(err, check.IsNil)

				err = s.PresenceS.DeleteNamespace(ns.Metadata.Name)
				c.Assert(err, check.IsNil)

				return out
			},
		},
		{
			name: "Static tokens",
			kind: services.WatchKind{
				Kind: services.KindStaticTokens,
			},
			crud: func() services.Resource {
				staticTokens, err := services.NewStaticTokens(services.StaticTokensSpecV2{
					StaticTokens: []services.ProvisionTokenV1{
						{
							Token:   "tok1",
							Roles:   teleport.Roles{teleport.RoleNode},
							Expires: time.Now().UTC().Add(time.Hour),
						},
					},
				})
				c.Assert(err, check.IsNil)

				err = s.ConfigS.SetStaticTokens(staticTokens)
				c.Assert(err, check.IsNil)

				out, err := s.ConfigS.GetStaticTokens()
				c.Assert(err, check.IsNil)

				err = s.ConfigS.DeleteStaticTokens()
				c.Assert(err, check.IsNil)

				return out
			},
		},
		{
			name: "Role",
			kind: services.WatchKind{
				Kind: services.KindRole,
			},
			crud: func() services.Resource {
				role, err := services.NewRole("role1", services.RoleSpecV3{
					Options: services.RoleOptions{
						MaxSessionTTL: services.Duration(time.Hour),
					},
					Allow: services.RoleConditions{
						Logins:     []string{"root", "bob"},
						NodeLabels: services.Labels{services.Wildcard: []string{services.Wildcard}},
					},
					Deny: services.RoleConditions{},
				})
				c.Assert(err, check.IsNil)

				err = s.Access.UpsertRole(ctx, role)
				c.Assert(err, check.IsNil)

				out, err := s.Access.GetRole(role.GetName())
				c.Assert(err, check.IsNil)

				err = s.Access.DeleteRole(ctx, role.GetName())
				c.Assert(err, check.IsNil)

				return out
			},
		},
		{
			name: "User",
			kind: services.WatchKind{
				Kind: services.KindUser,
			},
			crud: func() services.Resource {
				user := newUser("user1", []string{"admin"})
				err := s.Users().UpsertUser(user)
				c.Assert(err, check.IsNil)

				out, err := s.Users().GetUser(user.GetName(), false)
				c.Assert(err, check.IsNil)

				c.Assert(s.Users().DeleteUser(context.TODO(), user.GetName()), check.IsNil)
				return out
			},
		},
		{
			name: "Node",
			kind: services.WatchKind{
				Kind: services.KindNode,
			},
			crud: func() services.Resource {
				srv := NewServer(services.KindNode, "srv1", "127.0.0.1:2022", defaults.Namespace)

				_, err := s.PresenceS.UpsertNode(srv)
				c.Assert(err, check.IsNil)

				out, err := s.PresenceS.GetNodes(srv.Metadata.Namespace)
				c.Assert(err, check.IsNil)

				err = s.PresenceS.DeleteAllNodes(srv.Metadata.Namespace)
				c.Assert(err, check.IsNil)

				return out[0]
			},
		},
		{
			name: "Proxy",
			kind: services.WatchKind{
				Kind: services.KindProxy,
			},
			crud: func() services.Resource {
				srv := NewServer(services.KindProxy, "srv1", "127.0.0.1:2022", defaults.Namespace)

				err := s.PresenceS.UpsertProxy(srv)
				c.Assert(err, check.IsNil)

				out, err := s.PresenceS.GetProxies()
				c.Assert(err, check.IsNil)

				err = s.PresenceS.DeleteAllProxies()
				c.Assert(err, check.IsNil)

				return out[0]
			},
		},
		{
			name: "Tunnel connection",
			kind: services.WatchKind{
				Kind: services.KindTunnelConnection,
			},
			crud: func() services.Resource {
				conn, err := services.NewTunnelConnection("conn1", services.TunnelConnectionSpecV2{
					ClusterName:   "example.com",
					ProxyName:     "p1",
					LastHeartbeat: time.Now().UTC(),
				})
				c.Assert(err, check.IsNil)

				err = s.PresenceS.UpsertTunnelConnection(conn)
				c.Assert(err, check.IsNil)

				out, err := s.PresenceS.GetTunnelConnections("example.com")
				c.Assert(err, check.IsNil)

				err = s.PresenceS.DeleteAllTunnelConnections()
				c.Assert(err, check.IsNil)

				return out[0]
			},
		},
		{
			name: "Reverse tunnel",
			kind: services.WatchKind{
				Kind: services.KindReverseTunnel,
			},
			crud: func() services.Resource {
				tunnel := newReverseTunnel("example.com", []string{"example.com:2023"})
				c.Assert(s.PresenceS.UpsertReverseTunnel(tunnel), check.IsNil)

				out, err := s.PresenceS.GetReverseTunnels()
				c.Assert(err, check.IsNil)

				err = s.PresenceS.DeleteReverseTunnel(tunnel.Spec.ClusterName)
				c.Assert(err, check.IsNil)

				return out[0]
			},
		},
		{
			name: "Remote cluster",
			kind: services.WatchKind{
				Kind: services.KindRemoteCluster,
			},
			crud: func() services.Resource {
				rc, err := services.NewRemoteCluster("example.com")
				rc.SetConnectionStatus(teleport.RemoteClusterStatusOffline)
				c.Assert(err, check.IsNil)
				c.Assert(s.PresenceS.CreateRemoteCluster(rc), check.IsNil)

				out, err := s.PresenceS.GetRemoteClusters()
				c.Assert(err, check.IsNil)

				err = s.PresenceS.DeleteRemoteCluster(rc.GetName())
				c.Assert(err, check.IsNil)

				return out[0]
			},
		},
	}
	s.runEventsTests(c, testCases)

	// Namespace with a name
	testCases = []eventTest{
		{
			name: "Namespace with a name",
			kind: services.WatchKind{
				Kind: services.KindNamespace,
				Name: "shmest",
			},
			crud: func() services.Resource {
				ns := services.Namespace{
					Kind:    services.KindNamespace,
					Version: services.V2,
					Metadata: services.Metadata{
						Name:      "shmest",
						Namespace: defaults.Namespace,
					},
				}
				err := s.PresenceS.UpsertNamespace(ns)
				c.Assert(err, check.IsNil)

				out, err := s.PresenceS.GetNamespace(ns.Metadata.Name)
				c.Assert(err, check.IsNil)

				err = s.PresenceS.DeleteNamespace(ns.Metadata.Name)
				c.Assert(err, check.IsNil)

				return out
			},
		},
	}
	s.runEventsTests(c, testCases)
}

// EventsClusterConfig tests cluster config resource events
func (s *ServicesTestSuite) EventsClusterConfig(c *check.C) {
	testCases := []eventTest{
		{
			name: "Cluster config",
			kind: services.WatchKind{
				Kind: services.KindClusterConfig,
			},
			crud: func() services.Resource {
				config, err := services.NewClusterConfig(services.ClusterConfigSpecV3{})
				c.Assert(err, check.IsNil)

				err = s.ConfigS.SetClusterConfig(config)
				c.Assert(err, check.IsNil)

				out, err := s.ConfigS.GetClusterConfig()
				c.Assert(err, check.IsNil)

				err = s.ConfigS.DeleteClusterConfig()
				c.Assert(err, check.IsNil)

				return out
			},
		},
		{
			name: "Cluster name",
			kind: services.WatchKind{
				Kind: services.KindClusterName,
			},
			crud: func() services.Resource {
				clusterName, err := services.NewClusterName(services.ClusterNameSpecV2{
					ClusterName: "example.com",
				})
				c.Assert(err, check.IsNil)

				err = s.ConfigS.SetClusterName(clusterName)
				c.Assert(err, check.IsNil)

				out, err := s.ConfigS.GetClusterName()
				c.Assert(err, check.IsNil)

				err = s.ConfigS.DeleteClusterName()
				c.Assert(err, check.IsNil)
				return out
			},
		},
	}
	s.runEventsTests(c, testCases)
}

// ProxyWatcher tests proxy watcher
func (s *ServicesTestSuite) ProxyWatcher(c *check.C) {
	w, err := s.NewProxyWatcher()
	c.Assert(err, check.IsNil)
	defer w.Close()

	// since no proxy is yet present, the ProxyWatcher should immediately
	// yield back to its retry loop.
	select {
	case <-w.Reset():
	case <-time.After(time.Second):
		c.Fatalf("Timeout waiting for ProxyWatcher reset")
	}

	proxy := NewServer(services.KindProxy, "proxy1", "127.0.0.1:2023", defaults.Namespace)
	c.Assert(s.PresenceS.UpsertProxy(proxy), check.IsNil)

	// the first event is always the current list of proxies
	select {
	case changeset := <-w.ProxiesC:
		c.Assert(changeset, check.HasLen, 1)
		out, err := s.PresenceS.GetProxies()
		c.Assert(err, check.IsNil)
		fixtures.DeepCompare(c, changeset[0], out[0])
	case <-w.Done():
		c.Fatalf("Watcher has unexpectedly exited.")
	case <-time.After(2 * time.Second):
		c.Fatalf("Timeout waiting for the first event")
	}

	// add a second proxy
	proxy2 := NewServer(services.KindProxy, "proxy2", "127.0.0.1:2023", defaults.Namespace)
	c.Assert(s.PresenceS.UpsertProxy(proxy2), check.IsNil)

	// watcher should detect the proxy list change
	select {
	case changeset := <-w.ProxiesC:
		c.Assert(changeset, check.HasLen, 2)
	case <-w.Done():
		c.Fatalf("Watcher has unexpectedly exited.")
	case <-time.After(2 * time.Second):
		c.Fatalf("Timeout waiting for the first event")
	}

	c.Assert(s.PresenceS.DeleteProxy(proxy.GetName()), check.IsNil)

	// watcher should detect the proxy list change
	select {
	case changeset := <-w.ProxiesC:
		c.Assert(changeset, check.HasLen, 1)
		out, err := s.PresenceS.GetProxies()
		c.Assert(err, check.IsNil)
		fixtures.DeepCompare(c, changeset[0], out[0])
	case <-w.Done():
		c.Fatalf("Watcher has unexpectedly exited.")
	case <-time.After(2 * time.Second):
		c.Fatalf("Timeout waiting for the first event")
	}
}

func (s *ServicesTestSuite) runEventsTests(c *check.C, testCases []eventTest) {
	ctx := context.TODO()
	w, err := s.EventsS.NewWatcher(ctx, services.Watch{
		Kinds: eventsTestKinds(testCases),
	})
	c.Assert(err, check.IsNil)
	defer w.Close()

	select {
	case event := <-w.Events():
		c.Assert(event.Type, check.Equals, backend.OpInit)
	case <-w.Done():
		c.Fatalf("Watcher exited with error %v", w.Error())
	case <-time.After(2 * time.Second):
		c.Fatalf("Timeout waiting for init event")
	}

	// filter out all events that could have been inserted
	// by the initialization routines
skiploop:
	for {
		select {
		case event := <-w.Events():
			log.Debugf("Skipping pre-test event: %v", event)
			continue skiploop
		default:
			break skiploop
		case <-w.Done():
			c.Fatalf("Watcher exited with error %v", w.Error())
		}
	}

	for _, tc := range testCases {
		c.Logf("test case %q", tc.name)
		resource := tc.crud()

		ExpectResource(c, w, 3*time.Second, resource)

		meta := resource.GetMetadata()
		header := &services.ResourceHeader{
			Kind:    resource.GetKind(),
			SubKind: resource.GetSubKind(),
			Version: resource.GetVersion(),
			Metadata: services.Metadata{
				Name:      meta.Name,
				Namespace: meta.Namespace,
			},
		}
		// delete events don't have IDs yet
		header.SetResourceID(0)
		ExpectDeleteResource(c, w, 3*time.Second, header)
	}
}

type eventTest struct {
	name string
	kind services.WatchKind
	crud func() services.Resource
}

func eventsTestKinds(tests []eventTest) []services.WatchKind {
	out := make([]services.WatchKind, len(tests))
	for i, tc := range tests {
		out[i] = tc.kind
	}
	return out
}

// ExpectResource expects a Put event of a certain resource
func ExpectResource(c *check.C, w services.Watcher, timeout time.Duration, resource services.Resource) {
	timeoutC := time.After(timeout)
waitLoop:
	for {
		select {
		case <-timeoutC:
			c.Fatalf("Timeout waiting for event")
		case <-w.Done():
			c.Fatalf("Watcher exited with error %v", w.Error())
		case event := <-w.Events():
			if event.Type != backend.OpPut {
				log.Debugf("Skipping event %v %v", event.Type, event.Resource.GetName())
				continue
			}
			if resource.GetResourceID() > event.Resource.GetResourceID() {
				log.Debugf("Skipping stale event %v %v %v %v, latest object version is %v", event.Type, event.Resource.GetKind(), event.Resource.GetName(), event.Resource.GetResourceID(), resource.GetResourceID())
				continue waitLoop
			}
			if resource.GetName() != event.Resource.GetName() || resource.GetKind() != event.Resource.GetKind() || resource.GetSubKind() != event.Resource.GetSubKind() {
				log.Debugf("Skipping event %v resource %v, expecting %v", event.Type, event.Resource.GetMetadata(), event.Resource.GetMetadata())
				continue waitLoop
			}
			fixtures.DeepCompare(c, resource, event.Resource)
			break waitLoop
		}
	}
}

// ExpectDeleteResource expects a delete event of a certain kind
func ExpectDeleteResource(c *check.C, w services.Watcher, timeout time.Duration, resource services.Resource) {
	timeoutC := time.After(timeout)
waitLoop:
	for {
		select {
		case <-timeoutC:
			c.Fatalf("Timeout waiting for delete resource %v", resource)
		case <-w.Done():
			c.Fatalf("Watcher exited with error %v", w.Error())
		case event := <-w.Events():
			if event.Type != backend.OpDelete {
				log.Debugf("Skipping stale event %v %v", event.Type, event.Resource.GetName())
				continue
			}
			fixtures.DeepCompare(c, resource, event.Resource)
			break waitLoop
		}
	}
}
