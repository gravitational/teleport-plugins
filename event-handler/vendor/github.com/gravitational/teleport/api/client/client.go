/*
Copyright 2020-2021 Gravitational, Inc.

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

package client

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/client/webclient"
	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/api/defaults"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/teleport/api/utils"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/gravitational/trace"
	"github.com/gravitational/trace/trail"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	ggzip "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"
)

func init() {
	// gzip is used for gRPC auditStream compression. SetLevel changes the
	// compression level, must be called in initialization, and is not thread safe.
	if err := ggzip.SetLevel(gzip.BestSpeed); err != nil {
		panic(err)
	}
}

// Client is a gRPC Client that connects to a Teleport Auth server either
// locally or over ssh through a Teleport web proxy or tunnel proxy.
//
// This client can be used to cover a variety of Teleport use cases,
// such as programmatically handling access requests, integrating
// with external tools, or dynamically configuring Teleport.
type Client struct {
	// c contains configuration values for the client.
	c Config
	// tlsConfig is the *tls.Config for a successfully connected client.
	tlsConfig *tls.Config
	// dialer is the ContextDialer for a successfully connected client.
	dialer ContextDialer
	// conn is a grpc connection to the auth server.
	conn *grpc.ClientConn
	// grpc is the gRPC client specification for the auth server.
	grpc proto.AuthServiceClient
	// closedFlag is set to indicate that the connnection is closed.
	// It's a pointer to allow the Client struct to be copied.
	closedFlag *int32
	// callOpts configure calls made by this client.
	callOpts []grpc.CallOption
}

// New creates a new API client with an open connection to a Teleport server.
//
// New will try to open a connection with all combinations of addresses and credentials.
// The first successful connection to a server will be used, or an aggregated error will
// be returned if all combinations fail.
//
// cfg.Credentials must be non-empty. One of cfg.Addrs and cfg.Dialer must be non-empty,
// unless LoadProfile is used to fetch Credentials and load a web proxy dialer.
//
// See the example below for usage.
func New(ctx context.Context, cfg Config) (clt *Client, err error) {
	if err = cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	// If cfg.DialInBackground is true, only a single connection is attempted.
	// This option is primarily meant for internal use where the client has
	// direct access to server values that guarantee a successful connection.
	if cfg.DialInBackground {
		return connectInBackground(ctx, cfg)
	}
	return connect(ctx, cfg)
}

// newClient constructs a new client.
func newClient(cfg Config, dialer ContextDialer, tlsConfig *tls.Config) (clt *Client) {
	return &Client{
		c:          cfg,
		dialer:     dialer,
		tlsConfig:  tlsConfig,
		closedFlag: new(int32),
	}
}

// connectInBackground connects the client to the server in the background.
// The client will use the first credentials and the given dialer. If
// no dialer is given, the first address will be used. This address must
// be an auth server address.
func connectInBackground(ctx context.Context, cfg Config) (*Client, error) {
	tlsConfig, err := cfg.Credentials[0].TLSConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dialer := cfg.Dialer
	addr := constants.APIDomain
	if dialer == nil && len(cfg.Addrs) == 0 {
		return nil, trace.BadParameter("must have a Dialer or Addrs in config")
	}
	if dialer == nil {
		dialer = NewDirectDialer(cfg.KeepAlivePeriod, cfg.DialTimeout)
		addr = cfg.Addrs[0]
	}

	clt := newClient(cfg, dialer, tlsConfig)
	if err := clt.dialGRPC(ctx, addr); err != nil {
		return nil, trace.Wrap(err)
	}

	return clt, nil
}

// connect connects the client to the server using the Credentials and
// Dialer/Addresses provided in the client's config. Multiple goroutines are started
// to make dial attempts with different combinations of dialers and credentials. The
// first client to successfully connect is used to populate the client's connection
// attributes. If none successfully connect, an aggregated error is returned.
func connect(ctx context.Context, cfg Config) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	cltChan := make(chan *Client)
	errChan := make(chan error)

	// sendError is used to send errors to errChan with context.
	sendError := func(err error) {
		select {
		case <-ctx.Done():
		case errChan <- trace.Wrap(err):
		}
	}

	// syncConnect is used to concurrently test several connections, and apply
	// the first successful connection to the client's connection attributes.
	syncConnect := func(clt *Client, addr string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := clt.dialGRPC(ctx, addr); err != nil {
				sendError(trace.Wrap(err))
				return
			}

			// If cltChan is empty, send clt.
			select {
			case cltChan <- clt:
			case <-ctx.Done():
				clt.Close()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Connect with provided credentials.
		for _, creds := range cfg.Credentials {
			tlsConfig, err := creds.TLSConfig()
			if err != nil {
				sendError(trace.Wrap(err))
				continue
			}

			sshConfig, err := creds.SSHClientConfig()
			if err != nil && !trace.IsNotImplemented(err) {
				sendError(trace.Wrap(err))
				continue
			}

			// Connect with dialer provided in config.
			if cfg.Dialer != nil {
				clt := newClient(cfg, cfg.Dialer, tlsConfig)
				syncConnect(clt, constants.APIDomain)
			}

			// Connect with dialer provided in creds.
			if dialer, err := creds.Dialer(cfg); err != nil {
				if !trace.IsNotImplemented(err) {
					sendError(trace.Wrap(err))
				}
			} else {
				clt := newClient(cfg, dialer, tlsConfig)
				syncConnect(clt, constants.APIDomain)
			}

			for _, addr := range cfg.Addrs {
				// Connect to auth.
				dialer := NewDirectDialer(cfg.KeepAlivePeriod, cfg.DialTimeout)
				clt := newClient(cfg, dialer, tlsConfig)
				syncConnect(clt, addr)

				// Connect to proxy.
				if sshConfig != nil {
					wg.Add(1)
					go func(addr string) {
						defer wg.Done()
						// Try connecting to web proxy to retrieve tunnel address.
						if pr, err := webclient.Find(ctx, addr, cfg.InsecureAddressDiscovery, nil); err == nil {
							addr = pr.Proxy.SSH.TunnelPublicAddr
						}
						dialer := newTunnelDialer(*sshConfig, cfg.KeepAlivePeriod, cfg.DialTimeout)
						clt := newClient(cfg, dialer, tlsConfig)
						syncConnect(clt, addr)
					}(addr)
				}
			}
		}
	}()

	// Start goroutine to wait for wait group.
	go func() {
		wg.Wait()
		// Close errChan to return errors.
		close(errChan)
	}()

	var errs []error
	for {
		select {
		case clt := <-cltChan:
			return clt, nil
		case err, ok := <-errChan:
			if ok {
				errs = append(errs, err)
				continue
			}
			// errChan is closed, return errors.
			if len(errs) == 0 {
				if len(cfg.Addrs) == 0 && cfg.Dialer == nil {
					// Some credentials don't require these fields. If no errors propagate, then they need to provide these fields.
					return nil, trace.Errorf("all auth methods failed: try providing Addrs or Dialer in config")
				}
				return nil, trace.Errorf("all auth methods failed")
			}
			return nil, trace.Wrap(trace.NewAggregate(errs...), "all auth methods failed")
		case <-ctx.Done():
			return nil, trace.Wrap(ctx.Err())
		}
	}
}

// dialGRPC dials a connection between server and client. If withBlock is true,
// the dial will block until the connection is up, rather than returning
// immediately and connecting to the server in the background.
func (c *Client) dialGRPC(ctx context.Context, addr string) error {
	dialContext, cancel := context.WithTimeout(ctx, c.c.DialTimeout)
	defer cancel()

	dialOptions := append(
		c.c.DialOpts,
		grpc.WithContextDialer(c.grpcDialer()),
		grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConfig)),
	)

	var err error
	if c.conn, err = grpc.DialContext(dialContext, addr, dialOptions...); err != nil {
		return trace.Wrap(err)
	}
	c.grpc = proto.NewAuthServiceClient(c.conn)

	return nil
}

// grpcDialer wraps the client's dialer with a grpcDialer.
func (c *Client) grpcDialer() func(ctx context.Context, addr string) (net.Conn, error) {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		if c.isClosed() {
			return nil, trace.ConnectionProblem(nil, "client is closed")
		}
		conn, err := c.dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, trace.ConnectionProblem(err, "failed to dial: %v", err)
		}
		return conn, nil
	}
}

// Config contains configuration of the client
type Config struct {
	// Addrs is a list of teleport auth/proxy server addresses to dial.
	Addrs []string
	// Credentials are a list of credentials to use when attempting
	// to connect to the server.
	Credentials []Credentials
	// Dialer is a custom dialer used to dial a server. If set, Dialer
	// takes precedence over all other connection options.
	Dialer ContextDialer
	// DialOpts define options for dialing the client connection.
	DialOpts []grpc.DialOption
	// DialInBackground specifies to dial the connection in the background
	// rather than blocking until the connection is up. A predefined Dialer
	// or an auth server address must be provided.
	DialInBackground bool
	// DialTimeout defines how long to attempt dialing before timing out.
	DialTimeout time.Duration
	// KeepAlivePeriod defines period between keep alives.
	KeepAlivePeriod time.Duration
	// KeepAliveCount specifies the amount of missed keep alives
	// to wait for before declaring the connection as broken.
	KeepAliveCount int
	// The web proxy uses a self-signed TLS certificate by default, which
	// requires this field to be set. If the web proxy was provided with
	// signed TLS certificates, this field should not be set.
	InsecureAddressDiscovery bool
}

// CheckAndSetDefaults checks and sets default config values.
func (c *Config) CheckAndSetDefaults() error {
	if len(c.Credentials) == 0 {
		return trace.BadParameter("missing connection credentials")
	}

	if c.KeepAlivePeriod == 0 {
		c.KeepAlivePeriod = defaults.ServerKeepAliveTTL
	}
	if c.KeepAliveCount == 0 {
		c.KeepAliveCount = defaults.KeepAliveCountMax
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = defaults.DefaultDialTimeout
	}

	c.DialOpts = append(c.DialOpts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                c.KeepAlivePeriod,
		Timeout:             c.KeepAlivePeriod * time.Duration(c.KeepAliveCount),
		PermitWithoutStream: true,
	}))
	if !c.DialInBackground {
		c.DialOpts = append(c.DialOpts, grpc.WithBlock())
	}

	return nil
}

// Config returns the tls.Config the client connected with.
func (c *Client) Config() *tls.Config {
	return c.tlsConfig
}

// Dialer returns the ContextDialer the client connected with.
func (c *Client) Dialer() ContextDialer {
	return c.dialer
}

// GetConnection returns GRPC connection.
func (c *Client) GetConnection() *grpc.ClientConn {
	return c.conn
}

// Close closes the Client connection to the auth server.
func (c *Client) Close() error {
	if c.setClosed() && c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return trace.Wrap(err)
	}
	return nil
}

// isClosed returns whether the client is marked as closed.
func (c *Client) isClosed() bool {
	return atomic.LoadInt32(c.closedFlag) == 1
}

// setClosed marks the client as closed and returns true if it was open.
func (c *Client) setClosed() bool {
	return atomic.CompareAndSwapInt32(c.closedFlag, 0, 1)
}

// WithCallOptions returns a copy of the client with the given call options set.
// This function should be used for chaining - client.WithCallOptions().Ping()
func (c *Client) WithCallOptions(opts ...grpc.CallOption) *Client {
	clt := *c
	clt.callOpts = append(clt.callOpts, opts...)
	return &clt
}

// Ping gets basic info about the auth server.
func (c *Client) Ping(ctx context.Context) (proto.PingResponse, error) {
	rsp, err := c.grpc.Ping(ctx, &proto.PingRequest{}, c.callOpts...)
	if err != nil {
		return proto.PingResponse{}, trail.FromGRPC(err)
	}
	return *rsp, nil
}

// UpdateRemoteCluster updates remote cluster from the specified value.
func (c *Client) UpdateRemoteCluster(ctx context.Context, rc types.RemoteCluster) error {
	rcV3, ok := rc.(*types.RemoteClusterV3)
	if !ok {
		return trace.BadParameter("unsupported remote cluster type %T", rcV3)
	}

	_, err := c.grpc.UpdateRemoteCluster(ctx, rcV3, c.callOpts...)
	return trail.FromGRPC(err)
}

// CreateUser creates a new user from the specified descriptor.
func (c *Client) CreateUser(ctx context.Context, user types.User) error {
	userV2, ok := user.(*types.UserV2)
	if !ok {
		return trace.BadParameter("unsupported user type %T", user)
	}

	_, err := c.grpc.CreateUser(ctx, userV2, c.callOpts...)
	return trail.FromGRPC(err)
}

// UpdateUser updates an existing user in a backend.
func (c *Client) UpdateUser(ctx context.Context, user types.User) error {
	userV2, ok := user.(*types.UserV2)
	if !ok {
		return trace.BadParameter("unsupported user type %T", user)
	}

	_, err := c.grpc.UpdateUser(ctx, userV2, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetUser returns a list of usernames registered in the system.
// withSecrets controls whether authentication details are returned.
func (c *Client) GetUser(name string, withSecrets bool) (types.User, error) {
	if name == "" {
		return nil, trace.BadParameter("missing username")
	}
	user, err := c.grpc.GetUser(context.TODO(), &proto.GetUserRequest{
		Name:        name,
		WithSecrets: withSecrets,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return user, nil
}

// GetUsers returns a list of users.
// withSecrets controls whether authentication details are returned.
func (c *Client) GetUsers(withSecrets bool) ([]types.User, error) {
	stream, err := c.grpc.GetUsers(context.TODO(), &proto.GetUsersRequest{
		WithSecrets: withSecrets,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	var users []types.User
	for user, err := stream.Recv(); err != io.EOF; user, err = stream.Recv() {
		if err != nil {
			return nil, trail.FromGRPC(err)
		}
		users = append(users, user)
	}
	return users, nil
}

// DeleteUser deletes a user by name.
func (c *Client) DeleteUser(ctx context.Context, user string) error {
	req := &proto.DeleteUserRequest{Name: user}
	_, err := c.grpc.DeleteUser(ctx, req, c.callOpts...)
	return trail.FromGRPC(err)
}

// GenerateUserCerts takes the public key in the OpenSSH `authorized_keys` plain
// text format, signs it using User Certificate Authority signing key and
// returns the resulting certificates.
func (c *Client) GenerateUserCerts(ctx context.Context, req proto.UserCertsRequest) (*proto.Certs, error) {
	certs, err := c.grpc.GenerateUserCerts(ctx, &req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return certs, nil
}

// EmitAuditEvent sends an auditable event to the auth server.
func (c *Client) EmitAuditEvent(ctx context.Context, event events.AuditEvent) error {
	grpcEvent, err := events.ToOneOf(event)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = c.grpc.EmitAuditEvent(ctx, grpcEvent, c.callOpts...)
	if err != nil {
		return trail.FromGRPC(err)
	}
	return nil
}

// RotateResetPasswordTokenSecrets rotates secrets for a given tokenID.
// It gets called every time a user fetches 2nd-factor secrets during registration attempt.
// This ensures that an attacker that gains the ResetPasswordToken link can not view it,
// extract the OTP key from the QR code, then allow the user to signup with
// the same OTP token.
func (c *Client) RotateResetPasswordTokenSecrets(ctx context.Context, tokenID string) (types.ResetPasswordTokenSecrets, error) {
	secrets, err := c.grpc.RotateResetPasswordTokenSecrets(ctx, &proto.RotateResetPasswordTokenSecretsRequest{
		TokenID: tokenID,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return secrets, nil
}

// GetResetPasswordToken returns a ResetPasswordToken for the specified tokenID.
func (c *Client) GetResetPasswordToken(ctx context.Context, tokenID string) (types.ResetPasswordToken, error) {
	token, err := c.grpc.GetResetPasswordToken(ctx, &proto.GetResetPasswordTokenRequest{
		TokenID: tokenID,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}

	return token, nil
}

// CreateResetPasswordToken creates reset password token.
func (c *Client) CreateResetPasswordToken(ctx context.Context, req *proto.CreateResetPasswordTokenRequest) (types.ResetPasswordToken, error) {
	token, err := c.grpc.CreateResetPasswordToken(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}

	return token, nil
}

// GetAccessRequests retrieves a list of all access requests matching the provided filter.
func (c *Client) GetAccessRequests(ctx context.Context, filter types.AccessRequestFilter) ([]types.AccessRequest, error) {
	rsp, err := c.grpc.GetAccessRequests(ctx, &filter, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	reqs := make([]types.AccessRequest, 0, len(rsp.AccessRequests))
	for _, req := range rsp.AccessRequests {
		reqs = append(reqs, req)
	}
	return reqs, nil
}

// CreateAccessRequest registers a new access request with the auth server.
func (c *Client) CreateAccessRequest(ctx context.Context, req types.AccessRequest) error {
	r, ok := req.(*types.AccessRequestV3)
	if !ok {
		return trace.BadParameter("unexpected access request type %T", req)
	}
	_, err := c.grpc.CreateAccessRequest(ctx, r, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteAccessRequest deletes an access request.
func (c *Client) DeleteAccessRequest(ctx context.Context, reqID string) error {
	_, err := c.grpc.DeleteAccessRequest(ctx, &proto.RequestID{ID: reqID}, c.callOpts...)
	return trail.FromGRPC(err)
}

// SetAccessRequestState updates the state of an existing access request.
func (c *Client) SetAccessRequestState(ctx context.Context, params types.AccessRequestUpdate) error {
	setter := proto.RequestStateSetter{
		ID:          params.RequestID,
		State:       params.State,
		Reason:      params.Reason,
		Annotations: params.Annotations,
		Roles:       params.Roles,
	}
	if d := utils.GetDelegator(ctx); d != "" {
		setter.Delegator = d
	}
	_, err := c.grpc.SetAccessRequestState(ctx, &setter, c.callOpts...)
	return trail.FromGRPC(err)
}

// SubmitAccessReview applies a review to a request and returns the post-application state.
func (c *Client) SubmitAccessReview(ctx context.Context, params types.AccessReviewSubmission) (types.AccessRequest, error) {
	req, err := c.grpc.SubmitAccessReview(ctx, &params, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return req, nil
}

// GetAccessCapabilities requests the access capabilities of a user.
func (c *Client) GetAccessCapabilities(ctx context.Context, req types.AccessCapabilitiesRequest) (*types.AccessCapabilities, error) {
	caps, err := c.grpc.GetAccessCapabilities(ctx, &req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return caps, nil
}

// GetPluginData loads all plugin data matching the supplied filter.
func (c *Client) GetPluginData(ctx context.Context, filter types.PluginDataFilter) ([]types.PluginData, error) {
	seq, err := c.grpc.GetPluginData(ctx, &filter, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	data := make([]types.PluginData, 0, len(seq.PluginData))
	for _, d := range seq.PluginData {
		data = append(data, d)
	}
	return data, nil
}

// UpdatePluginData updates a per-resource PluginData entry.
func (c *Client) UpdatePluginData(ctx context.Context, params types.PluginDataUpdateParams) error {
	_, err := c.grpc.UpdatePluginData(ctx, &params, c.callOpts...)
	return trail.FromGRPC(err)
}

// AcquireSemaphore acquires lease with requested resources from semaphore.
func (c *Client) AcquireSemaphore(ctx context.Context, params types.AcquireSemaphoreRequest) (*types.SemaphoreLease, error) {
	lease, err := c.grpc.AcquireSemaphore(ctx, &params, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return lease, nil
}

// KeepAliveSemaphoreLease updates semaphore lease.
func (c *Client) KeepAliveSemaphoreLease(ctx context.Context, lease types.SemaphoreLease) error {
	_, err := c.grpc.KeepAliveSemaphoreLease(ctx, &lease, c.callOpts...)
	return trail.FromGRPC(err)
}

// CancelSemaphoreLease cancels semaphore lease early.
func (c *Client) CancelSemaphoreLease(ctx context.Context, lease types.SemaphoreLease) error {
	_, err := c.grpc.CancelSemaphoreLease(ctx, &lease, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetSemaphores returns a list of all semaphores matching the supplied filter.
func (c *Client) GetSemaphores(ctx context.Context, filter types.SemaphoreFilter) ([]types.Semaphore, error) {
	rsp, err := c.grpc.GetSemaphores(ctx, &filter, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	sems := make([]types.Semaphore, 0, len(rsp.Semaphores))
	for _, s := range rsp.Semaphores {
		sems = append(sems, s)
	}
	return sems, nil
}

// DeleteSemaphore deletes a semaphore matching the supplied filter.
func (c *Client) DeleteSemaphore(ctx context.Context, filter types.SemaphoreFilter) error {
	_, err := c.grpc.DeleteSemaphore(ctx, &filter, c.callOpts...)
	return trail.FromGRPC(err)
}

// UpsertKubeService is used by kubernetes services to report their presence
// to other auth servers in form of hearbeat expiring after ttl period.
func (c *Client) UpsertKubeService(ctx context.Context, s types.Server) error {
	server, ok := s.(*types.ServerV2)
	if !ok {
		return trace.BadParameter("invalid type %T, expected *types.ServerV2", server)
	}
	_, err := c.grpc.UpsertKubeService(ctx, &proto.UpsertKubeServiceRequest{
		Server: server,
	}, c.callOpts...)
	return trace.Wrap(err)
}

// GetKubeServices returns the list of kubernetes services registered in the
// cluster.
func (c *Client) GetKubeServices(ctx context.Context) ([]types.Server, error) {
	resp, err := c.grpc.GetKubeServices(ctx, &proto.GetKubeServicesRequest{}, c.callOpts...)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var servers []types.Server
	for _, server := range resp.GetServers() {
		servers = append(servers, server)
	}
	return servers, nil
}

// GetAppServers gets all application servers.
func (c *Client) GetAppServers(ctx context.Context, namespace string, skipValidation bool) ([]types.Server, error) {
	resp, err := c.grpc.GetAppServers(ctx, &proto.GetAppServersRequest{
		Namespace:      namespace,
		SkipValidation: skipValidation,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}

	var servers []types.Server
	for _, server := range resp.GetServers() {
		servers = append(servers, server)
	}

	return servers, nil
}

// UpsertAppServer adds an application server.
func (c *Client) UpsertAppServer(ctx context.Context, server types.Server) (*types.KeepAlive, error) {
	s, ok := server.(*types.ServerV2)
	if !ok {
		return nil, trace.BadParameter("invalid type %T", server)
	}

	keepAlive, err := c.grpc.UpsertAppServer(ctx, &proto.UpsertAppServerRequest{
		Server: s,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return keepAlive, nil
}

// DeleteAppServer removes an application server.
func (c *Client) DeleteAppServer(ctx context.Context, namespace string, name string) error {
	_, err := c.grpc.DeleteAppServer(ctx, &proto.DeleteAppServerRequest{
		Namespace: namespace,
		Name:      name,
	}, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteAllAppServers removes all application servers.
func (c *Client) DeleteAllAppServers(ctx context.Context, namespace string) error {
	_, err := c.grpc.DeleteAllAppServers(ctx, &proto.DeleteAllAppServersRequest{
		Namespace: namespace,
	}, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetAppSession gets an application web session.
func (c *Client) GetAppSession(ctx context.Context, req types.GetAppSessionRequest) (types.WebSession, error) {
	resp, err := c.grpc.GetAppSession(ctx, &proto.GetAppSessionRequest{
		SessionID: req.SessionID,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}

	return resp.GetSession(), nil
}

// GetAppSessions gets all application web sessions.
func (c *Client) GetAppSessions(ctx context.Context) ([]types.WebSession, error) {
	resp, err := c.grpc.GetAppSessions(ctx, &empty.Empty{}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}

	out := make([]types.WebSession, 0, len(resp.GetSessions()))
	for _, v := range resp.GetSessions() {
		out = append(out, v)
	}
	return out, nil
}

// CreateAppSession creates an application web session. Application web
// sessions represent a browser session the client holds.
func (c *Client) CreateAppSession(ctx context.Context, req types.CreateAppSessionRequest) (types.WebSession, error) {
	resp, err := c.grpc.CreateAppSession(ctx, &proto.CreateAppSessionRequest{
		Username:    req.Username,
		PublicAddr:  req.PublicAddr,
		ClusterName: req.ClusterName,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}

	return resp.GetSession(), nil
}

// DeleteAppSession removes an application web session.
func (c *Client) DeleteAppSession(ctx context.Context, req types.DeleteAppSessionRequest) error {
	_, err := c.grpc.DeleteAppSession(ctx, &proto.DeleteAppSessionRequest{
		SessionID: req.SessionID,
	}, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteAllAppSessions removes all application web sessions.
func (c *Client) DeleteAllAppSessions(ctx context.Context) error {
	_, err := c.grpc.DeleteAllAppSessions(ctx, &empty.Empty{}, c.callOpts...)
	return trail.FromGRPC(err)
}

// GenerateAppToken creates a JWT token with application access.
func (c *Client) GenerateAppToken(ctx context.Context, req types.GenerateAppTokenRequest) (string, error) {
	resp, err := c.grpc.GenerateAppToken(ctx, &proto.GenerateAppTokenRequest{
		Username: req.Username,
		Roles:    req.Roles,
		URI:      req.URI,
		Expires:  req.Expires,
	})
	if err != nil {
		return "", trail.FromGRPC(err)
	}

	return resp.GetToken(), nil
}

// DeleteKubeService deletes a named kubernetes service.
func (c *Client) DeleteKubeService(ctx context.Context, name string) error {
	_, err := c.grpc.DeleteKubeService(ctx, &proto.DeleteKubeServiceRequest{
		Name: name,
	}, c.callOpts...)
	return trace.Wrap(err)
}

// DeleteAllKubeServices deletes all registered kubernetes services.
func (c *Client) DeleteAllKubeServices(ctx context.Context) error {
	_, err := c.grpc.DeleteAllKubeServices(ctx, &proto.DeleteAllKubeServicesRequest{}, c.callOpts...)
	return trace.Wrap(err)
}

// GetDatabaseServers returns all registered database proxy servers.
func (c *Client) GetDatabaseServers(ctx context.Context, namespace string, skipValidation bool) ([]types.DatabaseServer, error) {
	resp, err := c.grpc.GetDatabaseServers(ctx, &proto.GetDatabaseServersRequest{
		Namespace:      namespace,
		SkipValidation: skipValidation,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	servers := make([]types.DatabaseServer, 0, len(resp.GetServers()))
	for _, server := range resp.GetServers() {
		servers = append(servers, server)
	}
	return servers, nil
}

// UpsertDatabaseServer registers a new database proxy server.
func (c *Client) UpsertDatabaseServer(ctx context.Context, server types.DatabaseServer) (*types.KeepAlive, error) {
	s, ok := server.(*types.DatabaseServerV3)
	if !ok {
		return nil, trace.BadParameter("invalid type %T", server)
	}
	keepAlive, err := c.grpc.UpsertDatabaseServer(ctx, &proto.UpsertDatabaseServerRequest{
		Server: s,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return keepAlive, nil
}

// DeleteDatabaseServer removes the specified database proxy server.
func (c *Client) DeleteDatabaseServer(ctx context.Context, namespace, hostID, name string) error {
	_, err := c.grpc.DeleteDatabaseServer(ctx, &proto.DeleteDatabaseServerRequest{
		Namespace: namespace,
		HostID:    hostID,
		Name:      name,
	}, c.callOpts...)
	if err != nil {
		return trail.FromGRPC(err)
	}
	return nil
}

// DeleteAllDatabaseServers removes all registered database proxy servers.
func (c *Client) DeleteAllDatabaseServers(ctx context.Context, namespace string) error {
	_, err := c.grpc.DeleteAllDatabaseServers(ctx, &proto.DeleteAllDatabaseServersRequest{
		Namespace: namespace,
	}, c.callOpts...)
	if err != nil {
		return trail.FromGRPC(err)
	}
	return nil
}

// SignDatabaseCSR generates a client certificate used by proxy when talking
// to a remote database service.
func (c *Client) SignDatabaseCSR(ctx context.Context, req *proto.DatabaseCSRRequest) (*proto.DatabaseCSRResponse, error) {
	resp, err := c.grpc.SignDatabaseCSR(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GenerateDatabaseCert generates client certificate used by a database
// service to authenticate with the database instance.
func (c *Client) GenerateDatabaseCert(ctx context.Context, req *proto.DatabaseCertRequest) (*proto.DatabaseCertResponse, error) {
	resp, err := c.grpc.GenerateDatabaseCert(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetRole returns role by name
func (c *Client) GetRole(ctx context.Context, name string) (types.Role, error) {
	if name == "" {
		return nil, trace.BadParameter("missing name")
	}
	resp, err := c.grpc.GetRole(ctx, &proto.GetRoleRequest{Name: name}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetRoles returns a list of roles
func (c *Client) GetRoles(ctx context.Context) ([]types.Role, error) {
	resp, err := c.grpc.GetRoles(ctx, &empty.Empty{}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	roles := make([]types.Role, 0, len(resp.GetRoles()))
	for _, role := range resp.GetRoles() {
		roles = append(roles, role)
	}
	return roles, nil
}

// UpsertRole creates or updates role
func (c *Client) UpsertRole(ctx context.Context, role types.Role) error {
	roleV3, ok := role.(*types.RoleV3)
	if !ok {
		return trace.BadParameter("invalid type %T", role)
	}
	_, err := c.grpc.UpsertRole(ctx, roleV3, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteRole deletes role by name
func (c *Client) DeleteRole(ctx context.Context, name string) error {
	if name == "" {
		return trace.BadParameter("missing name")
	}
	_, err := c.grpc.DeleteRole(ctx, &proto.DeleteRoleRequest{Name: name}, c.callOpts...)
	return trail.FromGRPC(err)
}

func (c *Client) AddMFADevice(ctx context.Context) (proto.AuthService_AddMFADeviceClient, error) {
	stream, err := c.grpc.AddMFADevice(ctx, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return stream, nil
}

func (c *Client) DeleteMFADevice(ctx context.Context) (proto.AuthService_DeleteMFADeviceClient, error) {
	stream, err := c.grpc.DeleteMFADevice(ctx, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return stream, nil
}

func (c *Client) GetMFADevices(ctx context.Context, in *proto.GetMFADevicesRequest) (*proto.GetMFADevicesResponse, error) {
	resp, err := c.grpc.GetMFADevices(ctx, in, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

func (c *Client) GenerateUserSingleUseCerts(ctx context.Context) (proto.AuthService_GenerateUserSingleUseCertsClient, error) {
	stream, err := c.grpc.GenerateUserSingleUseCerts(ctx, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return stream, nil
}

func (c *Client) IsMFARequired(ctx context.Context, req *proto.IsMFARequiredRequest) (*proto.IsMFARequiredResponse, error) {
	resp, err := c.grpc.IsMFARequired(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetOIDCConnector returns an OIDC connector by name.
func (c *Client) GetOIDCConnector(ctx context.Context, name string, withSecrets bool) (types.OIDCConnector, error) {
	if name == "" {
		return nil, trace.BadParameter("cannot get OIDC Connector, missing name")
	}
	req := &types.ResourceWithSecretsRequest{Name: name, WithSecrets: withSecrets}
	resp, err := c.grpc.GetOIDCConnector(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetOIDCConnectors returns a list of OIDC connectors.
func (c *Client) GetOIDCConnectors(ctx context.Context, withSecrets bool) ([]types.OIDCConnector, error) {
	req := &types.ResourcesWithSecretsRequest{WithSecrets: withSecrets}
	resp, err := c.grpc.GetOIDCConnectors(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	oidcConnectors := make([]types.OIDCConnector, len(resp.OIDCConnectors))
	for i, oidcConnector := range resp.OIDCConnectors {
		oidcConnectors[i] = oidcConnector
	}
	return oidcConnectors, nil
}

// UpsertOIDCConnector creates or updates an OIDC connector.
func (c *Client) UpsertOIDCConnector(ctx context.Context, oidcConnector types.OIDCConnector) error {
	oidcConnectorV2, ok := oidcConnector.(*types.OIDCConnectorV2)
	if !ok {
		return trace.BadParameter("invalid type %T", oidcConnector)
	}
	_, err := c.grpc.UpsertOIDCConnector(ctx, oidcConnectorV2, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteOIDCConnector deletes an OIDC connector by name.
func (c *Client) DeleteOIDCConnector(ctx context.Context, name string) error {
	if name == "" {
		return trace.BadParameter("cannot delete OIDC Connector, missing name")
	}
	_, err := c.grpc.DeleteOIDCConnector(ctx, &types.ResourceRequest{Name: name}, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetSAMLConnector returns a SAML connector by name.
func (c *Client) GetSAMLConnector(ctx context.Context, name string, withSecrets bool) (types.SAMLConnector, error) {
	if name == "" {
		return nil, trace.BadParameter("cannot get SAML Connector, missing name")
	}
	req := &types.ResourceWithSecretsRequest{Name: name, WithSecrets: withSecrets}
	resp, err := c.grpc.GetSAMLConnector(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetSAMLConnectors returns a list of SAML connectors.
func (c *Client) GetSAMLConnectors(ctx context.Context, withSecrets bool) ([]types.SAMLConnector, error) {
	req := &types.ResourcesWithSecretsRequest{WithSecrets: withSecrets}
	resp, err := c.grpc.GetSAMLConnectors(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	samlConnectors := make([]types.SAMLConnector, len(resp.SAMLConnectors))
	for i, samlConnector := range resp.SAMLConnectors {
		samlConnectors[i] = samlConnector
	}
	return samlConnectors, nil
}

// UpsertSAMLConnector creates or updates a SAML connector.
func (c *Client) UpsertSAMLConnector(ctx context.Context, connector types.SAMLConnector) error {
	samlConnectorV2, ok := connector.(*types.SAMLConnectorV2)
	if !ok {
		return trace.BadParameter("invalid type %T", connector)
	}
	_, err := c.grpc.UpsertSAMLConnector(ctx, samlConnectorV2, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteSAMLConnector deletes a SAML connector by name.
func (c *Client) DeleteSAMLConnector(ctx context.Context, name string) error {
	if name == "" {
		return trace.BadParameter("cannot delete SAML Connector, missing name")
	}
	_, err := c.grpc.DeleteSAMLConnector(ctx, &types.ResourceRequest{Name: name}, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetGithubConnector returns a Github connector by name.
func (c *Client) GetGithubConnector(ctx context.Context, name string, withSecrets bool) (types.GithubConnector, error) {
	if name == "" {
		return nil, trace.BadParameter("cannot get Github Connector, missing name")
	}
	req := &types.ResourceWithSecretsRequest{Name: name, WithSecrets: withSecrets}
	resp, err := c.grpc.GetGithubConnector(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetGithubConnectors returns a list of Github connectors.
func (c *Client) GetGithubConnectors(ctx context.Context, withSecrets bool) ([]types.GithubConnector, error) {
	req := &types.ResourcesWithSecretsRequest{WithSecrets: withSecrets}
	resp, err := c.grpc.GetGithubConnectors(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	githubConnectors := make([]types.GithubConnector, len(resp.GithubConnectors))
	for i, githubConnector := range resp.GithubConnectors {
		githubConnectors[i] = githubConnector
	}
	return githubConnectors, nil
}

// UpsertGithubConnector creates or updates a Github connector.
func (c *Client) UpsertGithubConnector(ctx context.Context, connector types.GithubConnector) error {
	githubConnector, ok := connector.(*types.GithubConnectorV3)
	if !ok {
		return trace.BadParameter("invalid type %T", connector)
	}
	_, err := c.grpc.UpsertGithubConnector(ctx, githubConnector, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteGithubConnector deletes a Github connector by name.
func (c *Client) DeleteGithubConnector(ctx context.Context, name string) error {
	if name == "" {
		return trace.BadParameter("cannot delete Github Connector, missing name")
	}
	_, err := c.grpc.DeleteGithubConnector(ctx, &types.ResourceRequest{Name: name}, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetTrustedCluster returns a Trusted Cluster by name.
func (c *Client) GetTrustedCluster(ctx context.Context, name string) (types.TrustedCluster, error) {
	if name == "" {
		return nil, trace.BadParameter("cannot get trusted cluster, missing name")
	}
	req := &types.ResourceRequest{Name: name}
	resp, err := c.grpc.GetTrustedCluster(ctx, req, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetTrustedClusters returns a list of Trusted Clusters.
func (c *Client) GetTrustedClusters(ctx context.Context) ([]types.TrustedCluster, error) {
	resp, err := c.grpc.GetTrustedClusters(ctx, &empty.Empty{}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	trustedClusters := make([]types.TrustedCluster, len(resp.TrustedClusters))
	for i, trustedCluster := range resp.TrustedClusters {
		trustedClusters[i] = trustedCluster
	}
	return trustedClusters, nil
}

// UpsertTrustedCluster creates or updates a Trusted Cluster.
func (c *Client) UpsertTrustedCluster(ctx context.Context, trusedCluster types.TrustedCluster) (types.TrustedCluster, error) {
	trustedCluster, ok := trusedCluster.(*types.TrustedClusterV2)
	if !ok {
		return nil, trace.BadParameter("invalid type %T", trusedCluster)
	}
	resp, err := c.grpc.UpsertTrustedCluster(ctx, trustedCluster, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// DeleteTrustedCluster deletes a Trusted Cluster by name.
func (c *Client) DeleteTrustedCluster(ctx context.Context, name string) error {
	if name == "" {
		return trace.BadParameter("cannot delete trusted cluster, missing name")
	}
	_, err := c.grpc.DeleteTrustedCluster(ctx, &types.ResourceRequest{Name: name}, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetToken returns a provision token by name.
func (c *Client) GetToken(ctx context.Context, name string) (types.ProvisionToken, error) {
	if name == "" {
		return nil, trace.BadParameter("cannot get token, missing name")
	}
	resp, err := c.grpc.GetToken(ctx, &types.ResourceRequest{Name: name}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetTokens returns a list of active provision tokens for nodes and users.
func (c *Client) GetTokens(ctx context.Context) ([]types.ProvisionToken, error) {
	resp, err := c.grpc.GetTokens(ctx, &empty.Empty{}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}

	tokens := make([]types.ProvisionToken, len(resp.ProvisionTokens))
	for i, token := range resp.ProvisionTokens {
		tokens[i] = token
	}
	return tokens, nil
}

// UpsertToken creates or updates a provision token.
func (c *Client) UpsertToken(ctx context.Context, token types.ProvisionToken) error {
	tokenV2, ok := token.(*types.ProvisionTokenV2)
	if !ok {
		return trace.BadParameter("invalid type %T", token)
	}
	_, err := c.grpc.UpsertToken(ctx, tokenV2, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteToken deletes a provision token by name.
func (c *Client) DeleteToken(ctx context.Context, name string) error {
	if name == "" {
		return trace.BadParameter("cannot delete token, missing name")
	}
	_, err := c.grpc.DeleteToken(ctx, &types.ResourceRequest{Name: name}, c.callOpts...)
	return trail.FromGRPC(err)
}

// GetNode returns a node by name and namespace.
func (c *Client) GetNode(ctx context.Context, namespace, name string) (types.Server, error) {
	resp, err := c.grpc.GetNode(ctx, &types.ResourceInNamespaceRequest{
		Name:      name,
		Namespace: namespace,
	}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// GetNodes returns a list of nodes by namespace.
// Nodes that the user doesn't have access to are filtered out.
func (c *Client) GetNodes(ctx context.Context, namespace string) ([]types.Server, error) {
	if namespace == "" {
		return nil, trace.BadParameter("missing parameter namespace")
	}
	resp, err := c.grpc.GetNodes(ctx, &types.ResourcesInNamespaceRequest{Namespace: namespace}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	nodes := make([]types.Server, len(resp.Servers))
	for i, node := range resp.Servers {
		nodes[i] = node
	}
	return nodes, nil
}

// UpsertNode is used by SSH servers to report their presence
// to the auth servers in form of heartbeat expiring after ttl period.
func (c *Client) UpsertNode(ctx context.Context, node types.Server) (*types.KeepAlive, error) {
	if node.GetNamespace() == "" {
		return nil, trace.BadParameter("missing node namespace")
	}
	serverV2, ok := node.(*types.ServerV2)
	if !ok {
		return nil, trace.BadParameter("invalid type %T", node)
	}
	keepAlive, err := c.grpc.UpsertNode(ctx, serverV2, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return keepAlive, nil
}

// DeleteNode deletes a node by name and namespace.
func (c *Client) DeleteNode(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		return trace.BadParameter("missing parameter namespace")
	}
	if name == "" {
		return trace.BadParameter("missing parameter name")
	}
	_, err := c.grpc.DeleteNode(ctx, &types.ResourceInNamespaceRequest{
		Name:      name,
		Namespace: namespace,
	}, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteAllNodes deletes all nodes in a given namespace.
func (c *Client) DeleteAllNodes(ctx context.Context, namespace string) error {
	if namespace == "" {
		return trace.BadParameter("missing parameter namespace")
	}
	_, err := c.grpc.DeleteAllNodes(ctx, &types.ResourcesInNamespaceRequest{Namespace: namespace}, c.callOpts...)
	return trail.FromGRPC(err)
}

// SearchEvents allows searching for events with a full pagination support.
func (c *Client) SearchEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, startKey string) ([]events.AuditEvent, string, error) {
	request := &proto.GetEventsRequest{
		Namespace:  namespace,
		StartDate:  fromUTC,
		EndDate:    toUTC,
		EventTypes: eventTypes,
		Limit:      int32(limit),
		StartKey:   startKey,
	}

	response, err := c.grpc.GetEvents(ctx, request)
	if err != nil {
		return nil, "", trail.FromGRPC(err)
	}

	decodedEvents := make([]events.AuditEvent, 0, len(response.Items))
	for _, rawEvent := range response.Items {
		event, err := events.FromOneOf(*rawEvent)
		if err != nil {
			return nil, "", trace.Wrap(err)
		}
		decodedEvents = append(decodedEvents, event)
	}

	return decodedEvents, response.LastKey, nil
}

// SearchSessionEvents allows searching for session events with a full pagination support.
func (c *Client) SearchSessionEvents(ctx context.Context, fromUTC time.Time, toUTC time.Time, limit int, startKey string) ([]events.AuditEvent, string, error) {
	request := &proto.GetSessionEventsRequest{
		StartDate: fromUTC,
		EndDate:   toUTC,
		Limit:     int32(limit),
		StartKey:  startKey,
	}

	response, err := c.grpc.GetSessionEvents(ctx, request)
	if err != nil {
		return nil, "", trail.FromGRPC(err)
	}

	decodedEvents := make([]events.AuditEvent, 0, len(response.Items))
	for _, rawEvent := range response.Items {
		event, err := events.FromOneOf(*rawEvent)
		if err != nil {
			return nil, "", trace.Wrap(err)
		}
		decodedEvents = append(decodedEvents, event)
	}

	return decodedEvents, response.LastKey, nil
}

// GetClusterNetworkingConfig gets cluster networking configuration.
func (c *Client) GetClusterNetworkingConfig(ctx context.Context) (types.ClusterNetworkingConfig, error) {
	resp, err := c.grpc.GetClusterNetworkingConfig(ctx, &empty.Empty{}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// SetClusterNetworkingConfig sets cluster networking configuration.
func (c *Client) SetClusterNetworkingConfig(ctx context.Context, netConfig types.ClusterNetworkingConfig) error {
	netConfigV2, ok := netConfig.(*types.ClusterNetworkingConfigV2)
	if !ok {
		return trace.BadParameter("invalid type %T", netConfig)
	}
	_, err := c.grpc.SetClusterNetworkingConfig(ctx, netConfigV2, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteClusterNetworkingConfig not implemented: can only be called locally.
func (c *Client) DeleteClusterNetworkingConfig(ctx context.Context) error {
	return trace.NotImplemented(notImplementedMessage)
}

// GetSessionRecordingConfig gets session recording configuration.
func (c *Client) GetSessionRecordingConfig(ctx context.Context) (types.SessionRecordingConfig, error) {
	resp, err := c.grpc.GetSessionRecordingConfig(ctx, &empty.Empty{}, c.callOpts...)
	if err != nil {
		return nil, trail.FromGRPC(err)
	}
	return resp, nil
}

// SetSessionRecordingConfig sets session recording configuration.
func (c *Client) SetSessionRecordingConfig(ctx context.Context, recConfig types.SessionRecordingConfig) error {
	recConfigV2, ok := recConfig.(*types.SessionRecordingConfigV2)
	if !ok {
		return trace.BadParameter("invalid type %T", recConfig)
	}
	_, err := c.grpc.SetSessionRecordingConfig(ctx, recConfigV2, c.callOpts...)
	return trail.FromGRPC(err)
}

// DeleteSessionRecordingConfig not implemented: can only be called locally.
func (c *Client) DeleteSessionRecordingConfig(ctx context.Context) error {
	return trace.NotImplemented(notImplementedMessage)
}

// ResetAuthPreference resets cluster auth preference to defaults.
func (c *Client) ResetAuthPreference(ctx context.Context) error {
	_, err := c.grpc.ResetAuthPreference(ctx, &empty.Empty{})
	return trail.FromGRPC(err)
}
