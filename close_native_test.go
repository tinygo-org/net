//go:build linux && !baremetal && !tinygo.wasm

package net

import (
	"errors"
	"sync"
	"syscall"
	"testing"
	"time"
)

type closeCountNetdev struct {
	nopNetdev
	calls int
}

func (d *closeCountNetdev) Close(int) error {
	d.calls++
	return nil
}

func TestCloseConcurrent(t *testing.T) {
	previous := netdev
	defer func() { netdev = previous }()
	d := &closeCountNetdev{}
	netdev = d
	c := &TCPConn{fd: 42}
	var wg sync.WaitGroup
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- c.Close() }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	}
	if success != 1 || d.calls != 1 {
		t.Fatalf("success=%d device calls=%d", success, d.calls)
	}
}

func TestCloseDescriptorReuse(t *testing.T) {
	for _, kind := range []string{"listener", "tcp", "udp"} {
		t.Run(kind, func(t *testing.T) {
			fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
			if err != nil {
				t.Fatal(err)
			}
			var closeSocket func() error
			switch kind {
			case "listener":
				closeSocket = (&listener{fd: fd}).Close
			case "tcp":
				closeSocket = (&TCPConn{fd: fd}).Close
			case "udp":
				closeSocket = (&UDPConn{fd: fd}).Close
			}
			if err := closeSocket(); err != nil {
				t.Fatal(err)
			}
			replacement, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
			if err != nil {
				t.Fatal(err)
			}
			if replacement != fd {
				if err := syscall.Dup3(replacement, fd, 0); err != nil {
					syscall.Close(replacement)
					t.Fatal(err)
				}
				syscall.Close(replacement)
			}
			defer syscall.Close(fd)
			err = closeSocket()
			if !errors.Is(err, ErrClosed) {
				t.Errorf("second Close = %v, want ErrClosed", err)
			}
			if _, err := syscall.Getsockname(fd); err != nil {
				t.Fatalf("second Close damaged replacement socket: %v", err)
			}
		})
	}
}

func TestCloseBlockedAccept(t *testing.T) {
	l, err := Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if c != nil {
			c.Close()
		}
		done <- err
	}()
	time.Sleep(25 * time.Millisecond)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Accept succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("Accept did not stop after close")
	}
}

func TestCloseBlockedRead(t *testing.T) {
	pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(pair[1])
	// Every socket this netdev creates is SOCK_NONBLOCK; a hand-made fd must
	// match, or Read blocks in the kernel where the poller cannot wake it.
	if err := syscall.SetNonblock(pair[0], true); err != nil {
		t.Fatal(err)
	}
	c := &TCPConn{fd: pair[0], net: "tcp"}
	done := make(chan error, 1)
	go func() { _, err := c.Read(make([]byte, 1)); done <- err }()
	time.Sleep(25 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Read succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not stop after close")
	}
}

func TestCloseBlockedWrite(t *testing.T) {
	pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(pair[1])
	if err := syscall.SetsockoptInt(pair[0], syscall.SOL_SOCKET, syscall.SO_SNDBUF, 4096); err != nil {
		t.Fatal(err)
	}
	// See TestCloseBlockedRead: the fd must be non-blocking to park on the
	// poller rather than in the kernel.
	if err := syscall.SetNonblock(pair[0], true); err != nil {
		t.Fatal(err)
	}
	c := &TCPConn{fd: pair[0], net: "tcp"}
	done := make(chan error, 1)
	go func() { _, err := c.Write(make([]byte, 1<<20)); done <- err }()
	time.Sleep(25 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Write succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not stop after close")
	}
}
