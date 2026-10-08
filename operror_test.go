package net

import (
	"os"
	"testing"
)

func TestOpErrorIsNetError(t *testing.T) {
	var err error = &OpError{Op: "read", Net: "udp", Err: os.ErrDeadlineExceeded}
	ne, ok := err.(Error)
	if !ok {
		t.Fatal("*OpError does not implement net.Error")
	}
	if !ne.Timeout() {
		t.Error("Timeout() = false for a deadline error")
	}
	if _, ok := error(&AddrError{Err: "x"}).(Error); !ok {
		t.Error("*AddrError does not implement net.Error")
	}
}
