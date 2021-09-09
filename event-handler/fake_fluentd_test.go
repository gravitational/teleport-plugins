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
	"strings"

	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

const (
	// caCrtPath is a path to root CA
	caCrtPath = "example/keys/ca.crt"
	// crtPath is the path to the fluentd server certificate
	crtPath = "example/keys/server_nopass.crt"
	// keyPath is the path to the fluentd server key
	keyPath = "example/keys/server_nopass.key"
	// clientCrtPath is the path to the fluentd client certificate
	clientCrtPath = "example/keys/client.crt"
	// clientKeyPath is the path to the fluentd client key
	clientKeyPath = "example/keys/client.key"
)

var (
	// fluentdTestConfig is the app configuration with all required client variables
	fluentdTestConfig = &FluentdConfig{
		FluentdCA:   caCrtPath,
		FluentdCert: clientCrtPath,
		FluentdKey:  clientKeyPath,
	}
)

type FakeFluentd struct {
	server     *httptest.Server
	chMessages chan string
}

// NewFakeFluentd creates new unstarted fake server instance
func NewFakeFluentd(concurrency int) (*FakeFluentd, error) {
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

	tlsConfig := &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}}

	f := &FakeFluentd{}

	f.server = httptest.NewUnstartedServer(http.HandlerFunc(f.Respond))
	f.server.TLS = tlsConfig
	f.chMessages = make(chan string, concurrency)

	return f, nil
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
