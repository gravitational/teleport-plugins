/*
Copyright 2018 Gravitational, Inc.

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

package integration

import (
	"context"
	"crypto/rsa"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"runtime/debug"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/auth/native"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/backend/lite"
	"github.com/gravitational/teleport/lib/client"
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/service"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/sshutils"
	"github.com/gravitational/teleport/lib/tlsca"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

const (
	Loopback = "127.0.0.1"
	Host     = "localhost"
)

// SetTestTimeouts affects global timeouts inside Teleport, making connections
// work faster but consuming more CPU (useful for integration testing)
func SetTestTimeouts(t time.Duration) {
	defaults.KeepAliveInterval = t
	defaults.ResyncInterval = t
	defaults.ServerKeepAliveTTL = t
	defaults.SessionRefreshPeriod = t
	defaults.HeartbeatCheckPeriod = t
	defaults.CachePollPeriod = t
}

// TeleInstance represents an in-memory instance of a teleport
// process for testing
type TeleInstance struct {
	// Secrets holds the keys (pub, priv and derived cert) of i instance
	Secrets InstanceSecrets

	// Hostname is the name of the host where instance is running
	Hostname string

	// Internal stuff...
	Process *service.TeleportProcess
	Config  *service.Config
}

type User struct {
	Username      string          `json:"username"`
	AllowedLogins []string        `json:"logins"`
	Key           *client.Key     `json:"key"`
	Roles         []services.Role `json:"-"`
}

type InstanceSecrets struct {
	// instance name (aka "site name")
	SiteName string `json:"site_name"`
	// instance keys+cert (reused for hostCA and userCA)
	// PubKey is instance public key
	PubKey []byte `json:"pub"`
	// PrivKey is instance private key
	PrivKey []byte `json:"priv"`
	// Cert is SSH host certificate
	Cert []byte `json:"cert"`
	// TLSCACert is the certificate of the trusted certificate authority
	TLSCACert []byte `json:"tls_ca_cert"`
	// TLSCert is client TLS X509 certificate
	TLSCert []byte `json:"tls_cert"`
	// ListenAddr is a reverse tunnel listening port, allowing
	// other sites to connect to i instance. Set to empty
	// string if i instance is not allowing incoming tunnels
	ListenAddr string `json:"tunnel_addr"`
	// WebProxyAddr is address for web proxy
	WebProxyAddr string `json:"web_proxy_addr"`
	// list of users i instance trusts (key in the map is username)
	Users map[string]*User `json:"users"`
}

func (s *InstanceSecrets) String() string {
	bytes, _ := json.MarshalIndent(s, "", "\t")
	return string(bytes)
}

// InstanceConfig is an instance configuration
type InstanceConfig struct {
	// ClusterName is a cluster name of the instance
	ClusterName string
	// HostID is a host id of the instance
	HostID string
	// NodeName is a node name of the instance
	NodeName string
	// Priv is SSH private key of the instance
	Priv []byte
	// Pub is SSH public key of the instance
	Pub []byte
	// MultiplexProxy uses the same port for web and SSH reverse tunnel proxy
	MultiplexProxy bool
}

// NewInstance creates a new Teleport process instance
func NewInstance(cfg InstanceConfig) *TeleInstance {
	var err error
	if cfg.NodeName == "" {
		cfg.NodeName, err = os.Hostname()
		panicIf(err)
	}
	// generate instance secrets (keys):
	keygen, err := native.New(context.TODO(), native.PrecomputeKeys(0))
	panicIf(err)
	if cfg.Priv == nil || cfg.Pub == nil {
		cfg.Priv, cfg.Pub, _ = keygen.GenerateKeyPair("")
	}
	rsaKey, err := ssh.ParseRawPrivateKey(cfg.Priv)
	panicIf(err)

	tlsCAKey, tlsCACert, err := tlsca.GenerateSelfSignedCAWithPrivateKey(rsaKey.(*rsa.PrivateKey), pkix.Name{
		CommonName:   cfg.ClusterName,
		Organization: []string{cfg.ClusterName},
	}, nil, defaults.CATTL)
	panicIf(err)

	cert, err := keygen.GenerateHostCert(services.HostCertParams{
		PrivateCASigningKey: cfg.Priv,
		CASigningAlg:        defaults.CASignatureAlgorithm,
		PublicHostKey:       cfg.Pub,
		HostID:              cfg.HostID,
		NodeName:            cfg.NodeName,
		ClusterName:         cfg.ClusterName,
		Roles:               teleport.Roles{teleport.RoleAdmin},
		TTL:                 time.Hour * 24,
	})
	panicIf(err)
	tlsCA, err := tlsca.New(tlsCACert, tlsCAKey)
	panicIf(err)
	cryptoPubKey, err := sshutils.CryptoPublicKey(cfg.Pub)
	panicIf(err)
	identity := tlsca.Identity{
		Username: fmt.Sprintf("%v.%v", cfg.HostID, cfg.ClusterName),
		Groups:   []string{string(teleport.RoleAdmin)},
	}
	clock := clockwork.NewRealClock()
	subject, err := identity.Subject()
	panicIf(err)
	tlsCert, err := tlsCA.GenerateCertificate(tlsca.CertificateRequest{
		Clock:     clock,
		PublicKey: cryptoPubKey,
		Subject:   subject,
		NotAfter:  clock.Now().UTC().Add(time.Hour * 24),
	})
	panicIf(err)

	i := &TeleInstance{
		Hostname: cfg.NodeName,
	}
	secrets := InstanceSecrets{
		SiteName:  cfg.ClusterName,
		PrivKey:   cfg.Priv,
		PubKey:    cfg.Pub,
		Cert:      cert,
		TLSCACert: tlsCACert,
		TLSCert:   tlsCert,
		Users:     make(map[string]*User),
	}
	if cfg.MultiplexProxy {
		secrets.ListenAddr = secrets.WebProxyAddr
	}
	i.Secrets = secrets
	return i
}

// GetRoles returns a list of roles to initiate for this secret
func (s *InstanceSecrets) GetRoles() []services.Role {
	var roles []services.Role
	for _, ca := range s.GetCAs() {
		if ca.GetType() != services.UserCA {
			continue
		}
		role := services.RoleForCertAuthority(ca)
		role.SetLogins(services.Allow, s.AllowedLogins())
		roles = append(roles, role)
	}
	return roles
}

// GetCAs return an array of CAs stored by the secrets object. In i
// case we always return hard-coded userCA + hostCA (and they share keys
// for simplicity)
func (s *InstanceSecrets) GetCAs() []services.CertAuthority {
	hostCA := services.NewCertAuthority(
		services.HostCA,
		s.SiteName,
		[][]byte{s.PrivKey},
		[][]byte{s.PubKey},
		[]string{},
		services.CertAuthoritySpecV2_RSA_SHA2_512,
	)
	hostCA.SetTLSKeyPairs([]services.TLSKeyPair{{Cert: s.TLSCACert, Key: s.PrivKey}})
	return []services.CertAuthority{
		hostCA,
		services.NewCertAuthority(
			services.UserCA,
			s.SiteName,
			[][]byte{s.PrivKey},
			[][]byte{s.PubKey},
			[]string{services.RoleNameForCertAuthority(s.SiteName)},
			services.CertAuthoritySpecV2_RSA_SHA2_512,
		),
	}
}

func (s *InstanceSecrets) AllowedLogins() []string {
	var logins []string
	for i := range s.Users {
		logins = append(logins, s.Users[i].AllowedLogins...)
	}
	return logins
}

func (s *InstanceSecrets) AsSlice() []*InstanceSecrets {
	return []*InstanceSecrets{s}
}

func (s *InstanceSecrets) GetIdentity() *auth.Identity {
	i, err := auth.ReadIdentityFromKeyPair(&auth.PackedKeys{
		Key:        s.PrivKey,
		Cert:       s.Cert,
		TLSCert:    s.TLSCert,
		TLSCACerts: [][]byte{s.TLSCACert},
	})
	panicIf(err)
	return i
}

// Create creates a new instance of Teleport which trusts a lsit of other clusters (other
// instances)
func (i *TeleInstance) Create(trustedSecrets []*InstanceSecrets, console io.Writer) error {
	tconf := service.MakeDefaultConfig()
	tconf.Console = console
	tconf.Proxy.DisableWebService = true
	tconf.Proxy.DisableWebInterface = true
	return i.CreateEx(trustedSecrets, tconf)
}

// GenerateConfig generates instance config
func (i *TeleInstance) GenerateConfig(trustedSecrets []*InstanceSecrets, tconf *service.Config) (*service.Config, error) {
	var err error
	dataDir, err := ioutil.TempDir("", "cluster-"+i.Secrets.SiteName)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if tconf == nil {
		tconf = service.MakeDefaultConfig()
	}
	tconf.DataDir = dataDir
	tconf.CachePolicy.Enabled = false
	tconf.Auth.ClusterName, err = services.NewClusterName(services.ClusterNameSpecV2{
		ClusterName: i.Secrets.SiteName,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tconf.Auth.Authorities = append(tconf.Auth.Authorities, i.Secrets.GetCAs()...)
	tconf.Identities = append(tconf.Identities, i.Secrets.GetIdentity())
	for _, trusted := range trustedSecrets {
		tconf.Auth.Authorities = append(tconf.Auth.Authorities, trusted.GetCAs()...)
		tconf.Auth.Roles = append(tconf.Auth.Roles, trusted.GetRoles()...)
		tconf.Identities = append(tconf.Identities, trusted.GetIdentity())
		if trusted.ListenAddr != "" {
			tconf.ReverseTunnels = []services.ReverseTunnel{
				services.NewReverseTunnel(trusted.SiteName, []string{trusted.ListenAddr}),
			}
		}
	}
	tconf.HostUUID = i.Secrets.GetIdentity().ID.HostUUID
	tconf.Auth.SSHAddr.Addr = net.JoinHostPort(i.Hostname, "0")
	tconf.AuthServers = append(tconf.AuthServers, tconf.Auth.SSHAddr)
	tconf.Auth.PublicAddrs = []utils.NetAddr{
		utils.NetAddr{
			AddrNetwork: "tcp",
			Addr:        i.Hostname,
		},
		utils.NetAddr{
			AddrNetwork: "tcp",
			Addr:        Loopback,
		},
	}
	tconf.Auth.StorageConfig = backend.Config{
		Type:   lite.GetName(),
		Params: backend.Params{"path": dataDir + string(os.PathListSeparator) + defaults.BackendDir, "poll_stream_period": 50 * time.Millisecond},
	}
	tconf.Proxy.Enabled = false
	tconf.SSH.Enabled = false

	tconf.Keygen = testauthority.New()
	i.Config = tconf
	return tconf, nil
}

// CreateEx creates a new instance of Teleport which trusts a list of other clusters (other
// instances)
//
// Unlike Create() it allows for greater customization because it accepts
// a full Teleport config structure
func (i *TeleInstance) CreateEx(trustedSecrets []*InstanceSecrets, tconf *service.Config) error {
	ctx := context.TODO()
	tconf, err := i.GenerateConfig(trustedSecrets, tconf)
	if err != nil {
		return trace.Wrap(err)
	}
	i.Config = tconf
	i.Process, err = service.NewTeleport(tconf)
	if err != nil {
		return trace.Wrap(err)
	}

	// if the auth server is not enabled, nothing more to do be done
	if !tconf.Auth.Enabled {
		return nil
	}

	// if this instance contains an auth server, configure the auth server as well.
	// create users and roles if they don't exist, or sign their keys if they're
	// already present
	auth := i.Process.GetAuthServer()

	for _, user := range i.Secrets.Users {
		teleUser, err := services.NewUser(user.Username)
		if err != nil {
			return trace.Wrap(err)
		}
		// set hardcode traits to trigger new style certificates
		teleUser.SetTraits(map[string][]string{"testing": []string{"integration"}})
		if len(user.Roles) == 0 {
			role := services.RoleForUser(teleUser)
			role.SetLogins(services.Allow, user.AllowedLogins)

			// allow tests to forward agent, still needs to be passed in client
			roleOptions := role.GetOptions()
			roleOptions.ForwardAgent = services.NewBool(true)
			role.SetOptions(roleOptions)

			err = auth.UpsertRole(ctx, role)
			if err != nil {
				return trace.Wrap(err)
			}
			teleUser.AddRole(role.GetMetadata().Name)
		} else {
			for _, role := range user.Roles {
				err := auth.UpsertRole(ctx, role)
				if err != nil {
					return trace.Wrap(err)
				}
				teleUser.AddRole(role.GetName())
			}
		}
		err = auth.UpsertUser(teleUser)
		if err != nil {
			return trace.Wrap(err)
		}
		// if user keys are not present, auto-geneate keys:
		if user.Key == nil || len(user.Key.Pub) == 0 {
			priv, pub, _ := tconf.Keygen.GenerateKeyPair("")
			user.Key = &client.Key{
				Priv: priv,
				Pub:  pub,
			}
		}
		// sign user's keys:
		ttl := 24 * time.Hour
		user.Key.Cert, user.Key.TLSCert, err = auth.GenerateUserTestCerts(user.Key.Pub, teleUser.GetName(), ttl, teleport.CertificateFormatStandard, "")
		if err != nil {
			return err
		}
	}
	return nil
}

// Reset re-creates the teleport instance based on the same configuration
// This is needed if you want to stop the instance, reset it and start again
func (i *TeleInstance) Reset() (err error) {
	i.Process, err = service.NewTeleport(i.Config)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// AddUserUserWithRole adds user with one or many assigned roles
func (i *TeleInstance) AddUserWithRole(username string, roles ...services.Role) *User {
	user := &User{
		Username: username,
		Roles:    make([]services.Role, len(roles)),
	}
	copy(user.Roles, roles)
	i.Secrets.Users[username] = user
	return user
}

// Adds a new user into i Teleport instance. 'mappings' is a comma-separated
// list of OS users
func (i *TeleInstance) AddUser(username string, mappings []string) *User {
	log.Infof("teleInstance.AddUser(%v) mapped to %v", username, mappings)
	if mappings == nil {
		mappings = make([]string, 0)
	}
	user := &User{
		Username:      username,
		AllowedLogins: mappings,
	}
	i.Secrets.Users[username] = user
	return user
}

func (i *TeleInstance) CreateAccessRequest(ctx context.Context, user string, roles ...string) (services.AccessRequest, error) {
	auth := i.Process.GetAuthServer()
	req, err := services.NewAccessRequest(user, roles...)
	if err != nil {
		return req, err
	}
	err = auth.CreateAccessRequest(ctx, req)
	return req, err
}

func (i *TeleInstance) CreateExpiredAccessRequest(ctx context.Context, user string, roles ...string) (services.AccessRequest, error) {
	req, err := services.NewAccessRequest(user, roles...)
	if err != nil {
		return req, err
	}
	ttl := time.Millisecond * 250
	req.SetAccessExpiry(time.Now().Add(ttl))
	if err = i.Process.GetAuthServer().CreateAccessRequest(ctx, req); err != nil {
		return req, err
	}

	time.Sleep(ttl)
	ctx, cancel := context.WithTimeout(ctx, ttl)
	defer cancel()
	for {
		req1, err := i.GetAccessRequest(ctx, req.GetName())
		if err != nil {
			return req, trace.Wrap(err)
		}
		if req1 == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	return req, nil
}

func (i *TeleInstance) GetAccessRequest(ctx context.Context, reqID string) (services.AccessRequest, error) {
	requests, err := i.Process.GetAuthServer().GetAccessRequests(ctx, services.AccessRequestFilter{ID: reqID})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if len(requests) == 0 {
		return nil, nil
	}
	return requests[0], nil
}

func (i *TeleInstance) PollAccessRequestPluginData(ctx context.Context, plugin, reqID string) (map[string]string, error) {
	auth := i.Process.GetAuthServer()
	filter := services.PluginDataFilter{
		Kind:     services.KindAccessRequest,
		Resource: reqID,
		Plugin:   plugin,
	}
	for {
		pluginDatas, err := auth.GetPluginData(ctx, filter)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if len(pluginDatas) > 0 {
			pluginData := pluginDatas[0]
			entry := pluginData.Entries()[plugin]
			if entry != nil {
				return entry.Data, nil
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (i *TeleInstance) SearchAuditEvents(query string) ([]events.EventFields, error) {
	result, err := i.Process.GetAuditLog().SearchEvents(time.Now().UTC().AddDate(0, -1, 0), time.Now().UTC(), query, 0)
	return result, trace.Wrap(err)
}

func (i *TeleInstance) FilterAuditEvents(query string, filter events.EventFields) ([]events.EventFields, error) {
	searchResult, err := i.SearchAuditEvents(query)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var result []events.EventFields
	for _, event := range searchResult {
		ok := true
		for key, obj := range filter {
			switch value := obj.(type) {
			case int:
				ok = ok && event.GetInt(key) == value
			case string:
				ok = ok && event.GetString(key) == value
			default:
				return nil, trace.Fatalf("unsupported filter type %T", value)
			}
			if !ok {
				break
			}
		}
		if ok {
			result = append(result, event)
		}
	}
	return result, nil
}

// Start will start the TeleInstance and then block until it is ready to
// process requests based off the passed in configuration.
func (i *TeleInstance) Start() error {
	// Build a list of expected events to wait for before unblocking based off
	// the configuration passed in.
	expectedEvents := []string{}
	if i.Config.Auth.Enabled {
		expectedEvents = append(expectedEvents, service.AuthTLSReady)
	}

	// Start the process and block until the expected events have arrived.
	receivedEvents, err := startAndWait(i.Process, expectedEvents)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debugf("Teleport instance %v started: %v/%v events received.",
		i.Secrets.SiteName, len(receivedEvents), len(expectedEvents))
	return nil
}

// ClientConfig is a client configuration
type ClientConfig struct {
	// Login is SSH login name
	Login string
	// Cluster is a cluster name to connect to
	Cluster string
	// Host string is a target host to connect to
	Host string
	// Port is a target port to connect to
	Port int
	// ForwardAgent controls if the client requests it's agent be forwarded to
	// the server.
	ForwardAgent bool
	// JumpHost turns on jump host mode
	JumpHost bool
}

func (i *TeleInstance) Stop(removeData bool) error {
	if i.Config != nil && removeData {
		err := os.RemoveAll(i.Config.DataDir)
		if err != nil {
			log.Error("failed removing temporary local Teleport directory", err)
		}
	}

	log.Infof("Asking Teleport to stop")
	err := i.Process.Close()
	if err != nil {
		log.Error(err)
		return trace.Wrap(err)
	}
	defer func() {
		log.Infof("Teleport instance '%v' stopped!", i.Secrets.SiteName)
	}()
	return i.Process.Wait()
}

func startAndWait(process *service.TeleportProcess, expectedEvents []string) ([]service.Event, error) {
	// register to listen for all ready events on the broadcast channel
	broadcastCh := make(chan service.Event)
	for _, eventName := range expectedEvents {
		process.WaitForEvent(context.TODO(), eventName, broadcastCh)
	}

	// start the process
	err := process.Start()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// wait for all events to arrive or a timeout. if all the expected events
	// from above are not received, this instance will not start
	receivedEvents := []service.Event{}
	timeoutCh := time.After(10 * time.Second)

	for idx := 0; idx < len(expectedEvents); idx++ {
		select {
		case e := <-broadcastCh:
			receivedEvents = append(receivedEvents, e)
		case <-timeoutCh:
			return nil, trace.BadParameter("timed out, only %v/%v events received. received: %v, expected: %v",
				len(receivedEvents), len(expectedEvents), receivedEvents, expectedEvents)
		}
	}

	// Not all services follow a non-blocking Start/Wait pattern. This means a
	// *Ready event may be emit slightly before the service actually starts for
	// blocking services. Long term those services should be re-factored, until
	// then sleep for 250ms to handle this situation.
	time.Sleep(250 * time.Millisecond)

	return receivedEvents, nil
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("%v at %v", string(debug.Stack()), err)
	}
}
