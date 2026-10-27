//go:build linux && !baremetal && !nintendoswitch && !wasm_unknown && !tinygo.wasm && !scheduler.threads

package net

import "time"

// Other schedulers run every goroutine on one thread, so a blocking epoll_wait
// would stop the program. The poller checks without blocking and sleeps instead.
const (
	pollWaitMsec = 0
	pollIdle     = time.Millisecond
)
