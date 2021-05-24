package fluentd

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gravitational/teleport-plugins/fluentd/config"
	"github.com/gravitational/trace"
)

var (
	// client is https client
	client *http.Client
)

// Init initializes HTTPS connection to fluentd
func Init() error {
	cert, err := tls.LoadX509KeyPair(config.GetFluentdCert(), config.GetFluentdKey())
	if err != nil {
		return trace.Wrap(err)
	}

	ca, err := getCertPool()
	if err != nil {
		return trace.Wrap(err)
	}

	client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      ca,
				Certificates: []tls.Certificate{cert},
			},
		},
	}

	return nil
}

// Send sends event to fluentd
func Send(obj interface{}) error {
	if client == nil {
		return trace.Errorf("Call fluentd.InitConnection() before calling Send()")
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return trace.Wrap(err)
	}

	r, err := client.Post(config.GetFluentdURL(), "application/json", bytes.NewReader(b))
	if err != nil {
		return trace.Wrap(err)
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return trace.Errorf("Failed to send event to fluentd (HTTP %v)", r.StatusCode)
	}

	return nil
}

// getCertPool reads CA certificate and returns CA cert pool if passed
func getCertPool() (*x509.CertPool, error) {
	if config.GetFluentdCA() == "" {
		return nil, nil
	}

	caCert, err := ioutil.ReadFile(config.GetFluentdCA())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return caCertPool, nil
}
