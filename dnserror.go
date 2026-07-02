// TINYGO: The following is copied and modified from Go 1.26.2 official implementation.

// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

// DNSError represents a DNS lookup error.
type DNSError struct {
	UnwrapErr   error  // error returned by the [DNSError.Unwrap] method, might be nil
	Err         string // description of the error
	Name        string // name looked for
	Server      string // server used
	IsTimeout   bool   // if true, timed out; not all timeouts set this
	IsTemporary bool   // if true, error is temporary; not all errors set this

	// IsNotFound is set to true when the requested name does not
	// contain any records of the requested type (data not found),
	// or the name itself was not found (NXDOMAIN).
	IsNotFound bool
}

// Unwrap returns e.UnwrapErr.
func (e *DNSError) Unwrap() error { return e.UnwrapErr }

func (e *DNSError) Error() string {
	if e == nil {
		return "<nil>"
	}
	s := "lookup " + e.Name
	if e.Server != "" {
		s += " on " + e.Server
	}
	s += ": " + e.Err
	return s
}

// Timeout reports whether the DNS lookup is known to have timed out.
// This is not always known; a DNS lookup may fail due to a timeout
// and return a [DNSError] for which Timeout returns false.
func (e *DNSError) Timeout() bool { return e.IsTimeout }

// Temporary reports whether the DNS error is known to be temporary.
// This is not always known; a DNS lookup may fail due to a temporary
// error and return a [DNSError] for which Temporary returns false.
func (e *DNSError) Temporary() bool { return e.IsTimeout || e.IsTemporary }
