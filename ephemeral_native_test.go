//go:build (linux || darwin) && !baremetal && !tinygo.wasm

package net

import (
	"net/netip"
	"testing"
	"time"
)

func ephemeralTestPair(t *testing.T) (Conn, Conn) {
	t.Helper()
	l, err := Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if l.Addr().(*TCPAddr).Port == 0 {
		t.Fatalf("listener reports port 0: %v", l.Addr())
	}
	client, err := Dial("tcp4", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := l.Accept()
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	return client, server
}

func TestEphemeralListenDial(t *testing.T) {
	client, server := ephemeralTestPair(t)
	defer client.Close()
	defer server.Close()
	server.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 1)
	if n, err := server.Read(b); n != 1 || err != nil || b[0] != 'x' {
		t.Fatalf("read=%d %q error=%v", n, b, err)
	}
}

// ephemeralTestNetdev lets listenTCP succeed without a kernel socket, and
// reports a fixed bound address so the applied port and zone are predictable.
type ephemeralTestNetdev struct {
	nopNetdev
}

func (*ephemeralTestNetdev) Socket(int, int, int) (int, error) { return 42, nil }
func (*ephemeralTestNetdev) Bind(int, netip.AddrPort) error    { return nil }
func (*ephemeralTestNetdev) Listen(int, int) error             { return nil }
func (*ephemeralTestNetdev) Close(int) error                   { return nil }
func (*ephemeralTestNetdev) GetSockname(int) (netip.AddrPort, error) {
	return netip.MustParseAddrPort("[fe80::1]:12345"), nil
}

func TestEphemeralPreservesZone(t *testing.T) {
	previous := netdev
	defer func() { netdev = previous }()
	netdev = &ephemeralTestNetdev{}
	addr := &TCPAddr{IP: ParseIP("fe80::1"), Zone: "test-zone"}
	l, err := listenTCP(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := l.Addr().(*TCPAddr)
	if got.Port != 12345 || got.Zone != addr.Zone || !got.IP.Equal(addr.IP) {
		t.Fatalf("bound address=%v, want [%s%%%s]:12345", got, addr.IP, addr.Zone)
	}
	if addr.Port != 0 {
		t.Fatalf("input address changed: %v", addr)
	}
}
