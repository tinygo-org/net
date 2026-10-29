//go:build linux && !baremetal && !tinygo.wasm

package net

import (
	"context"
	"errors"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// fullBacklogAddr returns the address of a listener whose accept queue is
// full, so Linux drops further SYNs and a connect to it never completes.
func fullBacklogAddr(t *testing.T) string {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Close(fd) })
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Listen(fd, 0); err != nil {
		t.Fatal(err)
	}
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	addr := "127.0.0.1:" + strconv.Itoa(sa.(*syscall.SockaddrInet4).Port)
	for i := 0; i < 8; i++ {
		c, err := DialTimeout("tcp4", addr, 200*time.Millisecond)
		if err != nil {
			return addr
		}
		t.Cleanup(func() { c.Close() })
	}
	t.Skip("accept queue never filled")
	return ""
}

func TestDialTimeoutFullBacklog(t *testing.T) {
	addr := fullBacklogAddr(t)
	start := time.Now()
	_, err := DialTimeout("tcp4", addr, 300*time.Millisecond)
	var ne Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("got %v, want a timeout", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("timed out after %v, want about 300ms", d)
	}
}

func TestDialContextCancel(t *testing.T) {
	addr := fullBacklogAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	var d Dialer
	start := time.Now()
	_, err := d.DialContext(ctx, "tcp4", addr)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("canceled after %v, want about 200ms", d)
	}
}
