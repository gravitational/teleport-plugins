package access

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"os"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/trace"
)

var ErrInvalidCertificate = errors.New("invalid certificate")

func LoadTLSConfig(conf lib.TeleportConfig) (*tls.Config, error) {
	creds := conf.Credentials()
	if len(creds) == 0 {
		return nil, trace.Errorf("no credentials found in the config")
	}
	return creds[0].TLSConfig()
}

// LoadTLSCert loads a X.509 keypair from file paths and
// retains parsed form of the certificate.
func LoadX509Cert(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, trace.Wrap(err)
	}

	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, trace.Wrap(err)
	}

	return cert, nil
}

// LoadTLSCertPool is useful to load root CA certs from file path.
func LoadX509CertPool(path string) (*x509.CertPool, error) {
	caFile, err := os.Open(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	caCerts, err := ioutil.ReadAll(caFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caCerts); !ok {
		return nil, trace.BadParameter("invalid CA cert PEM")
	}
	return pool, nil
}
