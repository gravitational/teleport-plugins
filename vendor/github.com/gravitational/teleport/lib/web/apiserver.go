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

// Package web implements web proxy handler that provides
// web interface to view and connect to teleport nodes
package web

import (
	"compress/gzip"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/client"
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/httplib"
	"github.com/gravitational/teleport/lib/httplib/csrf"
	"github.com/gravitational/teleport/lib/jwt"
	"github.com/gravitational/teleport/lib/reversetunnel"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/session"
	"github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/teleport/lib/web/app"
	"github.com/gravitational/teleport/lib/web/ui"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/teleport/lib/secret"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/julienschmidt/httprouter"
	lemma_secret "github.com/mailgun/lemma/secret"
	"github.com/sirupsen/logrus"
	"github.com/tstranex/u2f"
	"golang.org/x/crypto/ssh"
)

// Handler is HTTP web proxy handler
type Handler struct {
	log logrus.FieldLogger

	sync.Mutex
	httprouter.Router
	cfg                     Config
	auth                    *sessionCache
	sessionStreamPollPeriod time.Duration
	clock                   clockwork.Clock
	// sshPort specifies the SSH proxy port extracted
	// from configuration
	sshPort string
}

// HandlerOption is a functional argument - an option that can be passed
// to NewHandler function
type HandlerOption func(h *Handler) error

// SetSessionStreamPollPeriod sets polling period for session streams
func SetSessionStreamPollPeriod(period time.Duration) HandlerOption {
	return func(h *Handler) error {
		if period < 0 {
			return trace.BadParameter("period should be non zero")
		}
		h.sessionStreamPollPeriod = period
		return nil
	}
}

// Config represents web handler configuration parameters
type Config struct {
	// Proxy is a reverse tunnel proxy that handles connections
	// to local cluster or remote clusters using unified interface
	Proxy reversetunnel.Tunnel
	// AuthServers is a list of auth servers this proxy talks to
	AuthServers utils.NetAddr
	// DomainName is a domain name served by web handler
	DomainName string
	// ProxyClient is a client that authenticated as proxy
	ProxyClient auth.ClientI
	// DisableUI allows to turn off serving web based UI
	DisableUI bool
	// ProxySSHAddr points to the SSH address of the proxy
	ProxySSHAddr utils.NetAddr
	// ProxyWebAddr points to the web (HTTPS) address of the proxy
	ProxyWebAddr utils.NetAddr

	// CipherSuites is the list of cipher suites Teleport suppports.
	CipherSuites []uint16

	// ProxySettings is a settings communicated to proxy
	ProxySettings client.ProxySettings

	// FIPS mode means Teleport started in a FedRAMP/FIPS 140-2 compliant
	// configuration.
	FIPS bool

	// AccessPoint holds a cache to the Auth Server.
	AccessPoint auth.AccessPoint

	// Emitter is event emitter
	Emitter events.StreamEmitter

	// HostUUID is the UUID of this process.
	HostUUID string

	// Context is used to signal process exit.
	Context context.Context
}

type RewritingHandler struct {
	http.Handler
	handler *Handler

	// appHandler is a http.Handler to forward requests to applications.
	appHandler *app.Handler

	// publicAddr is the public address the proxy is running at.
	publicAddr string
}

// Check if this request should be forwarded to an application handler to
// be handled by the UI and handle the request appropriately.
func (h *RewritingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// If the request is either to the fragment authentication endpoint or if the
	// request is already authenticated (has a session cookie), forward to
	// application handlers. If the request is unauthenticated and requesting a
	// FQDN that is not of the proxy, redirect to application launcher.
	if app.HasFragment(r) || app.HasSession(r) {
		h.appHandler.ServeHTTP(w, r)
		return
	}
	if redir, ok := app.HasName(r, h.publicAddr); ok {
		http.Redirect(w, r, redir, http.StatusFound)
		return
	}

	// Serve the Web UI.
	h.Handler.ServeHTTP(w, r)
}

func (h *RewritingHandler) Close() error {
	return h.handler.Close()
}

