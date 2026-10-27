//go:build linux && !baremetal && !nintendoswitch && !wasm_unknown && !tinygo.wasm

// TINYGO: a small epoll poller for the host netdev. A goroutine that would
// block parks here until the fd is ready, its deadline passes, or it closes.

package net

import (
	"errors"
	"sync"
	"syscall"
	"time"
)

var errPollClosed = errors.New("net: use of closed network connection")

type pollDesc struct {
	readers []chan error
	writers []chan error
	inEpoll bool

	// An interrupt that found no parked waiter, so the next wait returns at once.
	readInterrupt  bool
	writeInterrupt bool

	// Calls in progress on the fd. Close defers close(2) until this drops to 0,
	// so the fd number cannot be reused while a call may still wait on it.
	refs    int
	closing bool
}

type netPoller struct {
	mu      sync.Mutex
	started bool
	err     error
	epfd    int
	fds     map[int]*pollDesc
}

var poller netPoller

// start creates the epoll instance on first use. Must be called with p.mu held.
func (p *netPoller) start() error {
	if !p.started {
		p.started = true
		p.epfd, p.err = syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
		if p.err == nil {
			go p.loop(p.epfd)
		}
	}
	return p.err
}

// acquire registers a call on fd. It fails once Close has run.
func (p *netPoller) acquire(fd int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fds == nil {
		p.fds = make(map[int]*pollDesc)
	}
	pd := p.fds[fd]
	if pd == nil {
		pd = &pollDesc{}
		p.fds[fd] = pd
	}
	if pd.closing {
		return errPollClosed
	}
	pd.refs++
	return nil
}

// release ends a call started with acquire, and closes fd if it was the last
// call after Close.
func (p *netPoller) release(fd int) {
	p.mu.Lock()
	pd := p.fds[fd]
	pd.refs--
	if pd.refs > 0 {
		p.mu.Unlock()
		return
	}
	closing := pd.closing
	delete(p.fds, fd)
	p.mu.Unlock()
	if closing {
		syscall.Close(fd)
	}
}

// close wakes every waiter on fd with errPollClosed and closes fd, at once if
// no call is in progress and otherwise when the last one releases it.
func (p *netPoller) close(fd int) error {
	p.mu.Lock()
	pd := p.fds[fd]
	if pd == nil {
		p.mu.Unlock()
		return syscall.Close(fd)
	}
	pd.closing = true
	if pd.inEpoll {
		syscall.EpollCtl(p.epfd, syscall.EPOLL_CTL_DEL, fd, nil)
		pd.inEpoll = false
	}
	chs := append(pd.readers, pd.writers...)
	pd.readers, pd.writers = nil, nil
	p.mu.Unlock()
	for _, ch := range chs {
		ch <- errPollClosed
	}
	return nil
}

func (pd *pollDesc) events() uint32 {
	var ev uint32
	if len(pd.readers) != 0 {
		ev |= syscall.EPOLLIN | syscall.EPOLLRDHUP
	}
	if len(pd.writers) != 0 {
		ev |= syscall.EPOLLOUT
	}
	if ev != 0 {
		ev |= syscall.EPOLLONESHOT
	}
	return ev
}

// arm programs epoll for the waiters on fd. If that fails, it wakes them with
// the error. Must be called with p.mu held.
func (p *netPoller) arm(fd int, pd *pollDesc) {
	ev := pd.events()
	if ev == 0 {
		if pd.inEpoll {
			syscall.EpollCtl(p.epfd, syscall.EPOLL_CTL_DEL, fd, nil)
			pd.inEpoll = false
		}
		return
	}
	op := syscall.EPOLL_CTL_ADD
	if pd.inEpoll {
		op = syscall.EPOLL_CTL_MOD
	}
	err := syscall.EpollCtl(p.epfd, op, fd, &syscall.EpollEvent{Events: ev, Fd: int32(fd)})
	if err == nil {
		pd.inEpoll = true
		return
	}
	chs := append(pd.readers, pd.writers...)
	pd.readers, pd.writers = nil, nil
	for _, ch := range chs {
		ch <- err
	}
}

