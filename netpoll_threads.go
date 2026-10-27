//go:build linux && !baremetal && !nintendoswitch && !wasm_unknown && !tinygo.wasm && scheduler.threads

package net

import "time"

// The poller goroutine has its own thread, so it can block in epoll_wait.
const (
	pollWaitMsec = -1
	pollIdle     = time.Duration(0)
)
