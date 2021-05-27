package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
)

const (
	// caCrtPath is a path to root CA
	caCrtPath = "example/keys/ca.crt"

	// crtPath is the path to server certificate
	crtPath = "example/keys/server_nopass.crt"

	// keyPath is the path to server key
	keyPath = "example/keys/server_nopass.key"
)

var (
	// fluentdConfig is app configuration with all required client variables
	fluentdConfig = &Config{
		FluentdCA:   caCrtPath,
		FluentdCert: "example/keys/client.crt",
		FluentdKey:  "example/keys/client.key",
	}

	// obj represents mock object
	obj = struct {
		A string
		B string
	}{"Test", "Value"}
)

// setupTLS reads and prepares tls.Config for test server
func setupTLS() (*tls.Config, error) {
	caCert, err := ioutil.ReadFile(caCrtPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)

	cert, err := tls.LoadX509KeyPair(crtPath, keyPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}}, nil
}

// newTLSServer constructs an instance of unstarted TLS server
func newTLSServer(t *testing.T) (*httptest.Server, error) {
	tlsConfig, err := setupTLS()
	if err != nil {
		return nil, err
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that request body contains test object JSON representation
		objJSON, err := json.Marshal(obj)
		require.NoError(t, err)

		var req []byte = make([]byte, len(objJSON))

		// We omit err here because it always returns weird EOF. It has something to do with httptest. TODO: find out and resolve.
		r.Body.Read(req)
		require.NoError(t, err)

		require.Equal(t, objJSON, req)
		require.NoError(t, err)

		fmt.Fprintln(w, "OK")
	}))
	ts.TLS = tlsConfig

	return ts, nil
}

func TestSend(t *testing.T) {
	ts, err := newTLSServer(t)
	require.NoError(t, err)

	ts.StartTLS()
	defer ts.Close()

	fluentdConfig.FluentdURL = ts.URL

	f, err := NewFluentdClient(fluentdConfig)
	require.NoError(t, err)

	err = f.Send(obj)
	require.NoError(t, err)
}
