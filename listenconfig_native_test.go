//go:build linux && !baremetal && !tinygo.wasm

package net

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func TestListenConfigTCPExchange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l, err := (&ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	cancel()
	sa, err := syscall.Getsockname(l.(*listener).fd)
	if err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if err := syscall.Connect(fd, sa); err != nil {
		t.Fatal(err)
	}
	c, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := syscall.Write(fd, []byte("x")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 1)
	if n, err := c.Read(b); n != 1 || err != nil || b[0] != 'x' {
		t.Fatalf("read=%d %q error=%v", n, b, err)
	}
}

func TestListenConfigUDPReceive(t *testing.T) {
	c, err := (&ListenConfig{}).ListenPacket(context.Background(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(time.Second))
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	addr := c.LocalAddr().(*UDPAddr)
	sa := &syscall.SockaddrInet4{Port: addr.Port, Addr: [4]byte{127, 0, 0, 1}}
	if err := syscall.Sendto(fd, []byte("x"), 0, sa); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 1)
	if n, err := c.(*UDPConn).Read(b); n != 1 || err != nil || b[0] != 'x' {
		t.Fatalf("read=%d %q error=%v", n, b, err)
	}
}
