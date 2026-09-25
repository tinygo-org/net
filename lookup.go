// TINYGO: The following is copied and modified from Go 1.26.2 official implementation.

// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"context"
	"errors"
	"net/netip"
)

// A Resolver looks up names and numbers.
//
// A nil *Resolver is equivalent to a zero Resolver.
//
// TINYGO: name resolution is backed by the netdev GetHostByName hook;
// the PreferGo/StrictErrors/Dial fields exist for API compatibility with
// upstream Go and are ignored.
type Resolver struct {
	// PreferGo controls whether Go's built-in DNS resolver is preferred
	// on platforms where it's available. It is equivalent to setting
	// GODEBUG=netdns=go, but scoped to just this resolver.
	PreferGo bool

	// StrictErrors controls the behavior of temporary errors
	// (including timeout, socket errors, and SERVFAIL) when using
	// Go's built-in resolver.
	StrictErrors bool

	// Dial optionally specifies an alternate dialer for use by
	// Go's built-in DNS resolver to make TCP and UDP connections
	// to DNS services.
	Dial func(ctx context.Context, network, address string) (Conn, error)
}

// DefaultResolver is the resolver used by the package-level Lookup
// functions and by Dialers without a specified Resolver.
var DefaultResolver = &Resolver{}

// LookupHost looks up the given host using the local resolver.
// It returns a slice of that host's addresses.
//
// LookupHost uses [context.Background] internally; to specify the context, use
// [Resolver.LookupHost].
func LookupHost(host string) (addrs []string, err error) {
	return DefaultResolver.LookupHost(context.Background(), host)
}

// LookupHost looks up the given host using the local resolver.
// It returns a slice of that host's addresses.
//
// TINYGO: resolves via netdev.GetHostByName, returning at most one address.
func (r *Resolver) LookupHost(ctx context.Context, host string) (addrs []string, err error) {
	if host == "" {
		return nil, &DNSError{Err: errNoSuchHost.Error(), Name: host, IsNotFound: true}
	}
	ip, err := netdev.GetHostByName(host)
	if err != nil {
		return nil, newLookupError(host, err)
	}
	return []string{ip.String()}, nil
}

// LookupIP looks up host using the local resolver.
// It returns a slice of that host's IPv4 and IPv6 addresses.
func LookupIP(host string) ([]IP, error) {
	addrs, err := DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return nil, err
	}
	ips := make([]IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

// LookupIPAddr looks up host using the local resolver.
// It returns a slice of that host's IPv4 and IPv6 addresses.
//
// TINYGO: resolves via netdev.GetHostByName, returning at most one address.
func (r *Resolver) LookupIPAddr(ctx context.Context, host string) ([]IPAddr, error) {
	if host == "" {
		return nil, &DNSError{Err: errNoSuchHost.Error(), Name: host, IsNotFound: true}
	}
	ip, err := netdev.GetHostByName(host)
	if err != nil {
		return nil, newLookupError(host, err)
	}
	return []IPAddr{{IP: ip.AsSlice(), Zone: ip.Zone()}}, nil
}

// LookupNetIP looks up host using the local resolver.
// It returns a slice of that host's IP addresses of the type specified by
// network. The network must be one of "ip", "ip4" or "ip6".
//
// TINYGO: resolves via netdev.GetHostByName, returning at most one address.
func (r *Resolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	if host == "" {
		return nil, &DNSError{Err: errNoSuchHost.Error(), Name: host, IsNotFound: true}
	}
	ip, err := netdev.GetHostByName(host)
	if err != nil {
		return nil, newLookupError(host, err)
	}
	return []netip.Addr{ip}, nil
}

// LookupPort looks up the port for the given network and service.
//
// LookupPort uses [context.Background] internally; to specify the context, use
// [Resolver.LookupPort].
func LookupPort(network, service string) (port int, err error) {
	return DefaultResolver.LookupPort(context.Background(), network, service)
}

// LookupPort looks up the port for the given network and service.
//
// TINYGO: not implemented; netdev provides no service database.
func (r *Resolver) LookupPort(ctx context.Context, network, service string) (port int, err error) {
	return 0, errors.New("net:LookupPort not implemented")
}

// An SRV represents a single DNS SRV record.
type SRV struct {
	Target   string
	Port     uint16
	Priority uint16
	Weight   uint16
}

// LookupSRV tries to resolve an SRV query of the given service, protocol, and
// domain name.
//
// TINYGO: not implemented; netdev provides no SRV record lookup.
func (r *Resolver) LookupSRV(ctx context.Context, service, proto, name string) (string, []*SRV, error) {
	return "", nil, errors.New("net:LookupSRV not implemented")
}

// LookupTXT returns the DNS TXT records for the given domain name.
//
// TINYGO: not implemented; netdev provides no TXT record lookup.
func (r *Resolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return nil, errors.New("net:LookupTXT not implemented")
}

// errNoSuchHost is returned when the host lookup finds no matching records.
var errNoSuchHost = errors.New("no such host")

// newLookupError wraps a netdev resolver error in a DNSError for host.
//
// TINYGO: the netdev can report a DNSError of its own, and callers read
// IsNotFound and IsTimeout on the outer error, so carry those flags out.
func newLookupError(host string, err error) *DNSError {
	dnsErr := &DNSError{Err: err.Error(), Name: host, UnwrapErr: err}
	if inner, ok := err.(*DNSError); ok {
		// Take the inner description and not its Error(), which repeats the
		// "lookup <host>" prefix that this error adds.
		dnsErr.Err = inner.Err
		dnsErr.IsNotFound = inner.IsNotFound
		dnsErr.IsTimeout = inner.IsTimeout
		dnsErr.IsTemporary = inner.IsTemporary
		if inner.Server != "" {
			dnsErr.Server = inner.Server
		}
	}
	return dnsErr
}
