/*
Copyright 2015-2020 Gravitational, Inc.

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

package reversetunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/limiter"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/sshca"
	"github.com/gravitational/teleport/lib/sshutils"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

var (
	remoteClustersStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: teleport.MetricRemoteClusters,
			Help: "Number inbound connections from remote clusters and clusters stats",
		},
		[]string{"cluster"},
	)
	trustedClustersStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: teleport.MetricTrustedClusters,
			Help: "Number of tunnels per state",
		},
		[]string{"cluster", "state"},
	)
)

func init() {
	// Metrics have to be registered to be exposed:
	prometheus.MustRegister(remoteClustersStats)
	prometheus.MustRegister(trustedClustersStats)
}

// server is a "reverse tunnel server". it exposes the cluster capabilities
// (like access to a cluster's auth) to remote trusted clients
// (also known as 'reverse tunnel agents'.
type server struct {
	sync.RWMutex
	Config

	// localAuthClient provides access to the full Auth Server API for the
	// local cluster.
	localAuthClient auth.ClientI
	// localAccessPoint provides access to a cached subset of the Auth
	// Server API.
	localAccessPoint auth.AccessPoint

	// srv is the "base class" i.e. the underlying SSH server
	srv     *sshutils.Server
	limiter *limiter.Limiter

	// remoteSites is the list of conencted remote clusters
	remoteSites []*remoteSite

	// localSites is the list of local (our own cluster) tunnel clients,
	// usually each of them is a local proxy.
	localSites []*localSite

	// clusterPeers is a map of clusters connected to peer proxies
	// via reverse tunnels
	clusterPeers map[string]*clusterPeers

	// newAccessPoint returns new caching access point
	newAccessPoint auth.NewCachingAccessPoint

	// cancel function will cancel the
	cancel context.CancelFunc

	// ctx is a context used for signalling and broadcast
	ctx context.Context

	*log.Entry

	// proxyWatcher monitors changes to the proxies
	// and broadcasts updates
	proxyWatcher *services.ProxyWatcher

	// offlineThreshold is how long to wait for a keep alive message before
	// marking a reverse tunnel connection as invalid.
	offlineThreshold time.Duration
}

// DirectCluster is used to access cluster directly
type DirectCluster struct {
	// Name is a cluster name
	Name string
	// Client is a client to the cluster
	Client auth.ClientI
}

// Config is a reverse tunnel server configuration
type Config struct {
	// ID is the ID of this server proxy
	ID string
	// ClusterName is a name of this cluster
	ClusterName string
	// ClientTLS is a TLS config associated with this proxy
	// used to connect to remote auth servers on remote clusters
	ClientTLS *tls.Config
	// Listener is a listener address for reverse tunnel server
	Listener net.Listener
	// HostSigners is a list of host signers
	HostSigners []ssh.Signer
	// HostKeyCallback
	// Limiter is optional request limiter
	Limiter *limiter.Limiter
	// LocalAuthClient provides access to a full AuthClient for the local cluster.
	LocalAuthClient auth.ClientI
	// AccessPoint provides access to a subset of AuthClient of the cluster.
	// AccessPoint caches values and can still return results during connection
	// problems.
	LocalAccessPoint auth.AccessPoint
	// NewCachingAccessPoint returns new caching access points
	// per remote cluster
	NewCachingAccessPoint auth.NewCachingAccessPoint
	// DirectClusters is a list of clusters accessed directly
	DirectClusters []DirectCluster
	// Context is a signalling context
	Context context.Context
	// Clock is a clock used in the server, set up to
	// wall clock if not set
	Clock clockwork.Clock

	// KeyGen is a process wide key generator. It is shared to speed up
	// generation of public/private keypairs.
	KeyGen sshca.Authority

	// Ciphers is a list of ciphers that the server supports. If omitted,
	// the defaults will be used.
	Ciphers []string

	// KEXAlgorithms is a list of key exchange (KEX) algorithms that the
	// server supports. If omitted, the defaults will be used.
	KEXAlgorithms []string

	// MACAlgorithms is a list of message authentication codes (MAC) that
	// the server supports. If omitted the defaults will be used.
	MACAlgorithms []string

	// DataDir is a local server data directory
	DataDir string

	// PollingPeriod specifies polling period for internal sync
	// goroutines, used to speed up sync-ups in tests.
	PollingPeriod time.Duration

	// Component is a component used in logs
	Component string

	// FIPS means Teleport was started in a FedRAMP/FIPS 140-2 compliant
	// configuration.
	FIPS bool

	// Emitter is event emitter
	Emitter events.StreamEmitter

	// DELETE IN: 5.1.
	//
	// Pass in a access point that can be configured with the old access point
	// policy until all clusters are migrated to 5.0 and above.
	NewCachingAccessPointOldProxy auth.NewCachingAccessPoint
}

// CheckAndSetDefaults checks parameters and sets default values
func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.ID == "" {
		return trace.BadParameter("missing parameter ID")
	}
	if cfg.ClusterName == "" {
		return trace.BadParameter("missing parameter ClusterName")
	}
	if cfg.ClientTLS == nil {
		return trace.BadParameter("missing parameter ClientTLS")
	}
	if cfg.Listener == nil {
		return trace.BadParameter("missing parameter Listener")
	}
	if cfg.DataDir == "" {
		return trace.BadParameter("missing parameter DataDir")
	}
	if cfg.Emitter == nil {
		return trace.BadParameter("missing parameter Emitter")
	}
	if cfg.Context == nil {
		cfg.Context = context.TODO()
	}
	if cfg.PollingPeriod == 0 {
		cfg.PollingPeriod = defaults.HighResPollingPeriod
	}
	if cfg.Limiter == nil {
		var err error
		cfg.Limiter, err = limiter.NewLimiter(limiter.Config{})
		if err != nil {
			return trace.Wrap(err)
		}
	}
	if cfg.Clock == nil {
		cfg.Clock = clockwork.NewRealClock()
	}
	if cfg.Component == "" {
		cfg.Component = teleport.Component(teleport.ComponentProxy, teleport.ComponentServer)
	}
	return nil
}

// NewServer creates and returns a reverse tunnel server which is fully
// initialized but hasn't been started yet
func NewServer(cfg Config) (Server, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	clusterConfig, err := cfg.LocalAccessPoint.GetClusterConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	offlineThreshold := time.Duration(clusterConfig.GetKeepAliveCountMax()) * clusterConfig.GetKeepAliveInterval()

	ctx, cancel := context.WithCancel(cfg.Context)

	entry := log.WithFields(log.Fields{
		trace.Component: cfg.Component,
	})
	proxyWatcher, err := services.NewProxyWatcher(services.ProxyWatcherConfig{
		Context:   ctx,
		Component: cfg.Component,
		Client:    cfg.LocalAuthClient,
		Entry:     entry,
		ProxiesC:  make(chan []services.Server, 10),
	})
	if err != nil {
		cancel()
		return nil, trace.Wrap(err)
	}

	srv := &server{
		Config:           cfg,
		localSites:       []*localSite{},
		remoteSites:      []*remoteSite{},
		localAuthClient:  cfg.LocalAuthClient,
		localAccessPoint: cfg.LocalAccessPoint,
		newAccessPoint:   cfg.NewCachingAccessPoint,
		limiter:          cfg.Limiter,
		ctx:              ctx,
		cancel:           cancel,
		proxyWatcher:     proxyWatcher,
		clusterPeers:     make(map[string]*clusterPeers),
		Entry:            entry,
		offlineThreshold: offlineThreshold,
	}

	for _, clusterInfo := range cfg.DirectClusters {
		cluster, err := newlocalSite(srv, clusterInfo.Name, clusterInfo.Client)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		srv.localSites = append(srv.localSites, cluster)
	}

	s, err := sshutils.NewServer(
		teleport.ComponentReverseTunnelServer,
		// TODO(klizhentas): improve interface, use struct instead of parameter list
		// this address is not used
		utils.NetAddr{Addr: "127.0.0.1:1", AddrNetwork: "tcp"},
		srv,
		cfg.HostSigners,
		sshutils.AuthMethods{
			PublicKey: srv.keyAuth,
		},
		sshutils.SetLimiter(cfg.Limiter),
		sshutils.SetCiphers(cfg.Ciphers),
		sshutils.SetKEXAlgorithms(cfg.KEXAlgorithms),
		sshutils.SetMACAlgorithms(cfg.MACAlgorithms),
		sshutils.SetFIPS(cfg.FIPS),
	)
	if err != nil {
		return nil, err
	}
	srv.srv = s
	go srv.periodicFunctions()
	return srv, nil
}

func remoteClustersMap(rc []services.RemoteCluster) map[string]services.RemoteCluster {
	out := make(map[string]services.RemoteCluster)
	for i := range rc {
		out[rc[i].GetName()] = rc[i]
	}
	return out
}

// disconnectClusters disconnects reverse tunnel connections from remote clusters
// that were deleted from the the local cluster side and cleans up in memory objects.
// In this case all local trust has been deleted, so all the tunnel connections have to be dropped.
func (s *server) disconnectClusters() error {
	connectedRemoteClusters := s.getRemoteClusters()
	if len(connectedRemoteClusters) == 0 {
		return nil
	}
	remoteClusters, err := s.localAuthClient.GetRemoteClusters()
	if err != nil {
		return trace.Wrap(err)
	}
	remoteMap := remoteClustersMap(remoteClusters)
	for _, cluster := range connectedRemoteClusters {
		if _, ok := remoteMap[cluster.GetName()]; !ok {
			s.Infof("Remote cluster %q has been deleted. Disconnecting it from the proxy.", cluster.GetName())
			s.removeSite(cluster.GetName())
			err := cluster.Close()
			if err != nil {
				s.Debugf("Failure closing cluster %q: %v.", cluster.GetName(), err)
			}
		}
	}
	return nil
}

func (s *server) periodicFunctions() {
	ticker := time.NewTicker(defaults.ResyncInterval)
	defer ticker.Stop()

	if err := s.fetchClusterPeers(); err != nil {
		s.Warningf("Failed to fetch cluster peers: %v.", err)
	}
	for {
		select {
		case <-s.ctx.Done():
			s.Debugf("Closing.")
			return
		// Proxies have been updated, notify connected agents about the update.
		case proxies := <-s.proxyWatcher.ProxiesC:
			s.fanOutProxies(proxies)
		case <-ticker.C:
			err := s.fetchClusterPeers()
			if err != nil {
				s.Warningf("Failed to fetch cluster peers: %v.", err)
			}
			err = s.disconnectClusters()
			if err != nil {
				s.Warningf("Failed to disconnect clusters: %v.", err)
			}
			err = s.reportClusterStats()
			if err != nil {
				s.Warningf("Failed to report cluster stats: %v.", err)
			}
		}
	}
}

// fetchClusterPeers pulls back all proxies that have registered themselves
// (created a services.TunnelConnection) in the backend and compares them to
// what was found in the previous iteration and updates the in-memory cluster
// peer map. This map is used later by GetSite(s) to return either local or
// remote site, or if non match, a cluster peer.
func (s *server) fetchClusterPeers() error {
	conns, err := s.LocalAccessPoint.GetAllTunnelConnections()
	if err != nil {
		return trace.Wrap(err)
	}
	newConns := make(map[string]services.TunnelConnection)
	for i := range conns {
		newConn := conns[i]
		// Filter out non-proxy tunnels.
		if newConn.GetType() != services.ProxyTunnel {
			continue
		}
		// Filter out peer records for own proxy.
		if newConn.GetProxyName() == s.ID {
			continue
		}
		newConns[newConn.GetName()] = newConn
	}
	existingConns := s.existingConns()
	connsToAdd, connsToUpdate, connsToRemove := s.diffConns(newConns, existingConns)
	s.removeClusterPeers(connsToRemove)
	s.updateClusterPeers(connsToUpdate)
	return s.addClusterPeers(connsToAdd)
}

func (s *server) reportClusterStats() error {
	clusters, err := s.GetSites()
	if err != nil {
		return trace.Wrap(err)
	}
	for _, cluster := range clusters {
		if _, ok := cluster.(*localSite); ok {
			// Don't count local cluster tunnels.
			continue
		}
		gauge, err := remoteClustersStats.GetMetricWithLabelValues(cluster.GetName())
		if err != nil {
			return trace.Wrap(err)
		}
		gauge.Set(float64(cluster.GetTunnelsCount()))
	}
	return nil
}

func (s *server) addClusterPeers(conns map[string]services.TunnelConnection) error {
	for key := range conns {
		connInfo := conns[key]
		peer, err := newClusterPeer(s, connInfo, s.offlineThreshold)
		if err != nil {
			return trace.Wrap(err)
		}
		s.addClusterPeer(peer)
	}
	return nil
}

func (s *server) updateClusterPeers(conns map[string]services.TunnelConnection) {
	for key := range conns {
		connInfo := conns[key]
		s.updateClusterPeer(connInfo)
	}
}

func (s *server) addClusterPeer(peer *clusterPeer) {
	s.Lock()
	defer s.Unlock()
	clusterName := peer.connInfo.GetClusterName()
	peers, ok := s.clusterPeers[clusterName]
	if !ok {
		peers = newClusterPeers(clusterName)
		s.clusterPeers[clusterName] = peers
	}
	peers.addPeer(peer)
}

func (s *server) updateClusterPeer(conn services.TunnelConnection) bool {
	s.Lock()
	defer s.Unlock()
	clusterName := conn.GetClusterName()
	peers, ok := s.clusterPeers[clusterName]
	if !ok {
		return false
	}
	return peers.updatePeer(conn)
}

func (s *server) removeClusterPeers(conns []services.TunnelConnection) {
	s.Lock()
	defer s.Unlock()
	for _, conn := range conns {
		peers, ok := s.clusterPeers[conn.GetClusterName()]
		if !ok {
			s.Warningf("failed to remove cluster peer, not found peers for %v", conn)
			continue
		}
		peers.removePeer(conn)
		s.Debugf("removed cluster peer %v", conn)
	}
}

func (s *server) existingConns() map[string]services.TunnelConnection {
	s.RLock()
	defer s.RUnlock()
	conns := make(map[string]services.TunnelConnection)
	for _, peers := range s.clusterPeers {
		for _, cluster := range peers.peers {
			conns[cluster.connInfo.GetName()] = cluster.connInfo
		}
	}
	return conns
}

func (s *server) diffConns(newConns, existingConns map[string]services.TunnelConnection) (map[string]services.TunnelConnection, map[string]services.TunnelConnection, []services.TunnelConnection) {
	connsToAdd := make(map[string]services.TunnelConnection)
	connsToUpdate := make(map[string]services.TunnelConnection)
	var connsToRemove []services.TunnelConnection

	for existingKey := range existingConns {
		conn := existingConns[existingKey]
		if _, ok := newConns[existingKey]; !ok { // tunnel was removed
			connsToRemove = append(connsToRemove, conn)
		}
	}

	for newKey := range newConns {
		conn := newConns[newKey]
		if _, ok := existingConns[newKey]; !ok { // tunnel was added
			connsToAdd[newKey] = conn
		} else {
			connsToUpdate[newKey] = conn
		}
	}

	return connsToAdd, connsToUpdate, connsToRemove
}

func (s *server) Wait() {
	s.srv.Wait(context.TODO())
}

func (s *server) Start() error {
	go s.srv.Serve(s.Listener)
	return nil
}

func (s *server) Close() error {
	s.cancel()
	return s.srv.Close()
}

func (s *server) Shutdown(ctx context.Context) error {
	s.cancel()
	return s.srv.Shutdown(ctx)
}

func (s *server) HandleNewChan(ctx context.Context, ccx *sshutils.ConnectionContext, nch ssh.NewChannel) {
	// Apply read/write timeouts to the server connection.
	conn := utils.ObeyIdleTimeout(ccx.NetConn,
		s.offlineThreshold,
		"reverse tunnel server")
	sconn := ccx.ServerConn

	channelType := nch.ChannelType()
	switch channelType {
	// Heartbeats can come from nodes or proxies.
	case chanHeartbeat:
		s.handleHeartbeat(conn, sconn, nch)
	// Transport requests come from nodes requesting a connection to the Auth
	// Server through the reverse tunnel.
	case chanTransport:
		s.handleTransport(sconn, nch)
	default:
		msg := fmt.Sprintf("reversetunnel received unknown channel request %v from %v",
			nch.ChannelType(), sconn)

		// If someone is trying to open a new SSH session by talking to a reverse
		// tunnel, they're most likely using the wrong port number. Give them an
		// explicit hint.
		if channelType == "session" {
			msg = "Cannot open new SSH session on reverse tunnel. Are you connecting to the right port?"
		}
		s.Warn(msg)
		s.rejectRequest(nch, ssh.ConnectionFailed, msg)
		return
	}
}

func (s *server) handleTransport(sconn *ssh.ServerConn, nch ssh.NewChannel) {
	s.Debugf("Transport request: %v.", nch.ChannelType())
	channel, requestCh, err := nch.Accept()
	if err != nil {
		sconn.Close()
		s.Warnf("Failed to accept request: %v.", err)
		return
	}

	t := &transport{
		log:              s.Entry,
		closeContext:     s.ctx,
		authClient:       s.LocalAccessPoint,
		channel:          channel,
		requestCh:        requestCh,
		component:        teleport.ComponentReverseTunnelServer,
		localClusterName: s.ClusterName,
	}
	go t.start()
}

// TODO(awly): unit test this
func (s *server) handleHeartbeat(conn net.Conn, sconn *ssh.ServerConn, nch ssh.NewChannel) {
	s.Debugf("New tunnel from %v.", sconn.RemoteAddr())
	if sconn.Permissions.Extensions[extCertType] != extCertTypeHost {
		s.Error(trace.BadParameter("can't retrieve certificate type in certType"))
		return
	}

	// Extract the role. For proxies, it's another cluster asking to join, for
	// nodes it's a node dialing back.
	val, ok := sconn.Permissions.Extensions[extCertRole]
	if !ok {
		log.Errorf("Failed to accept connection, missing %q extension", extCertRole)
		s.rejectRequest(nch, ssh.ConnectionFailed, "unknown role")
		return
	}

	role := teleport.Role(val)
	switch role {
	// Node is dialing back.
	case teleport.RoleNode:
		s.handleNewService(role, conn, sconn, nch, services.NodeTunnel)
	// App is dialing back.
	case teleport.RoleApp:
		s.handleNewService(role, conn, sconn, nch, services.AppTunnel)
	// Kubernetes service is dialing back.
	case teleport.RoleKube:
		s.handleNewService(role, conn, sconn, nch, services.KubeTunnel)
	// Proxy is dialing back.
	case teleport.RoleProxy:
		s.handleNewCluster(conn, sconn, nch)
	// Unknown role.
	default:
		log.Errorf("Unsupported role attempting to connect: %v", val)
		s.rejectRequest(nch, ssh.ConnectionFailed, fmt.Sprintf("unsupported role %v", val))
	}
}

func (s *server) handleNewService(role teleport.Role, conn net.Conn, sconn *ssh.ServerConn, nch ssh.NewChannel, connType services.TunnelType) {
	cluster, rconn, err := s.upsertServiceConn(conn, sconn, connType)
	if err != nil {
		log.Errorf("Failed to upsert %s: %v.", role, err)
		sconn.Close()
		return
	}

	ch, req, err := nch.Accept()
	if err != nil {
		log.Errorf("Failed to accept on channel: %v.", err)
		sconn.Close()
		return
	}

	go cluster.handleHeartbeat(rconn, ch, req)
}

func (s *server) handleNewCluster(conn net.Conn, sshConn *ssh.ServerConn, nch ssh.NewChannel) {
	// add the incoming site (cluster) to the list of active connections:
	site, remoteConn, err := s.upsertRemoteCluster(conn, sshConn)
	if err != nil {
		log.Error(trace.Wrap(err))
		s.rejectRequest(nch, ssh.ConnectionFailed, "failed to accept incoming cluster connection")
		return
	}
	// accept the request and start the heartbeat on it:
	ch, req, err := nch.Accept()
	if err != nil {
		log.Error(trace.Wrap(err))
		sshConn.Close()
		return
	}
	go site.handleHeartbeat(remoteConn, ch, req)
}

func (s *server) findLocalCluster(sconn *ssh.ServerConn) (*localSite, error) {
	// Cluster name was extracted from certificate and packed into extensions.
	clusterName := sconn.Permissions.Extensions[extAuthority]
	if strings.TrimSpace(clusterName) == "" {
		return nil, trace.BadParameter("empty cluster name")
	}

	for _, ls := range s.localSites {
		if ls.domainName == clusterName {
			return ls, nil
		}
	}

	return nil, trace.BadParameter("local cluster %v not found", clusterName)
}

func (s *server) getTrustedCAKeysByID(id services.CertAuthID) ([]ssh.PublicKey, error) {
	ca, err := s.localAccessPoint.GetCertAuthority(id, false, services.SkipValidation())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return ca.Checkers()
}

func (s *server) keyAuth(conn ssh.ConnMetadata, key ssh.PublicKey) (perm *ssh.Permissions, err error) {
	logger := s.WithFields(log.Fields{
		"remote": conn.RemoteAddr(),
		"user":   conn.User(),
	})
	// The crypto/x/ssh package won't log the returned error for us, do it
	// manually.
	defer func() {
		if err != nil {
			logger.Warnf("Failed to authenticate client, err: %v.", err)
		}
	}()

	cert, ok := key.(*ssh.Certificate)
	if !ok {
		return nil, trace.BadParameter("server doesn't support provided key type")
	}

	var clusterName, certRole, certType string
	var caType services.CertAuthType
	switch cert.CertType {
	case ssh.HostCert:
		var ok bool
		clusterName, ok = cert.Extensions[utils.CertExtensionAuthority]
		if !ok || clusterName == "" {
			return nil, trace.BadParameter("certificate missing %q extension; this SSH host certificate was not issued by Teleport or issued by an older version of Teleport; try upgrading your Teleport nodes/proxies", utils.CertExtensionAuthority)
		}
		certRole, ok = cert.Extensions[utils.CertExtensionRole]
		if !ok || certRole == "" {
			return nil, trace.BadParameter("certificate missing %q extension; this SSH host certificate was not issued by Teleport or issued by an older version of Teleport; try upgrading your Teleport nodes/proxies", utils.CertExtensionRole)
		}
		certType = extCertTypeHost
		caType = services.HostCA
	case ssh.UserCert:
		var ok bool
		clusterName, ok = cert.Extensions[teleport.CertExtensionTeleportRouteToCluster]
		if !ok || clusterName == "" {
			clusterName = s.ClusterName
		}
		encRoles, ok := cert.Extensions[teleport.CertExtensionTeleportRoles]
		if !ok || encRoles == "" {
			return nil, trace.BadParameter("certificate missing %q extension; this SSH user certificate was not issued by Teleport or issued by an older version of Teleport; try upgrading your Teleport proxies/auth servers and logging in again (or exporting an identity file, if that's what you used)", teleport.CertExtensionTeleportRoles)
		}
		roles, err := services.UnmarshalCertRoles(encRoles)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if len(roles) == 0 {
			return nil, trace.BadParameter("certificate missing roles in %q extension; make sure your user has some roles assigned (or ask your Teleport admin to) and log in again (or export an identity file, if that's what you used)", teleport.CertExtensionTeleportRoles)
		}
		certRole = roles[0]
		certType = extCertTypeUser
		caType = services.UserCA
	default:
		return nil, trace.BadParameter("unsupported cert type: %v.", cert.CertType)
	}

	if err := s.checkClientCert(logger, conn.User(), clusterName, cert, caType); err != nil {
		return nil, trace.Wrap(err)
	}
	return &ssh.Permissions{
		Extensions: map[string]string{
			extHost:      conn.User(),
			extCertType:  certType,
			extCertRole:  certRole,
			extAuthority: clusterName,
		},
	}, nil
}

// checkClientCert verifies that client certificate is signed by the recognized
// certificate authority.
func (s *server) checkClientCert(logger *log.Entry, user string, clusterName string, cert *ssh.Certificate, caType services.CertAuthType) error {
	// fetch keys of the certificate authority to check
	// if there is a match
	keys, err := s.getTrustedCAKeysByID(services.CertAuthID{
		Type:       caType,
		DomainName: clusterName,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	// match key of the certificate authority with the signature key
	var match bool
	for _, k := range keys {
		if sshutils.KeysEqual(k, cert.SignatureKey) {
			match = true
			break
		}
	}
	if !match {
		return trace.NotFound("cluster %v has no matching CA keys", clusterName)
	}

	checker := utils.CertChecker{
		FIPS: s.FIPS,
	}
	if err := checker.CheckCert(user, cert); err != nil {
		return trace.BadParameter(err.Error())
	}

	return nil
}

func (s *server) upsertServiceConn(conn net.Conn, sconn *ssh.ServerConn, connType services.TunnelType) (*localSite, *remoteConn, error) {
	s.Lock()
	defer s.Unlock()

	cluster, err := s.findLocalCluster(sconn)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	nodeID, ok := sconn.Permissions.Extensions[extHost]
	if !ok {
		return nil, nil, trace.BadParameter("host id not found")
	}

	rconn, err := cluster.addConn(nodeID, connType, conn, sconn)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	return cluster, rconn, nil
}

func (s *server) upsertRemoteCluster(conn net.Conn, sshConn *ssh.ServerConn) (*remoteSite, *remoteConn, error) {
	domainName := sshConn.Permissions.Extensions[extAuthority]
	if strings.TrimSpace(domainName) == "" {
		return nil, nil, trace.BadParameter("cannot create reverse tunnel: empty cluster name")
	}

	s.Lock()
	defer s.Unlock()

	var site *remoteSite
	for _, st := range s.remoteSites {
		if st.domainName == domainName {
			site = st
			break
		}
	}
	var err error
	var remoteConn *remoteConn
	if site != nil {
		if remoteConn, err = site.addConn(conn, sshConn); err != nil {
			return nil, nil, trace.Wrap(err)
		}
	} else {
		site, err = newRemoteSite(s, domainName, sshConn.Conn)
		if err != nil {
			return nil, nil, trace.Wrap(err)
		}
		if remoteConn, err = site.addConn(conn, sshConn); err != nil {
			return nil, nil, trace.Wrap(err)
		}
		s.remoteSites = append(s.remoteSites, site)
	}
	site.Infof("Connection <- %v, clusters: %d.", conn.RemoteAddr(), len(s.remoteSites))
	// treat first connection as a registered heartbeat,
	// otherwise the connection information will appear after initial
	// heartbeat delay
	go site.registerHeartbeat(time.Now())
	return site, remoteConn, nil
}

func (s *server) GetSites() ([]RemoteSite, error) {
	s.RLock()
	defer s.RUnlock()
	out := make([]RemoteSite, 0, len(s.remoteSites)+len(s.localSites)+len(s.clusterPeers))
	for i := range s.localSites {
		out = append(out, s.localSites[i])
	}
	haveLocalConnection := make(map[string]bool)
	for i := range s.remoteSites {
		site := s.remoteSites[i]
		haveLocalConnection[site.GetName()] = true
		out = append(out, site)
	}
	for i := range s.clusterPeers {
		cluster := s.clusterPeers[i]
		if _, ok := haveLocalConnection[cluster.GetName()]; !ok {
			out = append(out, cluster)
		}
	}
	return out, nil
}

func (s *server) getRemoteClusters() []*remoteSite {
	s.RLock()
	defer s.RUnlock()
	out := make([]*remoteSite, len(s.remoteSites))
	copy(out, s.remoteSites)
	return out
}

// GetSite returns a RemoteSite. The first attempt is to find and return a
// remote site and that is what is returned if a remote remote agent has
// connected to this proxy. Next we loop over local sites and try and try and
// return a local site. If that fails, we return a cluster peer. This happens
// when you hit proxy that has never had an agent connect to it. If you end up
// with a cluster peer your best bet is to wait until the agent has discovered
// all proxies behind a the load balancer. Note, the cluster peer is a
// services.TunnelConnection that was created by another proxy.
func (s *server) GetSite(name string) (RemoteSite, error) {
	s.RLock()
	defer s.RUnlock()
	for i := range s.remoteSites {
		if s.remoteSites[i].GetName() == name {
			return s.remoteSites[i], nil
		}
	}
	for i := range s.localSites {
		if s.localSites[i].GetName() == name {
			return s.localSites[i], nil
		}
	}
	for i := range s.clusterPeers {
		if s.clusterPeers[i].GetName() == name {
			return s.clusterPeers[i], nil
		}
	}
	return nil, trace.NotFound("cluster %q is not found", name)
}

func (s *server) removeSite(domainName string) error {
	s.Lock()
	defer s.Unlock()
	for i := range s.remoteSites {
		if s.remoteSites[i].domainName == domainName {
			s.remoteSites = append(s.remoteSites[:i], s.remoteSites[i+1:]...)
			return nil
		}
	}
	for i := range s.localSites {
		if s.localSites[i].domainName == domainName {
			s.localSites = append(s.localSites[:i], s.localSites[i+1:]...)
			return nil
		}
	}
	return trace.NotFound("cluster %q is not found", domainName)
}

// fanOutProxies is a non-blocking call that updated the watches proxies
// list and notifies all clusters about the proxy list change
func (s *server) fanOutProxies(proxies []services.Server) {
	s.Lock()
	defer s.Unlock()
	for _, cluster := range s.localSites {
		cluster.fanOutProxies(proxies)
	}
	for _, cluster := range s.remoteSites {
		cluster.fanOutProxies(proxies)
	}
}

func (s *server) rejectRequest(ch ssh.NewChannel, reason ssh.RejectionReason, msg string) {
	if err := ch.Reject(reason, msg); err != nil {
		s.Warnf("Failed rejecting new channel request: %v", err)
	}
}

// newRemoteSite helper creates and initializes 'remoteSite' instance
func newRemoteSite(srv *server, domainName string, sconn ssh.Conn) (*remoteSite, error) {
	connInfo, err := services.NewTunnelConnection(
		fmt.Sprintf("%v-%v", srv.ID, domainName),
		services.TunnelConnectionSpecV2{
			ClusterName:   domainName,
			ProxyName:     srv.ID,
			LastHeartbeat: time.Now().UTC(),
		},
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	closeContext, cancel := context.WithCancel(srv.ctx)
	remoteSite := &remoteSite{
		srv:        srv,
		domainName: domainName,
		connInfo:   connInfo,
		Entry: log.WithFields(log.Fields{
			trace.Component: teleport.ComponentReverseTunnelServer,
			trace.ComponentFields: log.Fields{
				"cluster": domainName,
			},
		}),
		ctx:              closeContext,
		cancel:           cancel,
		clock:            srv.Clock,
		offlineThreshold: srv.offlineThreshold,
	}

	// configure access to the full Auth Server API and the cached subset for
	// the local cluster within which reversetunnel.Server is running.
	remoteSite.localClient = srv.localAuthClient
	remoteSite.localAccessPoint = srv.localAccessPoint

	clt, _, err := remoteSite.getRemoteClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	remoteSite.remoteClient = clt

	// DELETE IN: 5.1.0.
	//
	// Check if the cluster that is connecting is an older cluster. If it is,
	// don't request access to application servers because older servers policy
	// will reject that causing the cache to go into a re-sync loop.
	var accessPointFunc auth.NewCachingAccessPoint
	ok, err := isOldCluster(closeContext, sconn)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if ok {
		log.Debugf("Older cluster connecting, loading old cache policy.")
		accessPointFunc = srv.Config.NewCachingAccessPointOldProxy
	} else {
		accessPointFunc = srv.newAccessPoint
	}

	// Configure access to the cached subset of the Auth Server API of the remote
	// cluster this remote site provides access to.
	accessPoint, err := accessPointFunc(clt, []string{"reverse", domainName})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	remoteSite.remoteAccessPoint = accessPoint

	// instantiate a cache of host certificates for the forwarding server. the
	// certificate cache is created in each site (instead of creating it in
	// reversetunnel.server and passing it along) so that the host certificate
	// is signed by the correct certificate authority.
	certificateCache, err := newHostCertificateCache(srv.Config.KeyGen, srv.localAuthClient)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	remoteSite.certificateCache = certificateCache

	go remoteSite.periodicUpdateCertAuthorities()

	return remoteSite, nil
}

// DELETE IN: 5.1.0.
//
// isOldCluster checks if the cluster is older than 5.0.0.
func isOldCluster(ctx context.Context, conn ssh.Conn) (bool, error) {
	version, err := sendVersionRequest(ctx, conn)
	if err != nil {
		return false, trace.Wrap(err)
	}

	// Return true if the version is older than 5.0.0, the check is actually for
	// 4.5.0, a non-existent version, to allow this check to work during development.
	remoteClusterVersion, err := semver.NewVersion(version)
	if err != nil {
		return false, trace.Wrap(err)
	}
	minClusterVersion, err := semver.NewVersion("4.5.0")
	if err != nil {
		return false, trace.Wrap(err)
	}
	if remoteClusterVersion.LessThan(*minClusterVersion) {
		return true, nil
	}

	return false, nil
}

// sendVersionRequest sends a request for the version remote Teleport cluster.
func sendVersionRequest(ctx context.Context, sconn ssh.Conn) (string, error) {
	errorCh := make(chan error, 1)
	versionCh := make(chan string, 1)

	go func() {
		ok, payload, err := sconn.SendRequest(versionRequest, true, nil)
		if err != nil {
			errorCh <- err
			return
		}
		if !ok {
			errorCh <- trace.BadParameter("no response to %v request", versionRequest)
			return
		}
		versionCh <- string(payload)
	}()

	select {
	case ver := <-versionCh:
		return ver, nil
	case err := <-errorCh:
		return "", trace.Wrap(err)
	case <-time.After(defaults.WaitCopyTimeout):
		return "", trace.BadParameter("timeout waiting for version")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

const (
	extHost         = "host@teleport"
	extCertType     = "certtype@teleport"
	extAuthority    = "auth@teleport"
	extCertTypeHost = "host"
	extCertTypeUser = "user"
	extCertRole     = "role"

	versionRequest = "x-teleport-version"
)
