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
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gravitational/trace"
)

// FluentdClient represents Fluentd client
type FluentdClient struct {
	// client HTTP client to send requests
	client *http.Client

	// url is a fluentd url taken from config
	url string
}

// New creates new FluentdClient
func NewFluentdClient(c *Config) (*FluentdClient, error) {
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

	return &FluentdClient{client: client, url: c.FluentdURL}, nil
}

// getCertPool reads CA certificate and returns CA cert pool if passed
func getCertPool(c *Config) (*x509.CertPool, error) {
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
func (f *FluentdClient) Send(obj interface{}) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return trace.Wrap(err)
	}

	r, err := f.client.Post(f.url, "application/json", bytes.NewReader(b))
	if err != nil {
		return trace.Wrap(err)
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return trace.Errorf("Failed to send event to fluentd (HTTP %v)", r.StatusCode)
	}

	return nil
}
