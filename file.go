package net

import (
	"errors"
	"os"
)

// TINYGO: this package manages its own file descriptors and cannot adopt an
// arbitrary OS fd, so these exist only to let callers compile.

var errFileNotImplemented = errors.New("net: File-based listeners/conns are not implemented on this target")

// FileListener returns a copy of the network listener corresponding to the open
// file f.
func FileListener(f *os.File) (ln Listener, err error) {
	return nil, errFileNotImplemented
}

// FileConn returns a copy of the network connection corresponding to the open
// file f.
func FileConn(f *os.File) (c Conn, err error) {
	return nil, errFileNotImplemented
}

// File returns a copy of the underlying os.File.
//
// TINYGO: a listener here has no OS file descriptor to hand back.
func (l *TCPListener) File() (f *os.File, err error) {
	return nil, errFileNotImplemented
}
