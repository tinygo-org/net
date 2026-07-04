// TINYGO tests for ListenUDP and the UDPConn addr-carrying I/O methods.

package net

import (
	"net/netip"
	"testing"
)

func TestListenUDP(t *testing.T) {
	f := &fakeNetdev{socketFD: 7}
	withNetdev(t, f)

	uc, err := ListenUDP("udp", &UDPAddr{IP: IPv4(0, 0, 0, 0), Port: 1234})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	if uc.fd != 7 {
		t.Errorf("fd = %d, want 7", uc.fd)
	}
	if uc.net != "udp" {
		t.Errorf("net = %q, want udp", uc.net)
	}
	if uc.laddr.Port != 1234 {
		t.Errorf("laddr.Port = %d, want 1234", uc.laddr.Port)
	}
	if uc.raddr != nil {
		t.Errorf("raddr = %v, want nil (no connect)", uc.raddr)
	}
	if f.socketArgs != [3]int{_AF_INET, _SOCK_DGRAM, _IPPROTO_UDP} {
		t.Errorf("Socket args = %v, want AF_INET/SOCK_DGRAM/IPPROTO_UDP", f.socketArgs)
	}
	if f.bindAddr.Port() != 1234 {
		t.Errorf("Bind port = %d, want 1234", f.bindAddr.Port())
	}
}

func TestListenUDPBadNetwork(t *testing.T) {
	withNetdev(t, &fakeNetdev{})
	if _, err := ListenUDP("tcp", nil); err == nil {
		t.Fatal("ListenUDP(\"tcp\") = nil error, want error")
	}
}

func TestListenUDPEphemeralPort(t *testing.T) {
	withNetdev(t, &fakeNetdev{socketFD: 3})
	uc, err := ListenUDP("udp", nil)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	if uc.laddr.Port == 0 {
		t.Error("expected an ephemeral port to be assigned, got 0")
	}
}

func TestListenUDPBindErrorClosesSocket(t *testing.T) {
	f := &fakeNetdev{socketFD: 9, bindErr: errBind}
	withNetdev(t, f)

	if _, err := ListenUDP("udp", &UDPAddr{Port: 5}); err != errBind {
		t.Fatalf("ListenUDP err = %v, want %v", err, errBind)
	}
	if len(f.closedFDs) != 1 || f.closedFDs[0] != 9 {
		t.Errorf("closed fds = %v, want [9]", f.closedFDs)
	}
}

var errBind = &DNSError{Err: "bind failed"} // any sentinel error

func TestUDPConnReadFromUDP(t *testing.T) {
	raddr := &UDPAddr{IP: IPv4(5, 6, 7, 8), Port: 99}
	f := &fakeNetdev{recvData: []byte("hello")}
	withNetdev(t, f)

	c := &UDPConn{fd: 1, net: "udp", raddr: raddr}
	buf := make([]byte, 8)
	n, addr, err := c.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("read %q, want hello", buf[:n])
	}
	if addr != raddr {
		t.Errorf("source addr = %v, want %v (connected remote)", addr, raddr)
	}
}

func TestUDPConnReadFromUDPAddrPort(t *testing.T) {
	raddr := &UDPAddr{IP: IPv4(5, 6, 7, 8), Port: 99}
	withNetdev(t, &fakeNetdev{recvData: []byte("hi")})

	c := &UDPConn{fd: 1, net: "udp", raddr: raddr}
	n, ap, err := c.ReadFromUDPAddrPort(make([]byte, 8))
	if err != nil {
		t.Fatalf("ReadFromUDPAddrPort: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}
	if ap != raddr.AddrPort() {
		t.Errorf("addrport = %v, want %v", ap, raddr.AddrPort())
	}
}

func TestUDPConnWriteToUDP(t *testing.T) {
	f := &fakeNetdev{}
	withNetdev(t, f)

	c := &UDPConn{fd: 1, net: "udp"}
	n, err := c.WriteToUDP([]byte("data"), &UDPAddr{IP: IPv4(1, 1, 1, 1), Port: 53})
	if err != nil {
		t.Fatalf("WriteToUDP: %v", err)
	}
	if n != 4 || string(f.sent) != "data" {
		t.Errorf("sent %q (n=%d), want data (n=4)", f.sent, n)
	}
}

func TestUDPConnWriteToUDPAddrPort(t *testing.T) {
	f := &fakeNetdev{}
	withNetdev(t, f)

	c := &UDPConn{fd: 1, net: "udp"}
	ap := netip.AddrPortFrom(netip.MustParseAddr("2.2.2.2"), 53)
	n, err := c.WriteToUDPAddrPort([]byte("xyz"), ap)
	if err != nil {
		t.Fatalf("WriteToUDPAddrPort: %v", err)
	}
	if n != 3 || string(f.sent) != "xyz" {
		t.Errorf("sent %q (n=%d), want xyz (n=3)", f.sent, n)
	}
}

func TestUDPConnSetBuffersNoop(t *testing.T) {
	c := &UDPConn{}
	if err := c.SetReadBuffer(4096); err != nil {
		t.Errorf("SetReadBuffer: %v", err)
	}
	if err := c.SetWriteBuffer(4096); err != nil {
		t.Errorf("SetWriteBuffer: %v", err)
	}
}
