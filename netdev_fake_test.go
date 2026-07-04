// TINYGO test helper: an in-memory fake netdever for exercising the
// netdev-backed net APIs without real hardware/OS sockets.

package net

import (
	"net/netip"
	"testing"
	"time"
)

// fakeNetdev embeds nopNetdev (whose methods all error) and overrides only
// the calls a given test needs. Recorded fields let tests assert on how the
// net package drove the device.
type fakeNetdev struct {
	nopNetdev

	// GetHostByName control.
	hostIP  netip.Addr
	hostErr error

	// Socket/Bind/Connect control + capture.
	socketFD   int
	socketErr  error
	socketArgs [3]int
	bindErr    error
	bindAddr   netip.AddrPort
	connectErr error

	// Send/Recv control + capture.
	sent     []byte // accumulates everything written via Send
	sendErr  error
	recvData []byte // returned (once) by Recv
	recvErr  error

	closedFDs []int
}

func (f *fakeNetdev) GetHostByName(name string) (netip.Addr, error) {
	return f.hostIP, f.hostErr
}

func (f *fakeNetdev) Socket(domain, stype, protocol int) (int, error) {
	f.socketArgs = [3]int{domain, stype, protocol}
	return f.socketFD, f.socketErr
}

func (f *fakeNetdev) Bind(sockfd int, ip netip.AddrPort) error {
	f.bindAddr = ip
	return f.bindErr
}

func (f *fakeNetdev) Connect(sockfd int, host string, ip netip.AddrPort) error {
	return f.connectErr
}

func (f *fakeNetdev) Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	if f.sendErr != nil {
		return -1, f.sendErr
	}
	f.sent = append(f.sent, buf...)
	return len(buf), nil
}

func (f *fakeNetdev) Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	if f.recvErr != nil {
		return -1, f.recvErr
	}
	n := copy(buf, f.recvData)
	f.recvData = f.recvData[n:]
	return n, nil
}

func (f *fakeNetdev) Close(sockfd int) error {
	f.closedFDs = append(f.closedFDs, sockfd)
	return nil
}

// withNetdev installs d as the package netdev for the duration of the test.
func withNetdev(t *testing.T, d netdever) {
	t.Helper()
	old := netdev
	netdev = d
	t.Cleanup(func() { netdev = old })
}
