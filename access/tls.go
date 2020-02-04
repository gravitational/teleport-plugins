package access

import (
	"os"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"github.com/gravitational/trace"
)

// LoadTLSCert loads a X.509 keypair from file paths and retains parsed form of
// the certificate.
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
