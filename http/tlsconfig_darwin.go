//go:build darwin

// TINYGO: darwin trust roots for the HTTPS client.

package http

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"sync"
)

// certFileEnv is the environment variable that crypto/x509 reads on unix for
// an alternative root bundle.
const certFileEnv = "SSL_CERT_FILE"

// darwinCertFile is the PEM bundle of macOS. crypto/x509 does not read it on
// darwin, because it uses the system verifier, so the client must read it.
const darwinCertFile = "/etc/ssl/cert.pem"

var darwinRoots struct {
	once sync.Once
	pool *x509.CertPool
}

// defaultTLSConfig returns the config that the client dials HTTPS with.
//
// On darwin crypto/x509 verifies a certificate with the platform verifier and
// not with a root pool, and crypto/x509/internal/macos in TinyGo is a stub, so
// a nil RootCAs makes every verification fail. A non-nil pool sends x509 to its
// pure Go path, which works. The roots come from $SSL_CERT_FILE, or from
// /etc/ssl/cert.pem.
//
// If no file can be read, the config has no RootCAs, so the result is an
// ordinary verification error and not a check that is silently skipped.
func defaultTLSConfig() *tls.Config {
	darwinRoots.once.Do(loadDarwinRoots)
	if darwinRoots.pool == nil {
		return nil
	}
	return &tls.Config{RootCAs: darwinRoots.pool}
}

func loadDarwinRoots() {
	path := os.Getenv(certFileEnv)
	if path == "" {
		path = darwinCertFile
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pool := x509.NewCertPool()
	if pool.AppendCertsFromPEM(pem) {
		darwinRoots.pool = pool
	}
}
