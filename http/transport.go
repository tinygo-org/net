// TINYGO: The following is copied and modified from Go 1.21.4 official implementation.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP client implementation. See RFC 7230 through 7235.
//
// This is the low-level Transport implementation of RoundTripper.
// The high-level interface is in client.go.

package http

import (
	"io"
)

type readTrackingBody struct {
	io.ReadCloser
	didRead  bool
	didClose bool
}

type Transport struct{}

var DefaultTransport RoundTripper = &Transport{}

// roundTrip implements a RoundTripper over HTTP.
func (t *Transport) RoundTrip(req *Request) (*Response, error) {
	return roundTrip(req)
}
