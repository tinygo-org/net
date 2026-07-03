// TINYGO: The following is copied and modified from Go 1.26.2 official implementation.

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"errors"
	"time"
)

// BUG(mikio): On JS, WASIP1 and Plan 9, methods and functions related
// to UnixConn and UnixListener are not implemented.

// BUG(mikio): On Windows, methods and functions related to UnixConn
// and UnixListener don't work for "unixgram" and "unixpacket".

// BUG(paralin): On TinyGo, Unix sockets are not implemented.

// UnixAddr represents the address of a Unix domain socket end point.
type UnixAddr struct {
	Name string
	Net  string
}

// Network returns the address's network name, "unix", "unixgram" or
// "unixpacket".
func (a *UnixAddr) Network() string {
	return a.Net
}

func (a *UnixAddr) String() string {
	if a == nil {
		return "<nil>"
	}
	return a.Name
}

func (a *UnixAddr) isWildcard() bool {
	return a == nil || a.Name == ""
}

func (a *UnixAddr) opAddr() Addr {
	if a == nil {
		return nil
	}
	return a
}

// UnixConn is an implementation of the Conn interface for connections to Unix
// domain sockets.
//
// TINYGO: Unix sockets are not implemented (the browser has no filesystem
// sockets). This type exists only to satisfy references and type assertions
// such as those in github.com/gliderlabs/ssh agent forwarding; all methods
// return an error. The corresponding code paths are never reached at runtime in
// the wasm build.
type UnixConn struct {
	fd    int
	laddr *UnixAddr
	raddr *UnixAddr
}

var errUnixNotImplemented = errors.New("net: Unix sockets not implemented")

func (c *UnixConn) Read(b []byte) (int, error)  { return 0, errUnixNotImplemented }
func (c *UnixConn) Write(b []byte) (int, error) { return 0, errUnixNotImplemented }
func (c *UnixConn) Close() error                { return errUnixNotImplemented }
func (c *UnixConn) CloseRead() error            { return errUnixNotImplemented }
func (c *UnixConn) CloseWrite() error           { return errUnixNotImplemented }
func (c *UnixConn) LocalAddr() Addr             { return c.laddr }
func (c *UnixConn) RemoteAddr() Addr            { return c.raddr }
func (c *UnixConn) SetDeadline(t time.Time) error      { return errUnixNotImplemented }
func (c *UnixConn) SetReadDeadline(t time.Time) error  { return errUnixNotImplemented }
func (c *UnixConn) SetWriteDeadline(t time.Time) error { return errUnixNotImplemented }

// ResolveUnixAddr returns an address of Unix domain socket end point.
//
// The network must be a Unix network name.
//
// See func [Dial] for a description of the network and address
// parameters.
func ResolveUnixAddr(network, address string) (*UnixAddr, error) {
	switch network {
	case "unix", "unixgram", "unixpacket":
		return &UnixAddr{Name: address, Net: network}, nil
	default:
		return nil, UnknownNetworkError(network)
	}
}
