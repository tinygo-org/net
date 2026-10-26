//go:build !darwin

// TINYGO: trust roots for the HTTPS client, everywhere but darwin.

package http

import "crypto/tls"

// defaultTLSConfig returns the config that the client dials HTTPS with. Off
// darwin crypto/x509 reads the system roots from the usual certificate files,
// so a nil config is correct.
func defaultTLSConfig() *tls.Config {
	return nil
}
