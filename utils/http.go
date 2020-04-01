package utils

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

type HTTPConfig struct {
	Listen   string `toml:"listen"`
	KeyFile  string `toml:"https-key-file"`
	CertFile string `toml:"https-cert-file"`
	Hostname string `toml:"host"`
	BaseURL  string `toml:"base-url"`
	Insecure bool
}

// HTTP is a tiny wrapper around standard net/http.
// It starts either insecure server or secure one with TLS, depending on the settings.
// It also adds a context to its handlers and the server itself has context to.
// So you are guaranteed that server will be closed when the context is cancelled.
type HTTP struct {
	HTTPConfig
	*httprouter.Router
	server http.Server
}

// NewHTTP creates a new HTTP wrapper
func NewHTTP(config HTTPConfig) *HTTP {
	router := httprouter.New()
	return &HTTP{
		config,
		router,
		http.Server{Addr: config.Listen, Handler: router},
	}
}

// ListenAndServe runs a http(s) server on a provided port.
func (h *HTTP) ListenAndServe(ctx context.Context) error {
	defer log.Debug("HTTP server terminated")

	h.server.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}
	go func() {
		<-ctx.Done()
		h.server.Close()
	}()

	var err error
	if h.Insecure {
		log.Debugf("Starting insecure HTTP server on %s", h.Listen)
		err = h.server.ListenAndServe()
	} else {
		log.Debugf("Starting secure HTTPS server on %s", h.Listen)
		err = h.server.ListenAndServeTLS(h.CertFile, h.KeyFile)
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return trace.Wrap(err)
}

// Shutdown stops the server gracefully.
func (h *HTTP) Shutdown(ctx context.Context) error {
	return h.server.Shutdown(ctx)
}

// ShutdownWithTimeout stops the server gracefully.
func (h *HTTP) ShutdownWithTimeout(ctx context.Context, duration time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	return h.Shutdown(ctx)
}

// BaseURL returns an url on which the server is accessible externally.
func (h *HTTP) BaseURL() (*url.URL, error) {
	if h.HTTPConfig.BaseURL != "" {
		return url.Parse(h.HTTPConfig.BaseURL)
	} else if h.Hostname != "" {
		url := &url.URL{Host: h.Hostname}
		if h.Insecure {
			url.Scheme = "http"
		} else {
			url.Scheme = "https"
		}
		return url, nil
	} else {
		return nil, trace.BadParameter("no hostname or base-url was provided")
	}
}

// NewURL builds an external url for a specific path and query parameters.
func (h *HTTP) NewURL(subpath string, values url.Values) (*url.URL, error) {
	url, err := h.BaseURL()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	url.Path = path.Join(url.Path, subpath)

	if values != nil {
		url.RawQuery = values.Encode()
	}

	return url, nil
}

// EnsureCert checks cert and key files consistency. It also generates a self-signed cert if it was not specified.
func (h *HTTP) EnsureCert(defaultPath string) (err error) {
	if h.Insecure {
		return nil
	}
	// If files are specified by user then they should exist and possess right structure
	if h.CertFile != "" {
		_, err = tls.LoadX509KeyPair(h.CertFile, h.KeyFile)
		return err
	}

	log.Warningf("No TLS Keys provided, using self signed certificate.")

	// If files are not specified, try to fall back on self signed certificate.
	h.CertFile = defaultPath + ".crt"
	h.KeyFile = defaultPath + ".key"
	_, err = tls.LoadX509KeyPair(h.CertFile, h.KeyFile)
	if err == nil {
		// self-signed cert was generated previously
		return nil
	}
	if !os.IsNotExist(err) {
		return trace.Wrap(err, "unrecognized error reading certs")
	}

	log.Warningf("Generating self signed key and cert to %v %v.", h.KeyFile, h.CertFile)

	hostname := h.Hostname
	if hostname == "" && h.HTTPConfig.BaseURL != "" {
		url, err := h.BaseURL()
		if err != nil {
			return trace.Wrap(err)
		}
		hostname = url.Hostname()
	}
	if hostname == "" {
		return trace.BadParameter("no hostname or base-url was provided")
	}

	creds, err := utils.GenerateSelfSignedCert([]string{hostname, "localhost"})
	if err != nil {
		return trace.Wrap(err)
	}

	if err := ioutil.WriteFile(h.KeyFile, creds.PrivateKey, 0600); err != nil {
		return trace.Wrap(err, "error writing key PEM")
	}
	if err := ioutil.WriteFile(h.CertFile, creds.Cert, 0600); err != nil {
		return trace.Wrap(err, "error writing cert PEM")
	}
	return nil
}
