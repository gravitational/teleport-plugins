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
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"

	"github.com/gravitational/teleport-plugins/event-handler/lib"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// FluentdClient represents Fluentd client
type FluentdClient struct {
	// client HTTP client to send requests
	client *http.Client
}

// NewFluentdClient creates new FluentdClient
func NewFluentdClient(c *FluentdConfig) (*FluentdClient, error) {
	cert, err := tls.LoadX509KeyPair(c.FluentdCert, c.FluentdKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	ca, err := getCertPool(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      ca,
				Certificates: []tls.Certificate{cert},
			},
		},
	}

	return &FluentdClient{client: client}, nil
}

// getCertPool reads CA certificate and returns CA cert pool if passed
func getCertPool(c *FluentdConfig) (*x509.CertPool, error) {
	if c.FluentdCA == "" {
		return nil, nil
	}

	caCert, err := ioutil.ReadFile(c.FluentdCA)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return caCertPool, nil
}

// Send sends event to fluentd
func (f *FluentdClient) Send(url string, obj interface{}) error {
	b, err := lib.FastMarshal(obj)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithField("json", string(b)).Debug("JSON to send")

	r, err := f.client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return trace.Wrap(err)
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return trace.Errorf("Failed to send event to fluentd (HTTP %v)", r.StatusCode)
	}

	return nil
}