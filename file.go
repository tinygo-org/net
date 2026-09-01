package net

import (
	"errors"
	"os"
)

// TINYGO: The netdev-based net package manages its own file descriptors and
// cannot adopt an arbitrary OS file descriptor, so the File* constructors are
// not implemented. They exist so that packages referencing them (e.g. gin's
// RunFd helper) compile; callers that actually use them get a clear error.

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
