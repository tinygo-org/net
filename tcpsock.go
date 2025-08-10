// TINYGO: The following is copied and modified from Go 1.21.4 official implementation.

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"errors"
	"fmt"
	"internal/itoa"
	"io"
	"net/netip"
	"strconv"
	"syscall"
	"time"
)

// TCPAddr represents the address of a TCP end point.
type TCPAddr struct {
	IP   IP
	Port int
	Zone string // IPv6 scoped addressing zone
}

// AddrPort returns the TCPAddr a as a netip.AddrPort.
//
// If a.Port does not fit in a uint16, it's silently truncated.
//
// If a is nil, a zero value is returned.
func (a *TCPAddr) AddrPort() netip.AddrPort {
	if a == nil {
		return netip.AddrPort{}
	}
	na, _ := netip.AddrFromSlice(a.IP)
	na = na.WithZone(a.Zone)
	return netip.AddrPortFrom(na, uint16(a.Port))
}

// Network returns the address's network name, "tcp".
func (a *TCPAddr) Network() string { return "tcp" }

func (a *TCPAddr) String() string {
	if a == nil {
		return "<nil>"
	}
	ip := ipEmptyString(a.IP)
	if a.Zone != "" {
		return JoinHostPort(ip+"%"+a.Zone, itoa.Itoa(a.Port))
	}
	return JoinHostPort(ip, itoa.Itoa(a.Port))
}

func (a *TCPAddr) isWildcard() bool {
	if a == nil || a.IP == nil {
		return true
	}
	return a.IP.IsUnspecified()
}

func (a *TCPAddr) opAddr() Addr {
	if a == nil {
		return nil
	}
	return a
}

// ResolveTCPAddr returns an address of TCP end point.
//
// The network must be a TCP network name.
//
// If the host in the address parameter is not a literal IP address or
// the port is not a literal port number, ResolveTCPAddr resolves the
// address to an address of TCP end point.
// Otherwise, it parses the address as a pair of literal IP address
// and port number.
// The address parameter can use a host name, but this is not
// recommended, because it will return at most one of the host name's
// IP addresses.
//
// See func Dial for a description of the network and address
// parameters.
func ResolveTCPAddr(network, address string) (*TCPAddr, error) {

	switch network {
	case "tcp", "tcp4":
	default:
		return nil, fmt.Errorf("Network '%s' not supported", network)
	}

	switch address {
	case ":http":
		address = ":80"
	}

	// TINYGO: Use netdev resolver

	host, sport, err := SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(sport)
	if err != nil {
		return nil, fmt.Errorf("Error parsing port '%s' in address: %s",
			sport, err)
	}

	if host == "" {
		return &TCPAddr{IP: []byte{0, 0, 0, 0}, Port: port}, nil
	}

	ip, err := netdev.GetHostByName(host)
	if err != nil {
		return nil, fmt.Errorf("Lookup of host name '%s' failed: %s", host, err)
	}

	return &TCPAddr{IP: ip.AsSlice(), Port: port}, nil
}

// TCPAddrFromAddrPort returns addr as a TCPAddr. If addr.IsValid() is false,
// then the returned TCPAddr will contain a nil IP field, indicating an
// address family-agnostic unspecified address.
func TCPAddrFromAddrPort(addr netip.AddrPort) *TCPAddr {
	return &TCPAddr{
		IP:   addr.Addr().AsSlice(),
		Zone: addr.Addr().Zone(),
		Port: int(addr.Port()),
	}
}

// TCPConn is an implementation of the Conn interface for TCP network
// connections.
type TCPConn struct {
	fd            int
	net           string
	laddr         *TCPAddr
	raddr         *TCPAddr
	readDeadline  time.Time
	writeDeadline time.Time
}

// DialTCP acts like Dial for TCP networks.
//
// The network must be a TCP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialTCP(network string, laddr, raddr *TCPAddr) (*TCPConn, error) {

	switch network {
	case "tcp", "tcp4":
	default:
		return nil, errors.New("Network not supported: '" + network + "'")
	}

	// TINYGO: Use netdev to create TCP socket and connect

	if raddr == nil {
		raddr = &TCPAddr{}
	}

	if raddr.IP.IsUnspecified() {
		return nil, errors.New("Sorry, localhost isn't available on Tinygo")
	} else if len(raddr.IP) != 4 {
		return nil, errors.New("only ipv4 supported")
	}

	fd, err := netdev.Socket(_AF_INET, _SOCK_STREAM, _IPPROTO_TCP)
	if err != nil {
		return nil, err
	}

	rip, _ := netip.AddrFromSlice(raddr.IP)
	raddrport := netip.AddrPortFrom(rip, uint16(raddr.Port))
	if err = netdev.Connect(fd, "", raddrport); err != nil {
		netdev.Close(fd)
		return nil, err
	}

	return &TCPConn{
		fd:    fd,
		net:   network,
		laddr: laddr,
		raddr: raddr,
	}, nil
}

