//go:build linux && !baremetal && !tinygo.wasm

package net

import (
	"strconv"
	"syscall"
	"testing"
)

func tcpNoDelay(t *testing.T, c Conn) int {
	t.Helper()
	v, err := syscall.GetsockoptInt(c.(*TCPConn).fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTCPNoDelay(t *testing.T) {
	l, err := Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sa, err := syscall.Getsockname(l.(*listener).fd)
	if err != nil {
		t.Fatal(err)
	}
	client, err := Dial("tcp4", "127.0.0.1:"+strconv.Itoa(sa.(*syscall.SockaddrInet4).Port))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	if tcpNoDelay(t, client) == 0 || tcpNoDelay(t, server) == 0 {
		t.Fatal("TCP_NODELAY is not set by default")
	}
	if err := client.(*TCPConn).SetNoDelay(false); err != nil {
		t.Fatal(err)
	}
	if tcpNoDelay(t, client) != 0 {
		t.Fatal("SetNoDelay(false) did not clear TCP_NODELAY")
	}
}
