// L3/L4 network/transport layer

package net

import (
	"errors"
	"net/netip"
	"time"
)

const (
	_AF_INET       = 0x2
	_SOCK_STREAM   = 0x1
	_SOCK_DGRAM    = 0x2
	_SOL_SOCKET    = 0x1
	_SO_KEEPALIVE  = 0x9
	_SO_LINGER     = 0xd
	_SOL_TCP       = 0x6
	_TCP_KEEPINTVL = 0x5
	_IPPROTO_TCP   = 0x6
	_IPPROTO_UDP   = 0x11
	// Made up, not a real IP protocol number.  This is used to create a
	// TLS socket on the device, assuming the device supports mbed TLS.
	_IPPROTO_TLS = 0xFE
	_F_SETFL     = 0x4
)

// netdev is the current netdev, set by the application with useNetdev().
//
// Initialized to a NOP netdev that errors out cleanly in case netdev was not
// explicitly set with useNetdev().
var netdev netdever = &nopNetdev{}

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

	// # Socket Address family/domain argument
	//
	// Socket address families specifies a communication domain:
	//  - AF_UNIX, AF_LOCAL(synonyms): Local communication For further information, see unix(7).
	//  - AF_INET: IPv4 Internet protocols.  For further information, see ip(7).
	//
	// # Socket type argument
	//
	// Socket types which specifies the communication semantics.
	//  - SOCK_STREAM: Provides sequenced, reliable, two-way, connection-based
	//  byte streams.  An out-of-band data transmission mechanism may be supported.
	//  - SOCK_DGRAM: Supports datagrams (connectionless, unreliable messages of
	//  a fixed maximum length).
	//
	// The type argument serves a second purpose: in addition to specifying a
	// socket type, it may include the bitwise OR of any of the following values,
	// to modify the behavior of socket():
	//  - SOCK_NONBLOCK: Set the O_NONBLOCK file status flag on the open file description.
	//
	// # Socket protocol argument
	//
	// The protocol specifies a particular protocol to be used with the
	// socket.  Normally only a single protocol exists to support a
	// particular socket type within a given protocol family, in which
	// case protocol can be specified as 0. However, it is possible
	// that many protocols may exist, in which case a particular
	// protocol must be specified in this manner.
	//
	// # Return value
	//
	// On success, a file descriptor for the new socket is returned. Quoting man pages:
	// "On error, -1 is returned, and errno is set to indicate the error." Since
	// this is not C we may use a error type native to Go to represent the error
	// ocurred which by itself not only notifies of an error but also provides
	// information on the error as a human readable string when calling the Error method.
	Socket(domain int, stype int, protocol int) (sockfd int, _ error)
	Bind(sockfd int, ip netip.AddrPort) error
	Connect(sockfd int, host string, ip netip.AddrPort) error
	Listen(sockfd int, backlog int) error
	Accept(sockfd int) (int, netip.AddrPort, error)

	// # Flags argument on Send and Recv
	//
	// The flags argument is formed by ORing one or more of the following values:
	//  - MSG_CMSG_CLOEXEC: Set the close-on-exec flag for the file descriptor. Unix.
	//  - MSG_DONTWAIT: Enables nonblocking operation. If call would block then returns error.
	//  - MSG_ERRQUEUE: (see manpage) his flag specifies that queued errors should be received
	//  from the socket error queue.
	//  - MSG_OOB: his flag requests receipt of out-of-band data that would not be received in the normal data stream.
	//  - MSG_PEEK: This flag causes the receive operation to return data from
	//  the beginning of the receive queue without removing that data from the queue.
	//  - MSG_TRUNC: Ask for real length of datagram even when it was longer than passed buffer.
	//  - MSG_WAITALL: This flag requests that the operation block until the full request is satisfied.

	Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Close(sockfd int) error

	// SetSockOpt manipulates options for the socket
	// referred to by the file descriptor sockfd.  Options may exist at
	// multiple protocol levels; they are always present at the
	// uppermost socket level.
	//
	// # Level argument
	//
	// When manipulating socket options, the level at which the option
	// resides and the name of the option must be specified.  To
	// manipulate options at the sockets API level, level is specified
	// as SOL_SOCKET.  To manipulate options at any other level the
	// protocol number of the appropriate protocol controlling the
	// option is supplied.  For example, to indicate that an option is
	// to be interpreted by the TCP protocol, level should be set to the
	// protocol number of TCP; see getprotoent(3).
	//
	// # Option argument
	//
	// The arguments optval and optlen are used to access option values
	// for setsockopt().  For getsockopt() they identify a buffer in
	// which the value for the requested option(s) are to be returned.
	// In Go we provide developers with an `any` interface to be able
	// to pass driver-specific configurations.
	SetSockOpt(sockfd int, level int, opt int, value interface{}) error
}

var ErrNetdevNotSet = errors.New("Netdev not set")

// nopNetdev is a NOP netdev that errors out any interface calls
type nopNetdev struct {
}

func (n *nopNetdev) GetHostByName(name string) (netip.Addr, error) {
	return netip.Addr{}, ErrNetdevNotSet
}
func (n *nopNetdev) Addr() (netip.Addr, error) { return netip.Addr{}, ErrNetdevNotSet }
func (n *nopNetdev) Socket(domain int, stype int, protocol int) (sockfd int, _ error) {
	return -1, ErrNetdevNotSet
}
func (n *nopNetdev) Bind(sockfd int, ip netip.AddrPort) error                 { return ErrNetdevNotSet }
func (n *nopNetdev) Connect(sockfd int, host string, ip netip.AddrPort) error { return ErrNetdevNotSet }
func (n *nopNetdev) Listen(sockfd int, backlog int) error                     { return ErrNetdevNotSet }
func (n *nopNetdev) Accept(sockfd int) (int, netip.AddrPort, error) {
	return -1, netip.AddrPort{}, ErrNetdevNotSet
}
func (n *nopNetdev) Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	return -1, ErrNetdevNotSet
}
func (n *nopNetdev) Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	return -1, ErrNetdevNotSet
}
func (n *nopNetdev) Close(sockfd int) error { return ErrNetdevNotSet }
func (n *nopNetdev) SetSockOpt(sockfd int, level int, opt int, value interface{}) error {
	return ErrNetdevNotSet
}
