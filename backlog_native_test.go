//go:build linux && !baremetal && !tinygo.wasm && (amd64 || arm64)

package net

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestListenBacklog(t *testing.T) {
	l, err := Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	// On a listening socket Linux reports the accept queue limit in tcpi_sacked.
	var info syscall.TCPInfo
	size := uint32(unsafe.Sizeof(info))
	_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, uintptr(l.(*listener).fd),
		syscall.SOL_TCP, syscall.TCP_INFO, uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&size)), 0)
	if errno != 0 {
		t.Fatal(errno)
	}
	if info.Sacked <= 5 {
		t.Fatalf("listen backlog %d, want the system limit", info.Sacked)
	}
}
