/*
Copyright 2015-2021 Gravitational, Inc.

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

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

type FakeFluentd struct {
	keyTmpDir      string
	caCertPath     string
	caKeyPath      string
	clientCertPath string
	clientKeyPath  string
	serverCertPath string
	serverKeyPath  string

	server     *httptest.Server
	chMessages chan string
}

// NewFakeFluentd creates new unstarted fake server instance
func NewFakeFluentd(concurrency int) (*FakeFluentd, error) {
	dir, err := ioutil.TempDir("", "teleport-plugins-event-handler-*")
	if err != nil {
		return nil, trace.Wrap(err)
	}

	f := &FakeFluentd{keyTmpDir: dir}
	err = f.writeCerts()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = f.createServer(concurrency)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	f.chMessages = make(chan string, concurrency)

	return f, nil
}

// writeCerts generates and writes temporary mTLS keys
func (f *FakeFluentd) writeCerts() error {
	g, err := GenerateMTLSCerts("localhost", []string{"localhost"}, []string{}, time.Hour, 1024)
	if err != nil {
		return trace.Wrap(err)
	}

	f.caCertPath = path.Join(f.keyTmpDir, "ca.crt")
	f.caKeyPath = path.Join(f.keyTmpDir, "ca.key")
	f.serverCertPath = path.Join(f.keyTmpDir, "server.crt")
	f.serverKeyPath = path.Join(f.keyTmpDir, "server.key")
	f.clientCertPath = path.Join(f.keyTmpDir, "client.crt")
	f.clientKeyPath = path.Join(f.keyTmpDir, "client.key")

	err = g.CACert.WriteFile(f.caCertPath, f.caKeyPath, "")
	if err != nil {
		return trace.Wrap(err)
	}

	err = g.ServerCert.WriteFile(f.serverCertPath, f.serverKeyPath, "")
	if err != nil {
		return trace.Wrap(err)
	}

	err = g.ClientCert.WriteFile(f.clientCertPath, f.clientKeyPath, "")
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// createServer initialises new server instance
func (f *FakeFluentd) createServer(concurrency int) error {
	caCert, err := ioutil.ReadFile(f.caCertPath)
	if err != nil {
		return trace.Wrap(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)

	cert, err := tls.LoadX509KeyPair(f.serverCertPath, f.serverKeyPath)
	if err != nil {
		return trace.Wrap(err)
	}

	tlsConfig := &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}}

	f.server = httptest.NewUnstartedServer(http.HandlerFunc(f.Respond))
	f.server.TLS = tlsConfig

	return nil
}

// GetClientConfig returns FlientdConfig to connect to this fake fluentd server instance
func (f *FakeFluentd) GetClientConfig() FluentdConfig {
	return FluentdConfig{
		FluentdCA:   f.caCertPath,
		FluentdCert: f.clientCertPath,
		FluentdKey:  f.clientKeyPath,
	}
}

// Start starts fake fluentd server
func (f *FakeFluentd) Start() {
	f.server.StartTLS()
}

// Close closes fake fluentd server
func (f *FakeFluentd) Close() {
	f.server.Close()
	close(f.chMessages)
}

// GetURL returns fake server URL
func (f *FakeFluentd) GetURL() string {
	return f.server.URL
}

// Respond is the response function
func (f *FakeFluentd) Respond(w http.ResponseWriter, r *http.Request) {
	var req []byte = make([]byte, r.ContentLength)

	_, err := r.Body.Read(req)
	// We omit err here because it always returns weird EOF.
	// It has something to do with httptest, known bug.
	// TODO: find out and resolve.
	if !trace.IsEOF(err) {
		logger.Standard().WithError(err).Error("FakeFluentd Respond() failed")
	}

	f.chMessages <- strings.TrimSpace(string(req))
	fmt.Fprintln(w, "OK")
}

// GetMessage reads next message from a mock server buffer
func (f *FakeFluentd) GetMessage(ctx context.Context) (string, error) {
	select {
	case message := <-f.chMessages:
		return message, nil
	case <-ctx.Done():
		return "", trace.Wrap(ctx.Err())
	}
}