// wait blocks until fd is ready for writing (write) or reading, the deadline
// passes, or fd is closed. The caller must hold a reference from acquire.
func (p *netPoller) wait(fd int, write bool, deadline time.Time) error {
	ch := make(chan error, 1)

	p.mu.Lock()
	if err := p.start(); err != nil {
		p.mu.Unlock()
		return err
	}
	pd := p.fds[fd]
	if pd.closing {
		p.mu.Unlock()
		return errPollClosed
	}
	if write {
		if pd.writeInterrupt {
			pd.writeInterrupt = false
			p.mu.Unlock()
			return errPollInterrupted
		}
		pd.writers = append(pd.writers, ch)
	} else {
		if pd.readInterrupt {
			pd.readInterrupt = false
			p.mu.Unlock()
			return errPollInterrupted
		}
		pd.readers = append(pd.readers, ch)
	}
	p.arm(fd, pd)
	p.mu.Unlock()

	var timeout <-chan time.Time
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d <= 0 {
			return p.cancelWaiter(fd, write, ch)
		}
		t := time.NewTimer(d)
		defer t.Stop()
		timeout = t.C
	}

	select {
	case err := <-ch:
		return err
	case <-timeout:
		return p.cancelWaiter(fd, write, ch)
	}
}

// cancelWaiter removes a waiter whose deadline passed. If it was woken in the
// meantime, that result wins over the timeout.
func (p *netPoller) cancelWaiter(fd int, write bool, ch chan error) error {
	p.mu.Lock()
	pd := p.fds[fd]
	if write {
		pd.writers = removeChan(pd.writers, ch)
	} else {
		pd.readers = removeChan(pd.readers, ch)
	}
	p.arm(fd, pd)
	p.mu.Unlock()
	select {
	case err := <-ch:
		return err
	default:
		return timeoutError{}
	}
}

// interrupt wakes the goroutines parked on fd in one direction so they pick up
// a new deadline. net/http's abortPendingRead relies on this.
func (p *netPoller) interrupt(fd int, write bool) {
	p.mu.Lock()
	pd := p.fds[fd]
	if pd == nil || pd.refs == 0 {
		p.mu.Unlock()
		return
	}
	var chs []chan error
	if write {
		chs, pd.writers = pd.writers, nil
		pd.writeInterrupt = len(chs) == 0
	} else {
		chs, pd.readers = pd.readers, nil
		pd.readInterrupt = len(chs) == 0
	}
	p.arm(fd, pd)
	p.mu.Unlock()
	for _, ch := range chs {
		ch <- errPollInterrupted
	}
}

func (p *netPoller) loop(epfd int) {
	events := make([]syscall.EpollEvent, 64)
	for {
		n, err := syscall.EpollWait(epfd, events, pollWaitMsec)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return
		}
		if n == 0 {
			time.Sleep(pollIdle)
			continue
		}
		p.mu.Lock()
		for _, e := range events[:n] {
			pd := p.fds[int(e.Fd)]
			if pd == nil {
				continue
			}
			// On an error or hangup, wake everyone so they see the real result.
			hup := e.Events&(syscall.EPOLLERR|syscall.EPOLLHUP|syscall.EPOLLRDHUP) != 0
			var chs []chan error
			if hup || e.Events&syscall.EPOLLIN != 0 {
				chs, pd.readers = append(chs, pd.readers...), nil
			}
			if hup || e.Events&syscall.EPOLLOUT != 0 {
				chs, pd.writers = append(chs, pd.writers...), nil
			}
			for _, ch := range chs {
				ch <- nil
			}
			p.arm(int(e.Fd), pd)
		}
		p.mu.Unlock()
	}
}

func removeChan(s []chan error, ch chan error) []chan error {
	for i, c := range s {
		if c == ch {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}
