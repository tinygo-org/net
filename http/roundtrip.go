// TINYGO: The following is copied and modified from Go 1.21.4 official implementation.

//go:build !js

package http

// RoundTrip implements a RoundTripper over HTTP.
func (t *Transport) RoundTrip(req *Request) (*Response, error) {
	return roundTrip(req)
}