// NewHandler returns a new instance of web proxy handler
func NewHandler(cfg Config, opts ...HandlerOption) (*RewritingHandler, error) {
	const apiPrefix = "/" + teleport.WebAPIVersion
	lauth, err := newSessionCache(cfg.ProxyClient, []utils.NetAddr{cfg.AuthServers}, cfg.CipherSuites)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	h := &Handler{
		cfg:  cfg,
		auth: lauth,
		log:  newPackageLogger(),
	}

	for _, o := range opts {
		if err := o(h); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	_, sshPort, err := net.SplitHostPort(cfg.ProxySSHAddr.String())
	if err != nil {
		h.log.WithError(err).Warnf("Invalid SSH proxy address %q, will use default port %v.",
			cfg.ProxySSHAddr.String(), defaults.SSHProxyListenPort)
		sshPort = strconv.Itoa(defaults.SSHProxyListenPort)
	}
	h.sshPort = sshPort

	if h.clock == nil {
		h.clock = clockwork.NewRealClock()
	}

	// ping endpoint is used to check if the server is up. the /webapi/ping
	// endpoint returns the default authentication method and configuration that
	// the server supports. the /webapi/ping/:connector endpoint can be used to
	// query the authentication configuration for a specific connector.
	h.GET("/webapi/ping", httplib.MakeHandler(h.ping))
	h.GET("/webapi/ping/:connector", httplib.MakeHandler(h.pingWithConnector))
	// find is like ping, but is faster because it is optimized for servers
	// and does not fetch the data that servers don't need, e.g.
	// OIDC connectors and auth preferences
	h.GET("/webapi/find", httplib.MakeHandler(h.find))

	// Unauthenticated access to JWT public keys.
	h.GET("/.well-known/jwks.json", httplib.MakeHandler(h.jwks))

	// DELETE IN: 5.1.0
	//
	// Migrated this endpoint to /webapi/sessions/web below.
	h.POST("/webapi/sessions", httplib.WithCSRFProtection(h.createWebSession))

	// Web sessions
	h.POST("/webapi/sessions/web", httplib.WithCSRFProtection(h.createWebSession))
	h.POST("/webapi/sessions/app", h.WithAuth(h.createAppSession))
	h.DELETE("/webapi/sessions", h.WithAuth(h.deleteSession))
	h.POST("/webapi/sessions/renew", h.WithAuth(h.renewSession))
	h.POST("/webapi/sessions/renew/:requestId", h.WithAuth(h.renewSession))

	h.GET("/webapi/users/password/token/:token", httplib.MakeHandler(h.getResetPasswordTokenHandle))
	h.PUT("/webapi/users/password/token", httplib.WithCSRFProtection(h.changePasswordWithToken))
	h.PUT("/webapi/users/password", h.WithAuth(h.changePassword))
	h.POST("/webapi/users/password/token", h.WithAuth(h.createResetPasswordToken))

	// Issues SSH temp certificates based on 2FA access creds
	h.POST("/webapi/ssh/certs", httplib.MakeHandler(h.createSSHCert))

	// list available sites
	h.GET("/webapi/sites", h.WithAuth(h.getClusters))

	// Site specific API

	// get namespaces
	h.GET("/webapi/sites/:site/namespaces", h.WithClusterAuth(h.getSiteNamespaces))

	// get nodes
	h.GET("/webapi/sites/:site/namespaces/:namespace/nodes", h.WithClusterAuth(h.siteNodesGet))

	// Get applications.
	h.GET("/webapi/sites/:site/apps", h.WithClusterAuth(h.siteAppsGet))

	// active sessions handlers
	h.GET("/webapi/sites/:site/namespaces/:namespace/connect", h.WithClusterAuth(h.siteNodeConnect))       // connect to an active session (via websocket)
	h.GET("/webapi/sites/:site/namespaces/:namespace/sessions", h.WithClusterAuth(h.siteSessionsGet))      // get active list of sessions
	h.POST("/webapi/sites/:site/namespaces/:namespace/sessions", h.WithClusterAuth(h.siteSessionGenerate)) // create active session metadata
	h.GET("/webapi/sites/:site/namespaces/:namespace/sessions/:sid", h.WithClusterAuth(h.siteSessionGet))  // get active session metadata

	// recorded sessions handlers
	h.GET("/webapi/sites/:site/events", h.WithClusterAuth(h.clusterSearchSessionEvents))                               // get recorded list of sessions (from events)
	h.GET("/webapi/sites/:site/events/search", h.WithClusterAuth(h.clusterSearchEvents))                               // search site events
	h.GET("/webapi/sites/:site/namespaces/:namespace/sessions/:sid/events", h.WithClusterAuth(h.siteSessionEventsGet)) // get recorded session's timing information (from events)
	h.GET("/webapi/sites/:site/namespaces/:namespace/sessions/:sid/stream", h.siteSessionStreamGet)                    // get recorded session's bytes (from events)

	// scp file transfer
	h.GET("/webapi/sites/:site/namespaces/:namespace/nodes/:server/:login/scp", h.WithClusterAuth(h.transferFile))
	h.POST("/webapi/sites/:site/namespaces/:namespace/nodes/:server/:login/scp", h.WithClusterAuth(h.transferFile))

	// web context
	h.GET("/webapi/sites/:site/context", h.WithClusterAuth(h.getUserContext))

	// OIDC related callback handlers
	h.GET("/webapi/oidc/login/web", httplib.MakeHandler(h.oidcLoginWeb))
	h.POST("/webapi/oidc/login/console", httplib.MakeHandler(h.oidcLoginConsole))
	h.GET("/webapi/oidc/callback", httplib.MakeHandler(h.oidcCallback))

	// SAML 2.0 handlers
	h.POST("/webapi/saml/acs", httplib.MakeHandler(h.samlACS))
	h.GET("/webapi/saml/sso", httplib.MakeHandler(h.samlSSO))
	h.POST("/webapi/saml/login/console", httplib.MakeHandler(h.samlSSOConsole))

	// Github connector handlers
	h.GET("/webapi/github/login/web", httplib.MakeHandler(h.githubLoginWeb))
	h.POST("/webapi/github/login/console", httplib.MakeHandler(h.githubLoginConsole))
	h.GET("/webapi/github/callback", httplib.MakeHandler(h.githubCallback))

	// U2F related APIs
	h.GET("/webapi/u2f/signuptokens/:token", httplib.MakeHandler(h.u2fRegisterRequest))
	h.POST("/webapi/u2f/password/changerequest", h.WithAuth(h.u2fChangePasswordRequest))
	h.POST("/webapi/u2f/signrequest", httplib.MakeHandler(h.u2fSignRequest))
	h.POST("/webapi/u2f/sessions", httplib.MakeHandler(h.createSessionWithU2FSignResponse))
	h.POST("/webapi/u2f/certs", httplib.MakeHandler(h.createSSHCertWithU2FSignResponse))

	// trusted clusters
	h.POST("/webapi/trustedclusters/validate", httplib.MakeHandler(h.validateTrustedCluster))

	// User Status (used by client to check if user session is valid)
	h.GET("/webapi/user/status", h.WithAuth(h.getUserStatus))

	// Issue host credentials.
	h.POST("/webapi/host/credentials", httplib.MakeHandler(h.hostCredentials))

	// if Web UI is enabled, check the assets dir:
	var (
		indexPage *template.Template
		staticFS  http.FileSystem
	)
	if !cfg.DisableUI {
		staticFS, err = NewStaticFileSystem(isDebugMode())
		if err != nil {
			return nil, trace.Wrap(err)
		}
		index, err := staticFS.Open("/index.html")
		if err != nil {
			h.log.WithError(err).Error("Failed to open index file.")
			return nil, trace.Wrap(err)
		}
		defer index.Close()
		indexContent, err := ioutil.ReadAll(index)
		if err != nil {
			return nil, trace.ConvertSystemError(err)
		}
		indexPage, err = template.New("index").Parse(string(indexContent))
		if err != nil {
			return nil, trace.BadParameter("failed parsing index.html template: %v", err)
		}

		h.Handle("GET", "/web/config.js", httplib.MakeHandler(h.getWebConfig))
	}

	routingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// request is going to the API?
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			http.StripPrefix(apiPrefix, h).ServeHTTP(w, r)
			return
		}

		// request is going to the web UI
		if cfg.DisableUI {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}

		// redirect to "/web" when someone hits "/"
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/web", http.StatusFound)
			return
		}

		// serve Web UI:
		if strings.HasPrefix(r.URL.Path, "/web/app") {
			httplib.SetStaticFileHeaders(w.Header())
			http.StripPrefix("/web", http.FileServer(staticFS)).ServeHTTP(w, r)
		} else if strings.HasPrefix(r.URL.Path, "/web/") || r.URL.Path == "/web" {
			csrfToken, err := csrf.AddCSRFProtection(w, r)
			if err != nil {
				h.log.WithError(err).Warn("Failed to generate CSRF token.")
			}

			session := struct {
				Session string
				XCSRF   string
			}{
				XCSRF: csrfToken,
			}

			ctx, err := h.AuthenticateRequest(w, r, false)
			if err == nil {
				re, err := NewSessionResponse(ctx)
				if err == nil {
					out, err := json.Marshal(re)
					if err == nil {
						session.Session = base64.StdEncoding.EncodeToString(out)
					}
				} else {
					h.log.WithError(err).Debug("Could not authenticate.")
				}
			}
			httplib.SetIndexHTMLHeaders(w.Header())
			if err := indexPage.Execute(w, session); err != nil {
				h.log.WithError(err).Error("Failed to execute index page template.")
			}
		} else {
			http.NotFound(w, r)
		}
	})

	h.NotFound = routingHandler
	plugin := GetPlugin()
	if plugin != nil {
		plugin.AddHandlers(h)
	}

	// Create application specific handler. This handler handles sessions and
	// forwarding for application access.
	appHandler, err := app.NewHandler(cfg.Context, &app.HandlerConfig{
		Clock:         h.clock,
		AuthClient:    cfg.ProxyClient,
		AccessPoint:   cfg.AccessPoint,
		ProxyClient:   cfg.Proxy,
		CipherSuites:  cfg.CipherSuites,
		WebPublicAddr: cfg.ProxySettings.SSH.PublicAddr,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &RewritingHandler{
		Handler: httplib.RewritePaths(h,
			httplib.Rewrite("/webapi/sites/([^/]+)/sessions/(.*)", "/webapi/sites/$1/namespaces/default/sessions/$2"),
			httplib.Rewrite("/webapi/sites/([^/]+)/sessions", "/webapi/sites/$1/namespaces/default/sessions"),
			httplib.Rewrite("/webapi/sites/([^/]+)/nodes", "/webapi/sites/$1/namespaces/default/nodes"),
			httplib.Rewrite("/webapi/sites/([^/]+)/connect", "/webapi/sites/$1/namespaces/default/connect"),
		),
		handler:    h,
		appHandler: appHandler,
		publicAddr: cfg.ProxySettings.SSH.PublicAddr,
	}, nil
}

// GetProxyClient returns authenticated auth server client
func (h *Handler) GetProxyClient() auth.ClientI {
	return h.cfg.ProxyClient
}

// Close closes associated session cache operations
func (h *Handler) Close() error {
	return h.auth.Close()
}

func (h *Handler) getUserStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params, c *SessionContext) (interface{}, error) {
	return ok(), nil
}

