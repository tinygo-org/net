//go:build linux && !baremetal && !tinygo.wasm

package net

import (
	"strconv"
	"syscall"
	"testing"
)

func TestDialLocalSystem(t *testing.T) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Listen(fd, 1); err != nil {
		t.Fatal(err)
	}
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	port := sa.(*syscall.SockaddrInet4).Port

	for _, addr := range []string{":" + strconv.Itoa(port), "0.0.0.0:" + strconv.Itoa(port)} {
		c, err := Dial("tcp", addr)
		if err != nil {
			t.Errorf("Dial(tcp, %q): %v", addr, err)
			continue
		}
		c.Close()
	}
	u, err := DialUDP("udp", nil, &UDPAddr{Port: port})
	if err != nil {
		t.Errorf("DialUDP to port %d: %v", port, err)
	} else {
		u.Close()
	}
}
