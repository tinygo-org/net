//go:build linux && !baremetal && !tinygo.wasm

package net

import (
	"errors"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestWriteToClosedPeer(t *testing.T) {
	addr := "127.0.0.1:" + strconv.Itoa(40000+os.Getpid()%20000)
	ln, err := Listen("tcp", addr)
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
	}()
	c, err := Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	time.Sleep(100 * time.Millisecond)
	buf := make([]byte, 1024)
	for i := 0; i < 100; i++ {
		if _, err = c.Write(buf); err != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !errors.Is(err, syscall.EPIPE) && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("Write to a closed peer = %v, want EPIPE or ECONNRESET", err)
	}
}