// getUserContext returns user context
//
// GET /webapi/user/context
//
func (h *Handler) getUserContext(w http.ResponseWriter, r *http.Request, p httprouter.Params, c *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	clt, err := c.GetClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	cert, _, err := c.GetCertificates()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	roles, traits, err := services.ExtractFromCertificate(clt, cert)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	roleset, err := services.FetchRoles(roles, clt, traits)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	user, err := clt.GetUser(c.GetUser(), false)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	userContext, err := ui.NewUserContext(user, roleset)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	res, err := clt.GetAccessCapabilities(r.Context(), services.AccessCapabilitiesRequest{
		RequestableRoles: true,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	userContext.RequestableRoles = res.RequestableRoles
	userContext.Cluster, err = ui.GetClusterDetails(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return userContext, nil
}

func localSettings(authClient auth.ClientI, cap services.AuthPreference) (client.AuthenticationSettings, error) {
	as := client.AuthenticationSettings{
		Type:         teleport.Local,
		SecondFactor: cap.GetSecondFactor(),
	}

	// if the type is u2f, pull some additional data back
	if cap.GetSecondFactor() == teleport.U2F {
		u2fs, err := cap.GetU2F()
		if err != nil {
			return client.AuthenticationSettings{}, trace.Wrap(err)
		}

		as.U2F = &client.U2FSettings{AppID: u2fs.AppID}
	}

	return as, nil
}

func oidcSettings(connector services.OIDCConnector, cap services.AuthPreference) client.AuthenticationSettings {
	return client.AuthenticationSettings{
		Type: teleport.OIDC,
		OIDC: &client.OIDCSettings{
			Name:    connector.GetName(),
			Display: connector.GetDisplay(),
		},
		// if you falling back to local accounts
		SecondFactor: cap.GetSecondFactor(),
	}
}

func samlSettings(connector services.SAMLConnector, cap services.AuthPreference) client.AuthenticationSettings {
	return client.AuthenticationSettings{
		Type: teleport.SAML,
		SAML: &client.SAMLSettings{
			Name:    connector.GetName(),
			Display: connector.GetDisplay(),
		},
		// if you are falling back to local accounts
		SecondFactor: cap.GetSecondFactor(),
	}
}

func githubSettings(connector services.GithubConnector, cap services.AuthPreference) client.AuthenticationSettings {
	return client.AuthenticationSettings{
		Type: teleport.Github,
		Github: &client.GithubSettings{
			Name:    connector.GetName(),
			Display: connector.GetDisplay(),
		},
		SecondFactor: cap.GetSecondFactor(),
	}
}

func defaultAuthenticationSettings(authClient auth.ClientI) (client.AuthenticationSettings, error) {
	cap, err := authClient.GetAuthPreference()
	if err != nil {
		return client.AuthenticationSettings{}, trace.Wrap(err)
	}

	var as client.AuthenticationSettings

	switch cap.GetType() {
	case teleport.Local:
		as, err = localSettings(authClient, cap)
		if err != nil {
			return client.AuthenticationSettings{}, trace.Wrap(err)
		}
	case teleport.OIDC:
		if cap.GetConnectorName() != "" {
			oidcConnector, err := authClient.GetOIDCConnector(cap.GetConnectorName(), false)
			if err != nil {
				return client.AuthenticationSettings{}, trace.Wrap(err)
			}

			as = oidcSettings(oidcConnector, cap)
		} else {
			oidcConnectors, err := authClient.GetOIDCConnectors(false)
			if err != nil {
				return client.AuthenticationSettings{}, trace.Wrap(err)
			}
			if len(oidcConnectors) == 0 {
				return client.AuthenticationSettings{}, trace.BadParameter("no oidc connectors found")
			}

			as = oidcSettings(oidcConnectors[0], cap)
		}
	case teleport.SAML:
		if cap.GetConnectorName() != "" {
			samlConnector, err := authClient.GetSAMLConnector(cap.GetConnectorName(), false)
			if err != nil {
				return client.AuthenticationSettings{}, trace.Wrap(err)
			}

			as = samlSettings(samlConnector, cap)
		} else {
			samlConnectors, err := authClient.GetSAMLConnectors(false)
			if err != nil {
				return client.AuthenticationSettings{}, trace.Wrap(err)
			}
			if len(samlConnectors) == 0 {
				return client.AuthenticationSettings{}, trace.BadParameter("no saml connectors found")
			}

			as = samlSettings(samlConnectors[0], cap)
		}
	case teleport.Github:
		if cap.GetConnectorName() != "" {
			githubConnector, err := authClient.GetGithubConnector(cap.GetConnectorName(), false)
			if err != nil {
				return client.AuthenticationSettings{}, trace.Wrap(err)
			}
			as = githubSettings(githubConnector, cap)
		} else {
			githubConnectors, err := authClient.GetGithubConnectors(false)
			if err != nil {
				return client.AuthenticationSettings{}, trace.Wrap(err)
			}
			if len(githubConnectors) == 0 {
				return client.AuthenticationSettings{}, trace.BadParameter("no github connectors found")
			}
			as = githubSettings(githubConnectors[0], cap)
		}
	default:
		return client.AuthenticationSettings{}, trace.BadParameter("unknown type %v", cap.GetType())
	}

	return as, nil
}

func (h *Handler) ping(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var err error

	defaultSettings, err := defaultAuthenticationSettings(h.cfg.ProxyClient)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return client.PingResponse{
		Auth:             defaultSettings,
		Proxy:            h.cfg.ProxySettings,
		ServerVersion:    teleport.Version,
		MinClientVersion: teleport.MinClientVersion,
	}, nil
}

func (h *Handler) find(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	return client.PingResponse{
		Proxy:            h.cfg.ProxySettings,
		ServerVersion:    teleport.Version,
		MinClientVersion: teleport.MinClientVersion,
	}, nil
}

func (h *Handler) pingWithConnector(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	authClient := h.cfg.ProxyClient
	connectorName := p.ByName("connector")

	cap, err := authClient.GetAuthPreference()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	response := &client.PingResponse{
		Proxy:         h.cfg.ProxySettings,
		ServerVersion: teleport.Version,
	}

	if connectorName == teleport.Local {
		as, err := localSettings(h.cfg.ProxyClient, cap)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		response.Auth = as
		return response, nil
	}

	// first look for a oidc connector with that name
	oidcConnector, err := authClient.GetOIDCConnector(connectorName, false)
	if err == nil {
		response.Auth = oidcSettings(oidcConnector, cap)
		return response, nil
	}

	// if no oidc connector was found, look for a saml connector
	samlConnector, err := authClient.GetSAMLConnector(connectorName, false)
	if err == nil {
		response.Auth = samlSettings(samlConnector, cap)
		return response, nil
	}

	// look for github connector
	githubConnector, err := authClient.GetGithubConnector(connectorName, false)
	if err == nil {
		response.Auth = githubSettings(githubConnector, cap)
		return response, nil
	}

	return nil, trace.BadParameter("invalid connector name %v", connectorName)
}

// getWebConfig returns configuration for the web application.
func (h *Handler) getWebConfig(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	httplib.SetWebConfigHeaders(w.Header())

	authProviders := []ui.WebConfigAuthProvider{}
	secondFactor := teleport.OFF

	// get all OIDC connectors
	oidcConnectors, err := h.cfg.ProxyClient.GetOIDCConnectors(false)
	if err != nil {
		h.log.WithError(err).Error("Cannot retrieve OIDC connectors.")
	}
	for _, item := range oidcConnectors {
		authProviders = append(authProviders, ui.WebConfigAuthProvider{
			Type:        ui.WebConfigAuthProviderOIDCType,
			WebAPIURL:   ui.WebConfigAuthProviderOIDCURL,
			Name:        item.GetName(),
			DisplayName: item.GetDisplay(),
		})
	}

	// get all SAML connectors
	samlConnectors, err := h.cfg.ProxyClient.GetSAMLConnectors(false)
	if err != nil {
		h.log.WithError(err).Error("Cannot retrieve SAML connectors.")
	}
	for _, item := range samlConnectors {
		authProviders = append(authProviders, ui.WebConfigAuthProvider{
			Type:        ui.WebConfigAuthProviderSAMLType,
			WebAPIURL:   ui.WebConfigAuthProviderSAMLURL,
			Name:        item.GetName(),
			DisplayName: item.GetDisplay(),
		})
	}

	// get all Github connectors
	githubConnectors, err := h.cfg.ProxyClient.GetGithubConnectors(false)
	if err != nil {
		h.log.WithError(err).Error("Cannot retrieve Github connectors.")
	}
	for _, item := range githubConnectors {
		authProviders = append(authProviders, ui.WebConfigAuthProvider{
			Type:        ui.WebConfigAuthProviderGitHubType,
			WebAPIURL:   ui.WebConfigAuthProviderGitHubURL,
			Name:        item.GetName(),
			DisplayName: item.GetDisplay(),
		})
	}

	// get second factor type
	cap, err := h.cfg.ProxyClient.GetAuthPreference()
	if err != nil {
		h.log.WithError(err).Error("Cannot retrieve AuthPreferences.")
	} else {
		secondFactor = cap.GetSecondFactor()
	}

	// disable joining sessions if proxy session recording is enabled
	var canJoinSessions = true
	clsCfg, err := h.cfg.ProxyClient.GetClusterConfig()
	if err != nil {
		h.log.WithError(err).Error("Cannot retrieve ClusterConfig.")
	} else {
		canJoinSessions = services.IsRecordAtProxy(clsCfg.GetSessionRecording()) == false
	}

	authSettings := ui.WebConfigAuthSettings{
		Providers:        authProviders,
		SecondFactor:     secondFactor,
		LocalAuthEnabled: clsCfg.GetLocalAuth(),
		AuthType:         cap.GetType(),
	}

	webCfg := ui.WebConfig{
		Auth:            authSettings,
		CanJoinSessions: canJoinSessions,
	}

	resource, err := h.cfg.ProxyClient.GetClusterName()
	if err != nil {
		h.log.WithError(err).Warn("Failed to query cluster name.")
	} else {
		webCfg.ProxyClusterName = resource.GetClusterName()
	}

	out, err := json.Marshal(webCfg)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	fmt.Fprintf(w, "var GRV_CONFIG = %v;", string(out))
	return nil, nil
}

type JWKSResponse struct {
	// Keys is a list of public keys in JWK format.
	Keys []jwt.JWK `json:"keys"`
}

// jwks returns all public keys used to sign JWT tokens for this cluster.
func (h *Handler) jwks(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	clusterName, err := h.cfg.ProxyClient.GetDomainName()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Fetch the JWT public keys only.
	ca, err := h.cfg.ProxyClient.GetCertAuthority(services.CertAuthID{
		Type:       services.JWTSigner,
		DomainName: clusterName,
	}, false)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pairs := ca.GetJWTKeyPairs()

	// Create response and allocate space for the keys.
	var resp JWKSResponse
	resp.Keys = make([]jwt.JWK, 0, len(pairs))

	// Loop over and all add public keys in JWK format.
	for _, pair := range pairs {
		jwk, err := jwt.MarshalJWK(pair.PublicKey)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		resp.Keys = append(resp.Keys, jwk)
	}
	return &resp, nil
}

func (h *Handler) oidcLoginWeb(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	logger := h.log.WithField("auth", "oidc")
	logger.Debug("Web login start.")

	query := r.URL.Query()
	clientRedirectURL := query.Get("redirect_url")
	if clientRedirectURL == "" {
		return nil, trace.BadParameter("missing redirect_url query parameter")
	}
	connectorID := query.Get("connector_id")
	if connectorID == "" {
		return nil, trace.BadParameter("missing connector_id query parameter")
	}

	csrfToken, err := csrf.ExtractTokenFromCookie(r)
	if err != nil {
		logger.WithError(err).Warn("Unable to extract CSRF token from cookie.")
		return nil, trace.AccessDenied("access denied")
	}

	response, err := h.cfg.ProxyClient.CreateOIDCAuthRequest(
		services.OIDCAuthRequest{
			CSRFToken:         csrfToken,
			ConnectorID:       connectorID,
			CreateWebSession:  true,
			ClientRedirectURL: clientRedirectURL,
			CheckUser:         true,
		})
	if err != nil {
		// redirect to an error page
		pathToError := url.URL{
			Path:     "/web/msg/error/login_failed",
			RawQuery: url.Values{"details": []string{err.Error()}}.Encode(),
		}
		http.Redirect(w, r, pathToError.String(), http.StatusFound)
		return nil, nil
	}
	http.Redirect(w, r, response.RedirectURL, http.StatusFound)
	return nil, nil
}

func (h *Handler) githubLoginWeb(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	logger := h.log.WithField("auth", "github")
	logger.Debug("Web login start.")

	query := r.URL.Query()
	clientRedirectURL := query.Get("redirect_url")
	if clientRedirectURL == "" {
		return nil, trace.BadParameter("missing redirect_url query parameter")
	}
	connectorID := query.Get("connector_id")
	if connectorID == "" {
		return nil, trace.BadParameter("missing connector_id query parameter")
	}

	csrfToken, err := csrf.ExtractTokenFromCookie(r)
	if err != nil {
		logger.WithError(err).Warn("Unable to extract CSRF token from cookie.")
		return nil, trace.AccessDenied("access denied")
	}
	response, err := h.cfg.ProxyClient.CreateGithubAuthRequest(
		services.GithubAuthRequest{
			CSRFToken:         csrfToken,
			ConnectorID:       connectorID,
			CreateWebSession:  true,
			ClientRedirectURL: clientRedirectURL,
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	http.Redirect(w, r, response.RedirectURL, http.StatusFound)
	return nil, nil
}

func (h *Handler) githubLoginConsole(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	h.log.WithField("auth", "github").Debug("Console login start.")
	req := new(client.SSOLoginConsoleReq)
	if err := httplib.ReadJSON(r, req); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := req.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	response, err := h.cfg.ProxyClient.CreateGithubAuthRequest(
		services.GithubAuthRequest{
			ConnectorID:       req.ConnectorID,
			PublicKey:         req.PublicKey,
			CertTTL:           req.CertTTL,
			ClientRedirectURL: req.RedirectURL,
			Compatibility:     req.Compatibility,
			RouteToCluster:    req.RouteToCluster,
			KubernetesCluster: req.KubernetesCluster,
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &client.SSOLoginConsoleResponse{
		RedirectURL: response.RedirectURL,
	}, nil
}

func (h *Handler) githubCallback(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	logger := h.log.WithField("auth", "github")
	logger.Debugf("Callback start: %v.", r.URL.Query())

	response, err := h.cfg.ProxyClient.ValidateGithubAuthCallback(r.URL.Query())
	if err != nil {
		logger.Warnf("Error while processing callback: %v.", err)
		// redirect to an error page
		pathToError := url.URL{
			Path: "/web/msg/error/login_failed",
			RawQuery: url.Values{"details": []string{
				"Unable to process callback from Github."}}.Encode(),
		}
		http.Redirect(w, r, pathToError.String(), http.StatusFound)
		return nil, nil
	}
	// if we created web session, set session cookie and redirect to original url
	if response.Req.CreateWebSession {
		err = csrf.VerifyToken(response.Req.CSRFToken, r)
		if err != nil {
			logger.Warnf("Unable to verify CSRF token: %v.", err)
			return nil, trace.AccessDenied("access denied")
		}
		logger.Infof("Callback is redirecting to web browser.")
		err = SetSession(w, response.Username, response.Session.GetName())
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return nil, httplib.SafeRedirect(w, r, response.Req.ClientRedirectURL)
	}
	logger.Infof("Callback is redirecting to console login.")
	if len(response.Req.PublicKey) == 0 {
		return nil, trace.BadParameter("not a web or console Github login request")
	}

	redirectURL, err := ConstructSSHResponse(AuthParams{
		ClientRedirectURL: response.Req.ClientRedirectURL,
		Username:          response.Username,
		Identity:          response.Identity,
		Session:           response.Session,
		Cert:              response.Cert,
		TLSCert:           response.TLSCert,
		HostSigners:       response.HostSigners,
		FIPS:              h.cfg.FIPS,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
	return nil, nil
}

func (h *Handler) oidcLoginConsole(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	h.log.WithField("auth", "oidc").Debug("Console login start.")
	req := new(client.SSOLoginConsoleReq)
	if err := httplib.ReadJSON(r, req); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := req.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	response, err := h.cfg.ProxyClient.CreateOIDCAuthRequest(
		services.OIDCAuthRequest{
			ConnectorID:       req.ConnectorID,
			ClientRedirectURL: req.RedirectURL,
			PublicKey:         req.PublicKey,
			CertTTL:           req.CertTTL,
			CheckUser:         true,
			Compatibility:     req.Compatibility,
			RouteToCluster:    req.RouteToCluster,
			KubernetesCluster: req.KubernetesCluster,
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &client.SSOLoginConsoleResponse{
		RedirectURL: response.RedirectURL,
	}, nil
}

func (h *Handler) oidcCallback(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	logger := newPackageLogger("oidc")
	logger.Debug("Callback start.")

	response, err := h.cfg.ProxyClient.ValidateOIDCAuthCallback(r.URL.Query())
	if err != nil {
		logger.WithError(err).Warn("Error while processing callback.")

		message := "Unable to process callback from OIDC provider."
		// redirect to an error page
		pathToError := url.URL{
			Path:     "/web/msg/error/login_failed",
			RawQuery: url.Values{"details": []string{message}}.Encode(),
		}
		http.Redirect(w, r, pathToError.String(), http.StatusFound)
		return nil, nil
	}
	// if we created web session, set session cookie and redirect to original url
	if response.Req.CreateWebSession {
		err = csrf.VerifyToken(response.Req.CSRFToken, r)
		if err != nil {
			logger.WithError(err).Warn("Unable to verify CSRF token.")
			return nil, trace.AccessDenied("access denied")
		}

		logger.Info("Callback redirecting to web browser.")
		if err := SetSession(w, response.Username, response.Session.GetName()); err != nil {
			return nil, trace.Wrap(err)
		}
		return nil, httplib.SafeRedirect(w, r, response.Req.ClientRedirectURL)
	}
	logger.Info("Callback redirecting to console login.")
	if len(response.Req.PublicKey) == 0 {
		return nil, trace.BadParameter("not a web or console oidc login request")
	}
	redirectURL, err := ConstructSSHResponse(AuthParams{
		ClientRedirectURL: response.Req.ClientRedirectURL,
		Username:          response.Username,
		Identity:          response.Identity,
		Session:           response.Session,
		Cert:              response.Cert,
		TLSCert:           response.TLSCert,
		HostSigners:       response.HostSigners,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
	return nil, nil
}

// AuthParams are used to construct redirect URL containing auth
// information back to tsh login
type AuthParams struct {
	// Username is authenticated teleport username
	Username string
	// Identity contains validated OIDC identity
	Identity services.ExternalIdentity
	// Web session will be generated by auth server if requested in OIDCAuthRequest
	Session services.WebSession
	// Cert will be generated by certificate authority
	Cert []byte
	// TLSCert is PEM encoded TLS certificate
	TLSCert []byte
	// HostSigners is a list of signing host public keys
	// trusted by proxy, used in console login
	HostSigners []services.CertAuthority
	// ClientRedirectURL is a URL to redirect client to
	ClientRedirectURL string
	// FIPS mode means Teleport started in a FedRAMP/FIPS 140-2 compliant
	// configuration.
	FIPS bool
}

// ConstructSSHResponse creates a special SSH response for SSH login method
// that encodes everything using the client's secret key
func ConstructSSHResponse(response AuthParams) (*url.URL, error) {
	u, err := url.Parse(response.ClientRedirectURL)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	consoleResponse := auth.SSHLoginResponse{
		Username:    response.Username,
		Cert:        response.Cert,
		TLSCert:     response.TLSCert,
		HostSigners: auth.AuthoritiesToTrustedCerts(response.HostSigners),
	}
	out, err := json.Marshal(consoleResponse)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Extract secret out of the request. Look for both "secret" which is the
	// old format and "secret_key" which is the new fomat. If this is not done,
	// then users would have to update their callback URL in their identity
	// provider.
	values := u.Query()
	secretV1 := values.Get("secret")
	secretV2 := values.Get("secret_key")
	values.Set("secret", "")
	values.Set("secret_key", "")

	var ciphertext []byte

	switch {
	// AES-GCM based symmetric cipher.
	case secretV2 != "":
		key, err := secret.ParseKey([]byte(secretV2))
		if err != nil {
			return nil, trace.Wrap(err)
		}
		ciphertext, err = key.Seal(out)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	// NaCl based symmetric cipher (legacy).
	case secretV1 != "":
		// If FIPS mode was requested, make sure older clients that use NaCl get rejected.
		if response.FIPS {
			return nil, trace.BadParameter("non-FIPS compliant encryption: NaCl, check " +
				"that tsh release was downloaded from https://dashboard.gravitational.com")
		}

		secretKeyBytes, err := lemma_secret.EncodedStringToKey(secretV1)
		if err != nil {
			return nil, trace.BadParameter("bad secret")
		}
		encryptor, err := lemma_secret.New(&lemma_secret.Config{KeyBytes: secretKeyBytes})
		if err != nil {
			return nil, trace.Wrap(err)
		}
		sealedBytes, err := encryptor.Seal(out)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		ciphertext, err = json.Marshal(sealedBytes)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	default:
		return nil, trace.BadParameter("missing secret")
	}

	// Place ciphertext into the response body.
	values.Set("response", string(ciphertext))

	u.RawQuery = values.Encode()
	return u, nil
}

// CreateSessionReq is a request to create session from username, password and
// second factor token.
type CreateSessionReq struct {
	// User is the Teleport username.
	User string `json:"user"`
	// Pass is the password.
	Pass string `json:"pass"`
	// SecondFactorToken is the OTP.
	SecondFactorToken string `json:"second_factor_token"`
}

// CreateSessionResponse returns OAuth compabible data about
// access token: https://tools.ietf.org/html/rfc6749
type CreateSessionResponse struct {
	// Type is token type (bearer)
	Type string `json:"type"`
	// Token value
	Token string `json:"token"`
	// ExpiresIn sets seconds before this token is not valid
	ExpiresIn int `json:"expires_in"`
}

func NewSessionResponse(ctx *SessionContext) (*CreateSessionResponse, error) {
	clt, err := ctx.GetClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	webSession := ctx.GetWebSession()
	user, err := clt.GetUser(webSession.GetUser(), false)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var roles services.RoleSet
	for _, roleName := range user.GetRoles() {
		role, err := clt.GetRole(roleName)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		roles = append(roles, role)
	}
	_, err = roles.CheckLoginDuration(0)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &CreateSessionResponse{
		Type:      roundtrip.AuthBearer,
		Token:     webSession.GetBearerToken(),
		ExpiresIn: int(time.Until(webSession.GetBearerTokenExpiryTime()) / time.Second),
	}, nil
}

// createWebSession creates a new web session based on user, pass and 2nd factor token
//
// POST /v1/webapi/sessions
//
// {"user": "alex", "pass": "abc123", "second_factor_token": "token", "second_factor_type": "totp"}
//
// Response
//
// {"type": "bearer", "token": "bearer token", "user": {"name": "alex", "allowed_logins": ["admin", "bob"]}, "expires_in": 20}
//
func (h *Handler) createWebSession(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var req *CreateSessionReq
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	// get cluster preferences to see if we should login
	// with password or password+otp
	authClient := h.cfg.ProxyClient
	cap, err := authClient.GetAuthPreference()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var webSession services.WebSession

	switch cap.GetSecondFactor() {
	case teleport.OFF:
		webSession, err = h.auth.AuthWithoutOTP(req.User, req.Pass)
	case teleport.OTP, teleport.HOTP, teleport.TOTP:
		webSession, err = h.auth.AuthWithOTP(req.User, req.Pass, req.SecondFactorToken)
	default:
		return nil, trace.AccessDenied("unknown second factor type: %q", cap.GetSecondFactor())
	}
	if err != nil {
		h.log.WithError(err).Warnf("Access attempt denied for user %q.", req.User)
		return nil, trace.AccessDenied("bad auth credentials")
	}

	if err := SetSession(w, req.User, webSession.GetName()); err != nil {
		return nil, trace.Wrap(err)
	}

	ctx, err := h.auth.ValidateSession(req.User, webSession.GetName())
	if err != nil {
		h.log.WithError(err).Warnf("Access attempt denied for user %q.", req.User)
		return nil, trace.AccessDenied("need auth")
	}

	return NewSessionResponse(ctx)
}

// deleteSession is called to sign out user
//
// DELETE /v1/webapi/sessions/:sid
//
// Response:
//
// {"message": "ok"}
//
func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request, _ httprouter.Params, ctx *SessionContext) (interface{}, error) {
	err := h.logout(w, ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return ok(), nil
}

func (h *Handler) logout(w http.ResponseWriter, ctx *SessionContext) error {
	if err := ctx.Invalidate(); err != nil {
		return trace.Wrap(err)
	}
	ClearSession(w)

	return nil
}

// renewSession is called in two ways:
// 	- Without requestId: Creates new session that is about to expire.
// 	- With requestId: Creates new session that includes additional roles assigned with approving access request.
//
// 	It issues the new session and generates new session cookie.
// 	It's important to understand that the old session becomes effectively invalid.
func (h *Handler) renewSession(w http.ResponseWriter, r *http.Request, params httprouter.Params, ctx *SessionContext) (interface{}, error) {
	requestID := params.ByName("requestId")

	newSess, err := ctx.ExtendWebSession(requestID)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// transfer ownership over connections that were opened in the
	// sessionContext
	newContext, err := ctx.parent.ValidateSession(newSess.GetUser(), newSess.GetName())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	newContext.AddClosers(ctx.TransferClosers()...)
	if err := SetSession(w, newSess.GetUser(), newSess.GetName()); err != nil {
		return nil, trace.Wrap(err)
	}
	return NewSessionResponse(newContext)
}

func (h *Handler) changePasswordWithToken(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var req auth.ChangePasswordWithTokenRequest
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	sess, err := h.auth.proxyClient.ChangePasswordWithToken(r.Context(), req)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, err := h.auth.ValidateSession(sess.GetUser(), sess.GetName())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if err := SetSession(w, sess.GetUser(), sess.GetName()); err != nil {
		return nil, trace.Wrap(err)
	}

	return NewSessionResponse(ctx)
}

// createResetPasswordToken allows a UI user to reset a user's password.
// This handler is also required for after creating new users.
func (h *Handler) createResetPasswordToken(w http.ResponseWriter, r *http.Request, _ httprouter.Params, ctx *SessionContext) (interface{}, error) {
	clt, err := ctx.GetClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var req auth.CreateResetPasswordTokenRequest
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	token, err := clt.CreateResetPasswordToken(r.Context(),
		auth.CreateResetPasswordTokenRequest{
			Name: req.Name,
			Type: req.Type,
		})

	if err != nil {
		return nil, trace.Wrap(err)
	}

	return ui.ResetPasswordToken{
		TokenID: token.GetName(),
		Expiry:  token.Expiry(),
		User:    token.GetUser(),
	}, nil
}

func (h *Handler) getResetPasswordTokenHandle(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	result, err := h.getResetPasswordToken(context.TODO(), p.ByName("token"))
	if err != nil {
		h.log.WithError(err).Warn("Failed to fetch a reset password token.")
		// We hide the error from the remote user to avoid giving any hints.
		return nil, trace.AccessDenied("bad or expired token")
	}

	return result, nil
}

func (h *Handler) getResetPasswordToken(ctx context.Context, tokenID string) (interface{}, error) {
	token, err := h.auth.proxyClient.GetResetPasswordToken(ctx, tokenID)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// RotateResetPasswordTokenSecrets rotates secrets for a given tokenID.
	// It gets called every time a user fetches 2nd-factor secrets during registration attempt.
	// This ensures that an attacker that gains the ResetPasswordToken link can not view it,
	// extract the OTP key from the QR code, then allow the user to signup with
	// the same OTP token.
	secrets, err := h.auth.proxyClient.RotateResetPasswordTokenSecrets(ctx, tokenID)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return ui.ResetPasswordToken{
		TokenID: token.GetName(),
		User:    token.GetUser(),
		QRCode:  secrets.GetQRCode(),
	}, nil
}

// u2fRegisterRequest is called to get a U2F challenge for registering a U2F key
//
// GET /webapi/u2f/signuptokens/:token
//
// Response:
//
// {"version":"U2F_V2","challenge":"randombase64string","appId":"https://mycorp.com:3080"}
//
func (h *Handler) u2fRegisterRequest(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	token := p.ByName("token")
	u2fRegisterRequest, err := h.auth.GetUserInviteU2FRegisterRequest(token)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return u2fRegisterRequest, nil
}

// u2fSignRequest is called to get a U2F challenge for authenticating
//
// POST /webapi/u2f/signrequest
//
// {"user": "alex", "pass": "abc123"}
//
// Successful response:
//
// {"version":"U2F_V2","challenge":"randombase64string","keyHandle":"longbase64string","appId":"https://mycorp.com:3080"}
//
func (h *Handler) u2fSignRequest(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var req *client.U2fSignRequestReq
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}
	u2fSignReq, err := h.auth.GetU2FSignRequest(req.User, req.Pass)
	if err != nil {
		return nil, trace.AccessDenied("bad auth credentials")
	}

	return u2fSignReq, nil
}

// A request from the client to send the signature from the U2F key
type u2fSignResponseReq struct {
	User            string           `json:"user"`
	U2FSignResponse u2f.SignResponse `json:"u2f_sign_response"`
}

// createSessionWithU2FSignResponse is called to sign in with a U2F signature
//
// POST /webapi/u2f/session
//
// {"user": "alex", "u2f_sign_response": { "signatureData": "signatureinbase64", "clientData": "verylongbase64string", "challenge": "randombase64string" }}
//
// Successful response:
//
// {"type": "bearer", "token": "bearer token", "user": {"name": "alex", "allowed_logins": ["admin", "bob"]}, "expires_in": 20}
//
func (h *Handler) createSessionWithU2FSignResponse(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var req *u2fSignResponseReq
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	sess, err := h.auth.AuthWithU2FSignResponse(req.User, &req.U2FSignResponse)
	if err != nil {
		return nil, trace.AccessDenied("bad auth credentials")
	}
	if err := SetSession(w, req.User, sess.GetName()); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, err := h.auth.ValidateSession(req.User, sess.GetName())
	if err != nil {
		return nil, trace.AccessDenied("need auth")
	}
	return NewSessionResponse(ctx)
}

// getClusters returns a list of cluster and its data.
//
// GET /v1/webapi/sites
//
// Successful response:
//
// {"sites": {"name": "localhost", "last_connected": "RFC3339 time", "status": "active"}}
//
func (h *Handler) getClusters(w http.ResponseWriter, r *http.Request, p httprouter.Params, c *SessionContext) (interface{}, error) {
	// Get a client to the Auth Server with the logged in users identity. The
	// identity of the logged in user is used to fetch the list of nodes.
	clt, err := c.GetClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	remoteClusters, err := clt.GetRemoteClusters(services.SkipValidation())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	clusterName, err := clt.GetClusterName()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	rc, err := services.NewRemoteCluster(clusterName.GetClusterName())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	rc.SetLastHeartbeat(time.Now().UTC())
	rc.SetConnectionStatus(teleport.RemoteClusterStatusOnline)
	clusters := make([]services.RemoteCluster, 0, len(remoteClusters)+1)
	clusters = append(clusters, rc)
	clusters = append(clusters, remoteClusters...)
	out, err := ui.NewClustersFromRemote(clusters)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return out, nil
}

type getSiteNamespacesResponse struct {
	Namespaces []services.Namespace `json:"namespaces"`
}

/* getSiteNamespaces returns a list of namespaces for a given site

GET /v1/webapi/namespaces/:namespace/sites/:site/nodes

Successful response:

{"namespaces": [{..namespace resource...}]}
*/
func (h *Handler) getSiteNamespaces(w http.ResponseWriter, r *http.Request, _ httprouter.Params, c *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	clt, err := site.GetClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	namespaces, err := clt.GetNamespaces()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return getSiteNamespacesResponse{
		Namespaces: namespaces,
	}, nil
}

func (h *Handler) siteNodesGet(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	namespace := p.ByName("namespace")
	if !services.IsValidNamespace(namespace) {
		return nil, trace.BadParameter("invalid namespace %q", namespace)
	}

	// Get a client to the Auth Server with the logged in users identity. The
	// identity of the logged in user is used to fetch the list of nodes.
	clt, err := ctx.GetUserClient(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	servers, err := clt.GetNodes(namespace, services.SkipValidation())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	uiServers := ui.MakeServers(site.GetName(), servers)
	return makeResponse(uiServers)
}

// siteNodeConnect connect to the site node
//
// GET /v1/webapi/sites/:site/namespaces/:namespace/connect?access_token=bearer_token&params=<urlencoded json-structure>
//
// Due to the nature of websocket we can't POST parameters as is, so we have
// to add query parameters. The params query parameter is a url encodeed JSON strucrture:
//
// {"server_id": "uuid", "login": "admin", "term": {"h": 120, "w": 100}, "sid": "123"}
//
// Session id can be empty
//
// Successful response is a websocket stream that allows read write to the server
//
func (h *Handler) siteNodeConnect(
	w http.ResponseWriter,
	r *http.Request,
	p httprouter.Params,
	ctx *SessionContext,
	site reversetunnel.RemoteSite) (interface{}, error) {

	namespace := p.ByName("namespace")
	if !services.IsValidNamespace(namespace) {
		return nil, trace.BadParameter("invalid namespace %q", namespace)
	}

	q := r.URL.Query()
	params := q.Get("params")
	if params == "" {
		return nil, trace.BadParameter("missing params")
	}
	var req *TerminalRequest
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, trace.Wrap(err)
	}

	h.log.Debugf("New terminal request for ns=%s, server=%s, login=%s, sid=%s.",
		req.Namespace, req.Server, req.Login, req.SessionID)

	authAccessPoint, err := site.CachingAccessPoint()
	if err != nil {
		h.log.WithError(err).Debug("Unable to get auth access point.")
		return nil, trace.Wrap(err)
	}

	clusterConfig, err := authAccessPoint.GetClusterConfig()
	if err != nil {
		h.log.WithError(err).Debug("Unable to fetch cluster config.")
		return nil, trace.Wrap(err)
	}

	req.KeepAliveInterval = clusterConfig.GetKeepAliveInterval()
	req.Namespace = namespace
	req.ProxyHostPort = h.ProxyHostPort()
	req.Cluster = site.GetName()

	clt, err := ctx.GetUserClient(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	term, err := NewTerminal(*req, clt, ctx)
	if err != nil {
		h.log.WithError(err).Error("Unable to create terminal.")
		return nil, trace.Wrap(err)
	}

	// start the websocket session with a web-based terminal:
	h.log.Infof("Getting terminal to '%#v'.", req)
	term.Serve(w, r)

	return nil, nil
}

type siteSessionGenerateReq struct {
	Session session.Session `json:"session"`
}

type siteSessionGenerateResponse struct {
	Session session.Session `json:"session"`
}

// siteSessionCreate generates a new site session that can be used by UI
// The ServerID from request can be in the form of hostname, uuid, or ip address.
func (h *Handler) siteSessionGenerate(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	clt, err := ctx.GetUserClient(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	namespace := p.ByName("namespace")
	if !services.IsValidNamespace(namespace) {
		return nil, trace.BadParameter("invalid namespace %q", namespace)
	}

	var req *siteSessionGenerateReq
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	if req.Session.ServerID != "" {
		servers, err := clt.GetNodes(namespace, services.SkipValidation())
		if err != nil {
			return nil, trace.Wrap(err)
		}

		hostname, _, err := resolveServerHostPort(req.Session.ServerID, servers)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		req.Session.ServerHostname = hostname
	}

	req.Session.ID = session.NewID()
	req.Session.Created = time.Now().UTC()
	req.Session.LastActive = time.Now().UTC()
	req.Session.Namespace = namespace

	return siteSessionGenerateResponse{Session: req.Session}, nil
}

type siteSessionsGetResponse struct {
	Sessions []session.Session `json:"sessions"`
}

// siteSessionGet gets the list of site sessions filtered by creation time
// and either they're active or not
//
// GET /v1/webapi/sites/:site/namespaces/:namespace/sessions
//
// Response body:
//
// {"sessions": [{"id": "sid", "terminal_params": {"w": 100, "h": 100}, "parties": [], "login": "bob"}, ...] }
func (h *Handler) siteSessionsGet(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	clt, err := ctx.GetUserClient(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	namespace := p.ByName("namespace")
	if !services.IsValidNamespace(namespace) {
		return nil, trace.BadParameter("invalid namespace %q", namespace)
	}

	sessions, err := clt.GetSessions(namespace)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// DELETE IN: 5.0.0
	// Teleport Nodes < v4.3 does not set ClusterName, ServerHostname with new sessions,
	// which 4.3 UI client relies on to create URL's and display node inform.
	clusterName := p.ByName("site")
	for i, session := range sessions {
		if session.ClusterName == "" {
			sessions[i].ClusterName = clusterName
		}
		if session.ServerHostname == "" {
			sessions[i].ServerHostname = session.ServerID
		}
	}

	return siteSessionsGetResponse{Sessions: sessions}, nil
}

// siteSessionGet gets the list of site session by id
//
// GET /v1/webapi/sites/:site/namespaces/:namespace/sessions/:sid
//
// Response body:
//
// {"session": {"id": "sid", "terminal_params": {"w": 100, "h": 100}, "parties": [], "login": "bob"}}
//
func (h *Handler) siteSessionGet(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	sessionID, err := session.ParseID(p.ByName("sid"))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	h.log.Infof("web.getSession(%v)", sessionID)

	clt, err := ctx.GetUserClient(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	namespace := p.ByName("namespace")
	if !services.IsValidNamespace(namespace) {
		return nil, trace.BadParameter("invalid namespace %q", namespace)
	}

	sess, err := clt.GetSession(namespace, *sessionID)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// DELETE IN: 5.0.0
	// Teleport Nodes < v4.3 does not set ClusterName, ServerHostname with new sessions,
	// which 4.3 UI client relies on to create URL's and display node inform.
	if sess.ClusterName == "" {
		sess.ClusterName = p.ByName("site")
	}
	if sess.ServerHostname == "" {
		sess.ServerHostname = sess.ServerID
	}

	return *sess, nil
}

const maxStreamBytes = 5 * 1024 * 1024

// clusterSearchSessionEvents allows to search for session events on a cluster
//
// GET /v1/webapi/sites/:site/events
//
// Query parameters:
//   "from"  : date range from, encoded as RFC3339
//   "to"    : date range to, encoded as RFC3339
//   ...     : the rest of the query string is passed to the search back-end as-is,
//             the default backend performs exact search: ?key=value means "event
//             with a field 'key' with value 'value'
//
func (h *Handler) clusterSearchSessionEvents(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	query := r.URL.Query()

	clt, err := ctx.GetUserClient(site)
	if err != nil {
		h.log.WithError(err).Error("Failed to query user for site.")
		return nil, trace.Wrap(err)
	}

	// default values
	to := time.Now().In(time.UTC)
	from := to.AddDate(0, -1, 0) // one month ago

	// parse 'to' and 'from' params:
	fromStr := query.Get("from")
	if fromStr != "" {
		from, err = time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return nil, trace.BadParameter("from")
		}
	}
	toStr := query.Get("to")
	if toStr != "" {
		to, err = time.Parse(time.RFC3339, toStr)
		if err != nil {
			return nil, trace.BadParameter("to")
		}
	}

	el, err := clt.SearchSessionEvents(from, to, defaults.EventsIterationLimit)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return eventsListGetResponse{Events: el}, nil
}

// clusterSearchEvents returns all audit log events matching the provided criteria
//
// GET /v1/webapi/sites/:site/events/search
//
// Query parameters:
//   "from"   : date range from, encoded as RFC3339
//   "to"     : date range to, encoded as RFC3339
//   "include": optional semicolon-separated list of event names to return e.g.
//              include=session.start;session.end, all are returned if empty
//   "limit"  : optional maximum number of events to return
//
func (h *Handler) clusterSearchEvents(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	values := r.URL.Query()
	from, err := queryTime(values, "from", time.Now().UTC().AddDate(0, -1, 0))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	to, err := queryTime(values, "to", time.Now().UTC())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	limit, err := queryLimit(values, "limit", defaults.EventsIterationLimit)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	query := url.Values{}
	if include := values.Get("include"); include != "" {
		query[events.EventType] = strings.Split(include, ";")
	}
	clt, err := ctx.GetUserClient(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	fields, err := clt.SearchEvents(from, to, query.Encode(), limit)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return eventsListGetResponse{Events: fields}, nil
}

// queryTime parses the query string parameter with the specified name as a
// RFC3339 time and returns it.
//
// If there's no such parameter, specified default value is returned.
func queryTime(query url.Values, name string, def time.Time) (time.Time, error) {
	str := query.Get(name)
	if str == "" {
		return def, nil
	}
	parsed, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return time.Time{}, trace.BadParameter("failed to parse %v as RFC3339 time: %v", name, str)
	}
	return parsed, nil
}

// queryLimit returns the limit parameter with the specified name from the
// query string.
//
// If there's no such parameter, specified default limit is returned.
func queryLimit(query url.Values, name string, def int) (int, error) {
	str := query.Get(name)
	if str == "" {
		return def, nil
	}
	limit, err := strconv.Atoi(str)
	if err != nil {
		return 0, trace.BadParameter("failed to parse %v as limit: %v", name, str)
	}
	return limit, nil
}

// siteSessionStreamGet returns a byte array from a session's stream
//
// GET /v1/webapi/sites/:site/namespaces/:namespace/sessions/:sid/stream?query
//
// Query parameters:
//   "offset"   : bytes from the beginning
//   "bytes"    : number of bytes to read (it won't return more than 512Kb)
//
// Unlike other request handlers, this one does not return JSON.
// It returns the binary stream unencoded, directly in the respose body,
// with Content-Type of application/octet-stream, gzipped with up to 95%
// compression ratio.
func (h *Handler) siteSessionStreamGet(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	httplib.SetNoCacheHeaders(w.Header())

	var site reversetunnel.RemoteSite
	onError := func(err error) {
		h.log.WithError(err).Debug("Unable to retrieve session chunk.")
		http.Error(w, err.Error(), trace.ErrorToCode(err))
	}
	// authenticate first:
	ctx, err := h.AuthenticateRequest(w, r, true)
	if err != nil {
		h.log.WithError(err).Warn("Failed to authenticate.")
		// clear session just in case if the authentication request is not valid
		ClearSession(w)
		onError(err)
		return
	}
	// get the site interface:
	siteName := p.ByName("site")
	if siteName == currentSiteShortcut {
		sites, err := h.cfg.Proxy.GetSites()
		if err != nil {
			onError(trace.Wrap(err))
			return
		}
		if len(sites) < 1 {
			onError(trace.NotFound("no active sites"))
			return
		}
		siteName = sites[0].GetName()
	}
	site, err = h.cfg.Proxy.GetSite(siteName)
	if err != nil {
		onError(err)
		return
	}
	// get the session:
	sid, err := session.ParseID(p.ByName("sid"))
	if err != nil {
		onError(trace.Wrap(err))
		return
	}

	clt, err := ctx.GetUserClient(site)
	if err != nil {
		onError(trace.Wrap(err))
		return
	}

	// look at 'offset' parameter
	query := r.URL.Query()
	offset, _ := strconv.Atoi(query.Get("offset"))
	if err != nil {
		onError(trace.Wrap(err))
		return
	}
	max, err := strconv.Atoi(query.Get("bytes"))
	if err != nil || max <= 0 {
		max = maxStreamBytes
	}
	if max > maxStreamBytes {
		max = maxStreamBytes
	}
	namespace := p.ByName("namespace")
	if !services.IsValidNamespace(namespace) {
		onError(trace.BadParameter("invalid namespace %q", namespace))
		return
	}

	// call the site API to get the chunk:
	bytes, err := clt.GetSessionChunk(namespace, *sid, offset, max)
	if err != nil {
		onError(trace.Wrap(err))
		return
	}
	// see if we can gzip it:
	var writer io.Writer = w
	for _, acceptedEnc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		if strings.TrimSpace(acceptedEnc) == "gzip" {
			gzipper := gzip.NewWriter(w)
			writer = gzipper
			defer gzipper.Close()
			w.Header().Set("Content-Encoding", "gzip")
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = writer.Write(bytes)
	if err != nil {
		onError(trace.Wrap(err))
		return
	}
}

type eventsListGetResponse struct {
	Events []events.EventFields `json:"events"`
}

// siteSessionEventsGet gets the site session by id
//
// GET /v1/webapi/sites/:site/namespaces/:namespace/sessions/:sid/events?after=N
//
// Query:
//    "after" : cursor value of an event to return "newer than" events
//              good for repeated polling
//
// Response body (each event is an arbitrary JSON structure)
//
// {"events": [{...}, {...}, ...}
//
func (h *Handler) siteSessionEventsGet(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error) {
	sessionID, err := session.ParseID(p.ByName("sid"))
	if err != nil {
		return nil, trace.BadParameter("invalid session ID %q", p.ByName("sid"))
	}

	clt, err := ctx.GetUserClient(site)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	afterN, err := strconv.Atoi(r.URL.Query().Get("after"))
	if err != nil {
		afterN = 0
	}
	namespace := p.ByName("namespace")
	if !services.IsValidNamespace(namespace) {
		return nil, trace.BadParameter("invalid namespace %q", namespace)
	}
	e, err := clt.GetSessionEvents(namespace, *sessionID, afterN, true)
	if err != nil {
		h.log.WithError(err).Debugf("Unable to find events for session %v.", sessionID)
		if trace.IsNotFound(err) {
			return nil, trace.NotFound("unable to find events for session %q", sessionID)
		}

		return nil, trace.Wrap(err)
	}
	return eventsListGetResponse{Events: e}, nil
}

// hostCredentials sends a registration token and metadata to the Auth Server
// and gets back SSH and TLS certificates.
func (h *Handler) hostCredentials(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var req auth.RegisterUsingTokenRequest
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	authClient := h.cfg.ProxyClient
	packedKeys, err := authClient.RegisterUsingToken(req)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return packedKeys, nil
}

// createSSHCert is a web call that generates new SSH certificate based
// on user's name, password, 2nd factor token and public key user wishes to sign
//
// POST /v1/webapi/ssh/certs
//
// { "user": "bob", "password": "pass", "otp_token": "tok", "pub_key": "key to sign", "ttl": 1000000000 }
//
// Success response
//
// { "cert": "base64 encoded signed cert", "host_signers": [{"domain_name": "example.com", "checking_keys": ["base64 encoded public signing key"]}] }
//
func (h *Handler) createSSHCert(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var req *client.CreateSSHCertReq
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	authClient := h.cfg.ProxyClient
	cap, err := authClient.GetAuthPreference()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var cert *auth.SSHLoginResponse

	switch cap.GetSecondFactor() {
	case teleport.OFF:
		cert, err = h.auth.GetCertificateWithoutOTP(*req)
	case teleport.OTP, teleport.HOTP, teleport.TOTP:
		// convert legacy requests to new parameter here. remove once migration to TOTP is complete.
		if req.HOTPToken != "" {
			req.OTPToken = req.HOTPToken
		}
		cert, err = h.auth.GetCertificateWithOTP(*req)
	default:
		return nil, trace.AccessDenied("unknown second factor type: %q", cap.GetSecondFactor())
	}
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return cert, nil
}

// createSSHCertWithU2FSignResponse is a web call that generates new SSH certificate based
// on user's name, password, U2F signature and public key user wishes to sign
//
// POST /v1/webapi/u2f/certs
//
// { "user": "bob", "password": "pass", "u2f_sign_response": { "signatureData": "signatureinbase64", "clientData": "verylongbase64string", "challenge": "randombase64string" }, "pub_key": "key to sign", "ttl": 1000000000 }
//
// Success response
//
// { "cert": "base64 encoded signed cert", "host_signers": [{"domain_name": "example.com", "checking_keys": ["base64 encoded public signing key"]}] }
//
func (h *Handler) createSSHCertWithU2FSignResponse(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var req *client.CreateSSHCertWithU2FReq
	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	cert, err := h.auth.GetCertificateWithU2F(*req)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return cert, nil
}

// validateTrustedCluster validates the token for a trusted cluster and returns it's own host and user certificate authority.
//
// POST /webapi/trustedclusters/validate
//
// * Request body:
//
// {
//     "token": "foo",
//     "certificate_authorities": ["AQ==", "Ag=="]
// }
//
// * Response:
//
// {
//     "certificate_authorities": ["AQ==", "Ag=="]
// }
func (h *Handler) validateTrustedCluster(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	var validateRequestRaw auth.ValidateTrustedClusterRequestRaw
	if err := httplib.ReadJSON(r, &validateRequestRaw); err != nil {
		return nil, trace.Wrap(err)
	}

	validateRequest, err := validateRequestRaw.ToNative()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	validateResponse, err := h.auth.ValidateTrustedCluster(validateRequest)
	if err != nil {
		h.log.WithError(err).Error("Failed validating trusted cluster")
		if trace.IsAccessDenied(err) {
			return nil, trace.AccessDenied("access denied: the cluster token has been rejected")
		}
		return nil, trace.Wrap(err)
	}

	validateResponseRaw, err := validateResponse.ToRaw()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return validateResponseRaw, nil
}

func (h *Handler) String() string {
	return "multi site"
}

// currentSiteShortcut is a special shortcut that will return the first
// available site, is helpful when UI works in single site mode to reduce
// the amount of requests
const currentSiteShortcut = "-current-"

// ContextHandler is a handler called with the auth context, what means it is authenticated and ready to work
type ContextHandler func(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext) (interface{}, error)

// ClusterHandler is a authenticated handler that is called for some existing remote cluster
type ClusterHandler func(w http.ResponseWriter, r *http.Request, p httprouter.Params, ctx *SessionContext, site reversetunnel.RemoteSite) (interface{}, error)

// WithClusterAuth ensures that request is authenticated and is issued for existing cluster
func (h *Handler) WithClusterAuth(fn ClusterHandler) httprouter.Handle {
	return httplib.MakeHandler(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
		ctx, err := h.AuthenticateRequest(w, r, true)
		if err != nil {
			h.log.WithError(err).Warn("Failed to authenticate.")
			return nil, trace.Wrap(err)
		}
		clusterName := p.ByName("site")
		if clusterName == currentSiteShortcut {
			res, err := h.cfg.ProxyClient.GetClusterName()
			if err != nil {
				h.log.WithError(err).Warn("Failed to query cluster name.")
				return nil, trace.Wrap(err)
			}

			clusterName = res.GetClusterName()
		}
		site, err := h.cfg.Proxy.GetSite(clusterName)
		if err != nil {
			h.log.WithError(err).WithField("cluster-name", clusterName).Warn("Failed to query site.")
			return nil, trace.Wrap(err)
		}

		return fn(w, r, p, ctx, site)
	})
}

// WithAuth ensures that request is authenticated
func (h *Handler) WithAuth(fn ContextHandler) httprouter.Handle {
	return httplib.MakeHandler(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
		ctx, err := h.AuthenticateRequest(w, r, true)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return fn(w, r, p, ctx)
	})
}

// AuthenticateRequest authenticates request using combination of a session cookie
// and bearer token
func (h *Handler) AuthenticateRequest(w http.ResponseWriter, r *http.Request, checkBearerToken bool) (*SessionContext, error) {
	const missingCookieMsg = "missing session cookie"
	logger := h.log.WithField("request", fmt.Sprintf("%v %v", r.Method, r.URL.Path))
	cookie, err := r.Cookie(CookieName)
	if err != nil || (cookie != nil && cookie.Value == "") {
		if err != nil {
			logger.Warn(err)
		}
		return nil, trace.AccessDenied(missingCookieMsg)
	}
	d, err := DecodeCookie(cookie.Value)
	if err != nil {
		logger.WithError(err).Warn("Failed to decode cookie.")
		return nil, trace.AccessDenied("failed to decode cookie")
	}
	ctx, err := h.auth.ValidateSession(d.User, d.SID)
	if err != nil {
		logger.WithError(err).Warn("Invalid session.")
		ClearSession(w)
		return nil, trace.AccessDenied("need auth")
	}
	if checkBearerToken {
		creds, err := roundtrip.ParseAuthHeaders(r)
		if err != nil {
			logger.WithError(err).Warn("No auth headers.")
			return nil, trace.AccessDenied("need auth")
		}

		if subtle.ConstantTimeCompare([]byte(creds.Password), []byte(ctx.GetWebSession().GetBearerToken())) != 1 {
			logger.Warn("Request failed: bad bearer token.")
			return nil, trace.AccessDenied("bad bearer token")
		}
	}
	return ctx, nil
}

// ProxyHostPort returns the address of the proxy server using --proxy
// notation, i.e. "localhost:8030,8023"
func (h *Handler) ProxyHostPort() string {
	return fmt.Sprintf("%s,%s", h.cfg.ProxyWebAddr.String(), h.sshPort)
}

func message(msg string) interface{} {
	return map[string]interface{}{"message": msg}
}

func ok() interface{} {
	return message("ok")
}

type responseData struct {
	Items interface{} `json:"items"`
}

func makeResponse(items interface{}) (interface{}, error) {
	return responseData{Items: items}, nil
}

// makeTeleportClientConfig creates default teleport client configuration
// that is used to initiate an SSH terminal session or SCP file transfer
func makeTeleportClientConfig(ctx *SessionContext) (*client.Config, error) {
	agent, cert, err := ctx.GetAgent()
	if err != nil {
		return nil, trace.BadParameter("failed to get user credentials: %v", err)
	}

	signers, err := agent.Signers()
	if err != nil {
		return nil, trace.BadParameter("failed to get user credentials: %v", err)
	}

	tlsConfig, err := ctx.ClientTLSConfig()
	if err != nil {
		return nil, trace.BadParameter("failed to get client TLS config: %v", err)
	}

	config := &client.Config{
		Username:         ctx.user,
		Agent:            agent,
		SkipLocalAuth:    true,
		TLS:              tlsConfig,
		AuthMethods:      []ssh.AuthMethod{ssh.PublicKeys(signers...)},
		DefaultPrincipal: cert.ValidPrincipals[0],
		HostKeyCallback:  func(string, net.Addr, ssh.PublicKey) error { return nil },
	}

	return config, nil
}
