//go:build linux && !baremetal && !tinygo.wasm

package net

import (
	"syscall"
	"testing"
	"time"
)

// A read that took its deadline before a concurrent SetReadDeadline must still
// wake at the new deadline.
func TestParkedReadSeesNewDeadline(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fds[1])
	c := &TCPConn{fd: fds[0], net: "tcp"}
	defer c.Close()

	old := time.Now().Add(5 * time.Second)
	c.SetReadDeadline(old)
	first := make(chan error, 1)
	go func() {
		_, err := c.Read(make([]byte, 1))
		first <- err
	}()
	time.Sleep(50 * time.Millisecond)

	c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	start := time.Now()
	_, err = netdev.Recv(c.fd, make([]byte, 1), 0, old)
	if err == nil {
		t.Fatal("read returned data")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("late read waited %v for the old deadline", d)
	}
	<-first
}