// TINYGO: Use netdev for Conn methods: Read = Recv, Write = Send, etc.

// SyscallConn returns a raw network connection.
// This implements the syscall.Conn interface.
func (c *TCPConn) SyscallConn() (syscall.RawConn, error) {
	return nil, errors.New("SyscallConn not implemented")
}

func (c *TCPConn) Read(b []byte) (int, error) {
	n, err := netdev.Recv(c.fd, b, 0, c.readDeadline)
	// Turn the -1 socket error into 0 and let err speak for error
	if n < 0 {
		n = 0
	}
	if err != nil && err != io.EOF {
		err = &OpError{Op: "read", Net: c.net, Source: c.laddr, Addr: c.raddr, Err: err}
	}
	return n, err
}

func (c *TCPConn) Write(b []byte) (int, error) {
	n, err := netdev.Send(c.fd, b, 0, c.writeDeadline)
	// Turn the -1 socket error into 0 and let err speak for error
	if n < 0 {
		n = 0
	}
	if err != nil {
		err = &OpError{Op: "write", Net: c.net, Source: c.laddr, Addr: c.raddr, Err: err}
	}
	return n, err
}

func (c *TCPConn) Close() error {
	return netdev.Close(c.fd)
}

func (c *TCPConn) LocalAddr() Addr {
	return c.laddr
}

func (c *TCPConn) RemoteAddr() Addr {
	return c.raddr
}

func (c *TCPConn) SetDeadline(t time.Time) error {
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

// SetLinger sets the behavior of Close on a connection which still
// has data waiting to be sent or to be acknowledged.
//
// If sec < 0 (the default), the operating system finishes sending the
// data in the background.
//
// If sec == 0, the operating system discards any unsent or
// unacknowledged data.
//
// If sec > 0, the data is sent in the background as with sec < 0.
// On some operating systems including Linux, this may cause Close to block
// until all data has been sent or discarded.
// On some operating systems after sec seconds have elapsed any remaining
// unsent data may be discarded.
func (c *TCPConn) SetLinger(sec int) error {
	return netdev.SetSockOpt(c.fd, _SOL_SOCKET, _SO_LINGER, sec)
}

// SetKeepAlive sets whether the operating system should send
// keep-alive messages on the connection.
func (c *TCPConn) SetKeepAlive(keepalive bool) error {
	return netdev.SetSockOpt(c.fd, _SOL_SOCKET, _SO_KEEPALIVE, keepalive)
}

// SetKeepAlivePeriod sets period between keep-alives.
func (c *TCPConn) SetKeepAlivePeriod(d time.Duration) error {
	// Units are 1/2 seconds
	return netdev.SetSockOpt(c.fd, _SOL_TCP, _TCP_KEEPINTVL, 2*d.Seconds())
}

func (c *TCPConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

func (c *TCPConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

func (c *TCPConn) CloseWrite() error {
	return fmt.Errorf("CloseWrite not implemented")
}

type listener struct {
	fd    int
	laddr *TCPAddr
}

func (l *listener) Accept() (Conn, error) {
	fd, raddr, err := netdev.Accept(l.fd)
	if err != nil {
		return nil, err
	}

	return &TCPConn{
		fd:    fd,
		net:   "tcp",
		laddr: l.laddr,
		raddr: TCPAddrFromAddrPort(raddr),
	}, nil
}

func (l *listener) Close() error {
	return netdev.Close(l.fd)
}

func (l *listener) Addr() Addr {
	return l.laddr
}

func listenTCP(laddr *TCPAddr) (Listener, error) {
	fd, err := netdev.Socket(_AF_INET, _SOCK_STREAM, _IPPROTO_TCP)
	if err != nil {
		return nil, err
	}

	laddrport := laddr.AddrPort()
	err = netdev.Bind(fd, laddrport)
	if err != nil {
		return nil, err
	}

	err = netdev.Listen(fd, 5)
	if err != nil {
		return nil, err
	}

	return &listener{fd: fd, laddr: laddr}, nil
}

// TCPListener is a TCP network listener. Clients should typically
// use variables of type Listener instead of assuming TCP.
type TCPListener struct {
	listener
}
