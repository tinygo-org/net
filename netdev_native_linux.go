//go:build linux && !baremetal && !nintendoswitch && !wasm_unknown && !tinygo.wasm

package net

import "syscall"

// setSockDefaults applies the socket options that the host netdev wants on
// every socket. On linux that is only SO_REUSEADDR on a stream socket, which
// allows a quick rebind of a listening socket. This is the same set of options
// that the netdev applied before the darwin split.
func setSockDefaults(fd int, stype int) {
	if stype == syscall.SOCK_STREAM {
		syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	}
}
