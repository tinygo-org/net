// L3/L4 network/transport layer

package net

import (
	"net/netip"
	"time"
)

const (
	_AF_INET       = 0x2
	_SOCK_STREAM   = 0x1
	_SOCK_DGRAM    = 0x2
	_SOL_SOCKET    = 0x1
	_SO_KEEPALIVE  = 0x9
	_SOL_TCP       = 0x6
	_TCP_KEEPINTVL = 0x5
	_IPPROTO_TCP   = 0x6
	_IPPROTO_UDP   = 0x11
	// Made up, not a real IP protocol number.  This is used to create a
	// TLS socket on the device, assuming the device supports mbed TLS.
	_IPPROTO_TLS = 0xFE
	_F_SETFL     = 0x4
)

// netdev is the current netdev, set by the application with useNetdev()
var netdev netdever

// (useNetdev is go:linkname'd from tinygo/drivers package)
func useNetdev(dev netdever) {
	netdev = dev
}

// netdever is TinyGo's OSI L3/L4 network/transport layer interface.  Network
// drivers implement the netdever interface, providing a common network L3/L4
// interface to TinyGo's "net" package.  net.Conn implementations (TCPConn,
// UDPConn, and TLSConn) use the netdever interface for device I/O access.
//
// A netdever is passed to the "net" package using net.useNetdev().
//
// Just like a net.Conn, multiple goroutines may invoke methods on a netdever
// simultaneously.
//
// NOTE: The netdever interface is mirrored in drivers/netdev/netdev.go.
// NOTE: If making changes to this interface, mirror the changes in
// NOTE: drivers/netdev/netdev.go, and vice-versa.

type netdever interface {

	// GetHostByName returns the IP address of either a hostname or IPv4
	// address in standard dot notation
	GetHostByName(name string) (netip.Addr, error)

	// Addr returns IP address assigned to the interface, either by
	// DHCP or statically
	Addr() (netip.Addr, error)

	// Berkely Sockets-like interface, Go-ified.  See man page for socket(2), etc.
	Socket(domain int, stype int, protocol int) (int, error)
	Bind(sockfd int, ip netip.AddrPort) error
	Connect(sockfd int, host string, ip netip.AddrPort) error
	Listen(sockfd int, backlog int) error
	Accept(sockfd int, ip netip.AddrPort) (int, error)
	Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Close(sockfd int) error
	SetSockOpt(sockfd int, level int, opt int, value interface{}) error
}
