package access

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"os"
	"time"

	"github.com/gravitational/trace"
)

var ErrInvalidCertificate = errors.New("invalid certificate")

func LoadTLSConfig(certPath, keyPath, rootCAsPath string) (conf *tls.Config, err error) {
	clientCert, err := LoadX509Cert(certPath, keyPath)
	if err != nil {
		return
	}
	caPool, err := LoadX509CertPool(rootCAsPath)
	if err != nil {
		return
	}
	now := time.Now()
	if now.After(clientCert.Leaf.NotAfter) {
		err = trace.Wrap(ErrInvalidCertificate, "certificate seems to be expired, you should renew it.")
	}
	if now.Before(clientCert.Leaf.NotBefore) {
		err = trace.Wrap(ErrInvalidCertificate, "certificate seems to be invalid, check its notBefore date.")
	}
	conf = &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
	}
	return
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
