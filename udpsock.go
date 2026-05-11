// TINYGO: The following is copied and modified from Go 1.26.2 official implementation.

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"syscall"
	"time"
)

// UDPAddr represents the address of a UDP end point.
type UDPAddr struct {
	IP   IP
	Port int
	Zone string // IPv6 scoped addressing zone
}

// AddrPort returns the [UDPAddr] a as a [netip.AddrPort].
//
// If a.Port does not fit in a uint16, it's silently truncated.
//
// If a is nil, a zero value is returned.
func (a *UDPAddr) AddrPort() netip.AddrPort {
	if a == nil {
		return netip.AddrPort{}
	}
	na, _ := netip.AddrFromSlice(a.IP)
	na = na.WithZone(a.Zone)
	return netip.AddrPortFrom(na, uint16(a.Port))
}

// Network returns the address's network name, "udp".
func (a *UDPAddr) Network() string { return "udp" }

func (a *UDPAddr) String() string {
	if a == nil {
		return "<nil>"
	}
	ip := ipEmptyString(a.IP)
	if a.Zone != "" {
		return JoinHostPort(ip+"%"+a.Zone, strconv.Itoa(a.Port))
	}
	return JoinHostPort(ip, strconv.Itoa(a.Port))
}

func (a *UDPAddr) isWildcard() bool {
	if a == nil || a.IP == nil {
		return true
	}
	return a.IP.IsUnspecified()
}

func (a *UDPAddr) opAddr() Addr {
	if a == nil {
		return nil
	}
	return a
}

// ResolveUDPAddr returns an address of UDP end point.
//
// The network must be a UDP network name.
//
// If the host in the address parameter is not a literal IP address or
// the port is not a literal port number, ResolveUDPAddr resolves the
// address to an address of UDP end point.
// Otherwise, it parses the address as a pair of literal IP address
// and port number.
// The address parameter can use a host name, but this is not
// recommended, because it will return at most one of the host name's
// IP addresses.
//
// See func [Dial] for a description of the network and address
// parameters.
func ResolveUDPAddr(network, address string) (*UDPAddr, error) {

	switch network {
	case "udp", "udp4":
	default:
		return nil, fmt.Errorf("Network '%s' not supported", network)
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
		return &UDPAddr{Port: port}, nil
	}

	ip, err := netdev.GetHostByName(host)
	if err != nil {
		return nil, fmt.Errorf("Lookup of host name '%s' failed: %s", host, err)
	}

	return &UDPAddr{IP: ip.AsSlice(), Port: port}, nil
}

// UDPAddrFromAddrPort returns addr as a [UDPAddr]. If addr.IsValid() is false,
// then the returned UDPAddr will contain a nil IP field, indicating an
// address family-agnostic unspecified address.
func UDPAddrFromAddrPort(addr netip.AddrPort) *UDPAddr {
	return &UDPAddr{
		IP:   addr.Addr().AsSlice(),
		Zone: addr.Addr().Zone(),
		Port: int(addr.Port()),
	}
}

// UDPConn is the implementation of the Conn and PacketConn interfaces
// for UDP network connections.
type UDPConn struct {
	fd            int
	net           string
	laddr         *UDPAddr
	raddr         *UDPAddr
	readDeadline  time.Time
	writeDeadline time.Time
}

// Use IANA RFC 6335 port range 49152–65535 for ephemeral (dynamic) ports
var eport = int32(49151)

func ephemeralPort() int {
	if eport == int32(65535) {
		eport = int32(49151)
	} else {
		eport++
	}
	return int(eport)
}

// DialUDP acts like Dial for UDP networks.
//
// The network must be a UDP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialUDP(network string, laddr, raddr *UDPAddr) (*UDPConn, error) {
	switch network {
	case "udp", "udp4":
	default:
		return nil, fmt.Errorf("Network '%s' not supported", network)
	}

	// TINYGO: Use netdev to create UDP socket and connect

	if laddr == nil {
		laddr = &UDPAddr{}
	}

	if raddr == nil {
		raddr = &UDPAddr{}
	}

	if raddr.IP.IsUnspecified() {
		return nil, fmt.Errorf("Sorry, localhost isn't available on Tinygo")
	}

	// If no port was given, grab an ephemeral port
	if laddr.Port == 0 {
		laddr.Port = ephemeralPort()
	}

	fd, err := netdev.Socket(_AF_INET, _SOCK_DGRAM, _IPPROTO_UDP)
	if err != nil {
		return nil, err
	}
	lip, _ := netip.AddrFromSlice(laddr.IP)
	laddrport := netip.AddrPortFrom(lip, uint16(laddr.Port))

	// Local bind
	err = netdev.Bind(fd, laddrport)
	if err != nil {
		netdev.Close(fd)
		return nil, err
	}

	rip, _ := netip.AddrFromSlice(raddr.IP)
	raddrport := netip.AddrPortFrom(rip, uint16(raddr.Port))
	// Remote connect
	if err = netdev.Connect(fd, "", raddrport); err != nil {
		netdev.Close(fd)
		return nil, err
	}

	return &UDPConn{
		fd:    fd,
		net:   network,
		laddr: laddr,
		raddr: raddr,
	}, nil
}

// SyscallConn returns a raw network connection.
// This implements the syscall.Conn interface.
func (c *UDPConn) SyscallConn() (syscall.RawConn, error) {
	return nil, errors.New("SyscallConn not implemented")
}

// TINYGO: Use netdev for Conn methods: Read = Recv, Write = Send, etc.

