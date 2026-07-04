// TINYGO tests for the netdev-backed Resolver lookups.

package net

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestResolverLookupHost(t *testing.T) {
	ip := netip.MustParseAddr("1.2.3.4")
	withNetdev(t, &fakeNetdev{hostIP: ip})

	addrs, err := DefaultResolver.LookupHost(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("LookupHost: unexpected error %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "1.2.3.4" {
		t.Fatalf("LookupHost = %v, want [1.2.3.4]", addrs)
	}
}

func TestResolverLookupHostEmpty(t *testing.T) {
	withNetdev(t, &fakeNetdev{})

	_, err := DefaultResolver.LookupHost(context.Background(), "")
	var de *DNSError
	if !errors.As(err, &de) || !de.IsNotFound {
		t.Fatalf("LookupHost(\"\") = %v, want *DNSError with IsNotFound", err)
	}
}

func TestResolverLookupHostError(t *testing.T) {
	withNetdev(t, &fakeNetdev{hostErr: errors.New("dns down")})

	_, err := DefaultResolver.LookupHost(context.Background(), "example.com")
	var de *DNSError
	if !errors.As(err, &de) {
		t.Fatalf("LookupHost error = %T, want *DNSError", err)
	}
	if de.Name != "example.com" {
		t.Errorf("DNSError.Name = %q, want example.com", de.Name)
	}
}

func TestResolverLookupIPAddr(t *testing.T) {
	ip := netip.MustParseAddr("10.0.0.5")
	withNetdev(t, &fakeNetdev{hostIP: ip})

	addrs, err := DefaultResolver.LookupIPAddr(context.Background(), "host")
	if err != nil {
		t.Fatalf("LookupIPAddr: %v", err)
	}
	if len(addrs) != 1 || !addrs[0].IP.Equal(IPv4(10, 0, 0, 5)) {
		t.Fatalf("LookupIPAddr = %v, want [10.0.0.5]", addrs)
	}
}

func TestResolverLookupNetIP(t *testing.T) {
	ip := netip.MustParseAddr("10.0.0.6")
	withNetdev(t, &fakeNetdev{hostIP: ip})

	addrs, err := DefaultResolver.LookupNetIP(context.Background(), "ip", "host")
	if err != nil {
		t.Fatalf("LookupNetIP: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != ip {
		t.Fatalf("LookupNetIP = %v, want [%v]", addrs, ip)
	}
}

func TestLookupIP(t *testing.T) {
	ip := netip.MustParseAddr("192.168.1.1")
	withNetdev(t, &fakeNetdev{hostIP: ip})

	ips, err := LookupIP("host")
	if err != nil {
		t.Fatalf("LookupIP: %v", err)
	}
	if len(ips) != 1 || !ips[0].Equal(IPv4(192, 168, 1, 1)) {
		t.Fatalf("LookupIP = %v, want [192.168.1.1]", ips)
	}
}

func TestLookupHostPackage(t *testing.T) {
	ip := netip.MustParseAddr("172.16.0.9")
	withNetdev(t, &fakeNetdev{hostIP: ip})

	addrs, err := LookupHost("host")
	if err != nil {
		t.Fatalf("LookupHost: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "172.16.0.9" {
		t.Fatalf("LookupHost = %v, want [172.16.0.9]", addrs)
	}
}

func TestDefaultResolverNonNil(t *testing.T) {
	if DefaultResolver == nil {
		t.Fatal("DefaultResolver is nil")
	}
}
