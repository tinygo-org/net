// Copyright 2026 The TinyGo Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// hangListener accepts a connection and never speaks HTTP, matching the
// delayed-server case in tinygo-org/net#86.
func hangListener(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	done := make(chan struct{})
	go func() {
		defer ln.Close()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 1)
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = c.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			if _, err := c.Read(buf); err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
		}
	}()
	return ln.Addr().String(), func() { close(done); ln.Close() }
}

func TestClientTimeoutStopsHungRequest(t *testing.T) {
	addr, closeFn := hangListener(t)
	defer closeFn()

	client := &Client{Timeout: 200 * time.Millisecond}
	start := time.Now()
	resp, err := client.Get("http://" + addr + "/")
	elapsed := time.Since(start)
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Client.Timeout did not stop the hung request: elapsed=%v err=%v", elapsed, err)
	}
}

func TestRequestContextCancelStopsHungRequest(t *testing.T) {
	addr, closeFn := hangListener(t)
	defer closeFn()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, err := NewRequestWithContext(ctx, "GET", "http://"+addr+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	resp, err := DefaultClient.Do(req)
	elapsed := time.Since(start)
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("request context did not stop the hung request: elapsed=%v err=%v", elapsed, err)
	}
}