func (c *UDPConn) Read(b []byte) (int, error) {
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

func (c *UDPConn) Write(b []byte) (int, error) {
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

// ReadFromUDP acts like [UDPConn.ReadFrom] but returns a UDPAddr.
func (c *UDPConn) ReadFromUDP(b []byte) (n int, addr *UDPAddr, err error) {
	// This function is designed to allow the caller to control the lifetime
	// of the returned *UDPAddr and thereby prevent an allocation.
	// See https://blog.filippo.io/efficient-go-apis-with-the-inliner/.
	// The real work is done by readFromUDP, below.
	return c.readFromUDP(b, &UDPAddr{})
}

// readFromUDP implements ReadFromUDP.
func (c *UDPConn) readFromUDP(b []byte, addr *UDPAddr) (int, *UDPAddr, error) {
	n, addr, err := c.readFrom(b, addr)
	if err != nil {
		err = &OpError{Op: "read", Net: c.net, Source: c.laddr, Addr: c.raddr, Err: err}
	}
	return n, addr, err
}

// ReadFrom implements the PacketConn ReadFrom method.
func (c *UDPConn) ReadFrom(b []byte) (int, Addr, error) {
	n, addr, err := c.readFromUDP(b, &UDPAddr{})
	if addr == nil {
		// Return Addr(nil), not Addr(*UDPConn(nil)).
		return n, nil, err
	}
	return n, addr, err
}

func (c *UDPConn) readFrom(b []byte, addr *UDPAddr) (int, *UDPAddr, error) {
	n, raddr, err := netdev.RecvFrom(c.fd, b, 0, c.readDeadline)
	if n < 0 {
		n = 0
	}
	if err != nil && err != io.EOF {
		err = &OpError{Op: "read", Net: c.net, Source: c.laddr, Addr: c.raddr, Err: err}
	}
	return n, UDPAddrFromAddrPort(raddr), err
}

// ReadMsgUDP reads a message from c, copying the payload into b and
// the associated out-of-band data into oob. It returns the number of
// bytes copied into b, the number of bytes copied into oob, the flags
// that were set on the message and the source address of the message.
//
// The packages golang.org/x/net/ipv4 and golang.org/x/net/ipv6 can be
// used to manipulate IP-level socket options in oob.
func (c *UDPConn) ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *UDPAddr, err error) {
	err = errors.New("ReadMsgUDP not implemented")
	return
}

// WriteToUDP acts like [UDPConn.WriteTo] but takes a [UDPAddr].
func (c *UDPConn) WriteToUDP(b []byte, addr *UDPAddr) (int, error) {
	n, err := c.writeTo(b, addr)
	if err != nil {
		err = &OpError{Op: "write", Net: c.net, Source: c.laddr, Addr: addr.opAddr(), Err: err}
	}
	return n, err
}

// WriteToUDPAddrPort acts like [UDPConn.WriteTo] but takes a [netip.AddrPort].
func (c *UDPConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	n, err := c.writeToAddrPort(b, addr)
	if err != nil {
		err = &OpError{Op: "write", Net: c.net, Source: c.laddr, Addr: UDPAddrFromAddrPort(addr), Err: err}
	}
	return n, err
}

// WriteTo implements the [PacketConn] WriteTo method.
func (c *UDPConn) WriteTo(b []byte, addr Addr) (int, error) {
	a, ok := addr.(*UDPAddr)
	if !ok {
		return 0, &OpError{Op: "write", Net: c.net, Source: c.laddr, Addr: addr, Err: syscall.EINVAL}
	}
	n, err := c.writeTo(b, a)
	if err != nil {
		err = &OpError{Op: "write", Net: c.net, Source: c.laddr, Addr: a.opAddr(), Err: err}
	}
	return n, err
}

func (c *UDPConn) writeTo(b []byte, addr *UDPAddr) (int, error) {
	return c.writeToAddrPort(b, addr.AddrPort())
}

func (c *UDPConn) writeToAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	n, err := netdev.SendTo(c.fd, b, 0, c.writeDeadline, addr)
	if n < 0 {
		n = 0
	}
	if err != nil {
		err = &OpError{Op: "write", Net: c.net, Source: c.laddr, Addr: UDPAddrFromAddrPort(addr), Err: err}
	}
	return n, err
}

// WriteMsgUDP writes a message to addr via c if c isn't connected, or
// to c's remote address if c is connected (in which case addr must be
// nil). The payload is copied from b and the associated out-of-band
// data is copied from oob. It returns the number of payload and
// out-of-band bytes written.
//
// The packages [golang.org/x/net/ipv4] and [golang.org/x/net/ipv6] can be
// used to manipulate IP-level socket options in oob.
func (c *UDPConn) WriteMsgUDP(b, oob []byte, addr *UDPAddr) (n, oobn int, err error) {
	return 0, 0, errors.New("WriteMsgUDP not implemented")
}

// WriteMsgUDPAddrPort is like [UDPConn.WriteMsgUDP] but takes a [netip.AddrPort] instead of a [UDPAddr].
func (c *UDPConn) WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort) (n, oobn int, err error) {
	return 0, 0, errors.New("WriteMsgUDPAddrPort not implemented")
}

func (c *UDPConn) Close() error {
	return netdev.Close(c.fd)
}

func (c *UDPConn) LocalAddr() Addr {
	return c.laddr
}

func (c *UDPConn) RemoteAddr() Addr {
	return c.raddr
}

func (c *UDPConn) SetDeadline(t time.Time) error {
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

func (c *UDPConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

func (c *UDPConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

func listenUDP(laddr *UDPAddr) (*UDPConn, error) {
	fd, err := netdev.Socket(_AF_INET, _SOCK_DGRAM, _IPPROTO_UDP)

	if err != nil {
		return nil, err
	}

	laddrport := laddr.AddrPort()
	err = netdev.Bind(fd, laddrport)
	if err != nil {
		return nil, err
	}

	return &UDPConn{
		fd:    fd,
		laddr: laddr,
	}, nil
}
