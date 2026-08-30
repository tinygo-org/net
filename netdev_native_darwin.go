//go:build darwin && !baremetal && !nintendoswitch && !wasm_unknown && !tinygo.wasm

package net

import "syscall"

// setSockDefaults applies the socket options that the host netdev wants on
// every socket. SO_REUSEADDR allows a quick rebind of a listening socket, as on
// linux. SO_NOSIGPIPE is necessary because a BSD socket raises SIGPIPE on a
// write to a closed connection and darwin has no MSG_NOSIGNAL.
func setSockDefaults(fd int, stype int) {
	if stype == syscall.SOCK_STREAM {
		syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	}
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_NOSIGPIPE, 1)
}
