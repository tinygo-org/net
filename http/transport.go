// TINYGO: The following is copied and modified from Go 1.26.2 official implementation.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP client implementation. See RFC 7230 through 7235.
//
// This is the low-level Transport implementation of RoundTripper.
// The high-level interface is in client.go.

package http

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/url"
	"sync/atomic"
	"time"
)

type readTrackingBody struct {
	io.ReadCloser
	didRead  bool // not atomic.Bool because only one goroutine (the user's) should be accessing
	didClose atomic.Bool
}

// Transport is an HTTP/1.x RoundTripper. TINYGO: in the wasm/browser build the
// actual round trips are performed by the host fetch API (see roundtrip_js.go);
// these fields exist only to satisfy configuration by callers such as
// golang.org/x/net/http2 and google.golang.org/grpc. They are stored but, aside
// from fetch, have no effect at runtime.
type Transport struct {
	// Proxy specifies a function to return a proxy for a given Request.
	//
	// TINYGO: present for API compatibility; the netdev-backed transport dials
	// directly and does not consult it.
	Proxy func(*Request) (*url.URL, error)

	// TLSClientConfig specifies the TLS configuration to use with tls.Client.
	TLSClientConfig *tls.Config

	// TLSNextProto specifies how the Transport switches to an alternate
	// protocol (such as HTTP/2) after a TLS ALPN protocol negotiation.
	TLSNextProto map[string]func(authority string, c *tls.Conn) RoundTripper

	// DisableKeepAlives, if true, disables HTTP keep-alives.
	DisableKeepAlives bool

	// DisableCompression, if true, prevents the Transport from requesting
	// compression with an "Accept-Encoding: gzip" request header.
	DisableCompression bool

	// IdleConnTimeout is the maximum amount of time an idle connection will
	// remain idle before closing itself. Zero means no limit.
	IdleConnTimeout time.Duration

	// ResponseHeaderTimeout, if non-zero, specifies the amount of time to wait
	// for a server's response headers after fully writing the request.
	ResponseHeaderTimeout time.Duration

	// ExpectContinueTimeout, if non-zero, specifies the amount of time to wait
	// for a server's first response headers after fully writing the request
	// headers if the request has an "Expect: 100-continue" header.
	ExpectContinueTimeout time.Duration

	// TLSHandshakeTimeout specifies the maximum amount of time to
	// wait for a TLS handshake. Zero means no timeout.
	// Accepted for source compatibility with net/http; the TinyGo
	// transport performs the handshake inline so this is advisory.
	TLSHandshakeTimeout time.Duration

	// MaxResponseHeaderBytes specifies a limit on how many response bytes are
	// allowed in the server's response header. Zero means to use a default limit.
	MaxResponseHeaderBytes int64

	// MaxIdleConns controls the maximum number of idle (keep-alive) connections
	// across all hosts. Zero means no limit.
	MaxIdleConns int

	// MaxIdleConnsPerHost, if non-zero, controls the maximum idle (keep-alive)
	// connections to keep per-host.
	MaxIdleConnsPerHost int

	// MaxConnsPerHost optionally limits the total number of connections per host.
	MaxConnsPerHost int

	// HTTP2 configures HTTP/2 connections. This field does not yet have any effect.
	HTTP2 *HTTP2Config

	// Dial specifies the dial function for creating unencrypted TCP connections.
	//
	// Deprecated: Use DialContext instead. TINYGO: stored only; unused by the
	// fetch-based round tripper.
	Dial func(network, addr string) (net.Conn, error)

	// DialContext specifies the dial function for creating unencrypted TCP
	// connections. TINYGO: honored by callers that dial through the Transport
	// directly (e.g. the relay WebSocket transport), which is how netbird routes
	// connections through the netstack in the browser.
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

var DefaultTransport RoundTripper = &Transport{}

// CloseIdleConnections closes any connections which were previously connected
// from previous requests but are now sitting idle. TINYGO: no-op.
func (t *Transport) CloseIdleConnections() {}

// Clone returns a copy of t with its exported fields shared. TINYGO: the
// tinygo Transport carries only configuration (round trips go through the host
// fetch API), so a shallow struct copy with duplicated maps is sufficient.
func (t *Transport) Clone() *Transport {
	t2 := *t
	if t.TLSClientConfig != nil {
		t2.TLSClientConfig = t.TLSClientConfig.Clone()
	}
	if t.TLSNextProto != nil {
		npm := make(map[string]func(authority string, c *tls.Conn) RoundTripper, len(t.TLSNextProto))
		for k, v := range t.TLSNextProto {
			npm[k] = v
		}
		t2.TLSNextProto = npm
	}
	return &t2
}

// ProxyFromEnvironment returns the URL of the proxy to use for a given request.
// TINYGO: the wasm build has no environment proxy configuration, so this always
// reports "no proxy" (nil URL, nil error), which callers treat as a direct
// connection.
func ProxyFromEnvironment(req *Request) (*url.URL, error) {
	return nil, nil
}

// ProxyURL returns a proxy function (for use in a Transport) that always returns
// the same URL.
func ProxyURL(fixedURL *url.URL) func(*Request) (*url.URL, error) {
	return func(*Request) (*url.URL, error) {
		return fixedURL, nil
	}
}

// ErrSkipAltProtocol is a sentinel error value defined by Transport.RegisterProtocol.
var ErrSkipAltProtocol = errors.New("net/http: skip alternate protocol")

// RegisterProtocol registers a new protocol with scheme. TINYGO: no-op; the
// wasm build performs round trips via the host fetch API and does not dispatch
// on registered alternate protocols.
func (t *Transport) RegisterProtocol(scheme string, rt RoundTripper) {}

// httpTimeoutError represents a timeout.
// It implements net.Error and wraps context.DeadlineExceeded.
type timeoutError struct {
	err string
}

func (e *timeoutError) Error() string { return e.err }

func nop() {}
