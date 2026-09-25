//go:build darwin && !baremetal && !tinygo.wasm

package net

import (
	"syscall"
	"testing"
)

func TestEphemeralAcceptedNoSigpipe(t *testing.T) {
	client, server := ephemeralTestPair(t)
	defer client.Close()
	defer server.Close()
	value, err := syscall.GetsockoptInt(server.(*TCPConn).fd, syscall.SOL_SOCKET, syscall.SO_NOSIGPIPE)
	if err != nil || value != 1 {
		t.Fatalf("accepted SO_NOSIGPIPE=%d error=%v", value, err)
	}
}
