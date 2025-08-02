//go:build wasip2

// L3/L4 network/transport layer implementation using WASI preview 2 sockets

package net

import (
	"errors"
	"fmt"
	"net/netip"
	"time"

	instancenetwork "internal/wasi/sockets/v0.2.0/instance-network"
	ipnamelookup "internal/wasi/sockets/v0.2.0/ip-name-lookup"
	"internal/wasi/sockets/v0.2.0/network"
)

func TinygoToWasiAddr(ip netip.AddrPort) network.IPSocketAddress {
	if ip.Addr().Is4() {
		return network.IPSocketAddressIPv4(network.IPv4SocketAddress{
			Port:    ip.Port(),
			Address: ip.Addr().As4(),
		})
	}

	if ip.Addr().Is6() {
		as16 := ip.Addr().As16()
		var as8uint16 [8]uint16
		for i := 0; i < 8; i++ {
			as8uint16[i] = uint16(as16[i*2])<<8 | uint16(as16[i*2+1])
		}
		return network.IPSocketAddressIPv6(network.IPv6SocketAddress{
			Port:    ip.Port(),
			Address: as8uint16,
		})
	}

	return network.IPSocketAddress{}
}

func WasiAddrToTinygo(addr network.IPSocketAddress) netip.AddrPort {
	if addr4 := addr.IPv4(); addr4 != nil {
		return netip.AddrPortFrom(netip.AddrFrom4(addr4.Address), addr4.Port)
	}

	if addr6 := addr.IPv6(); addr6 != nil {
		var as16 [16]byte
		for i := 0; i < 8; i++ {
			as16[i*2] = byte(addr6.Address[i] >> 8)
			as16[i*2+1] = byte(addr6.Address[i] & 0xFF)
		}
		return netip.AddrPortFrom(netip.AddrFrom16(as16), addr6.Port)
	}

	return netip.AddrPort{}
}

type wasip2Socket interface {
	Recv(buf []byte, flags int, deadline time.Time) (int, error)
	Send(buf []byte, flags int, deadline time.Time) (int, error)
	Close() error
	Listen(backlog int) error
	Bind(globalNetwork instancenetwork.Network, ip network.IPSocketAddress) error
	Connect(globalNetwork instancenetwork.Network, host string, ip network.IPSocketAddress) error
	Accept() (wasip2Socket, *network.IPSocketAddress, error)
}

// wasip2Netdev is a netdev that uses WASI preview 2 sockets
type wasip2Netdev struct {
	fds    map[int]wasip2Socket
	nextFd int
	net    instancenetwork.Network
}

func init() {
	useNetdev(&wasip2Netdev{
		fds:    make(map[int]wasip2Socket),
		nextFd: 0,
		net:    instancenetwork.InstanceNetwork(),
	})
}

func (n *wasip2Netdev) GetHostByName(name string) (netip.Addr, error) {
	res := ipnamelookup.ResolveAddresses(n.net, name)

	if res.IsErr() {
		return netip.Addr{}, fmt.Errorf("failed to resolve address: %s", res.Err().String())
	}

	stream := res.OK()
	pollable := stream.Subscribe()

	for {
		pollable.Block()
		res := stream.ResolveNextAddress()

		if res.IsErr() {
			return netip.Addr{}, fmt.Errorf("failed to get resolved address: %s", res.Err().String())
		}

		if res.OK().None() {
			return netip.Addr{}, errors.New("no addresses found")
		}

		// TODO: handle IPv6
		if addr4 := res.OK().Some().IPv4(); addr4 != nil {
			return netip.AddrFrom4(*addr4), nil
		}
	}
}

func (n *wasip2Netdev) Addr() (netip.Addr, error) {
	fmt.Println("wasip2 TODO Addr") ///
	return netip.Addr{}, errors.New("wasip2 TODO Addr")
}

func (n *wasip2Netdev) getNextFD() int {
	n.nextFd++
	return n.nextFd - 1
}

func (n *wasip2Netdev) Socket(domain int, stype int, protocol int) (sockfd int, _ error) {
	af := network.IPAddressFamilyIPv4
	if domain == _AF_INET6 {
		af = network.IPAddressFamilyIPv6
	}

	var sock wasip2Socket
	var err error

	switch stype {
	case _SOCK_STREAM:
		sock, err = createTCPSocket(af)
		if err != nil {
			return -1, err
		}
	default:
		return -1, fmt.Errorf("wasip2: unsupported socket type %d", stype)
	}

	fd := n.getNextFD()
	n.fds[fd] = sock

	return fd, nil
}

func (n *wasip2Netdev) Bind(sockfd int, ip netip.AddrPort) error {
	sock, ok := n.fds[sockfd]
	if !ok {
		fmt.Println("wasip2: invalid socket fd") ///
		return errors.New("wasip2: invalid socket fd")
	}

	return sock.Bind(n.net, TinygoToWasiAddr(ip))
}

func (n *wasip2Netdev) Connect(sockfd int, host string, ip netip.AddrPort) error {
	sock, ok := n.fds[sockfd]
	if !ok {
		fmt.Println("wasip2: invalid socket fd") ///
		return errors.New("wasip2: invalid socket fd")
	}

	return sock.Connect(n.net, host, TinygoToWasiAddr(ip))
}

func (n *wasip2Netdev) Listen(sockfd int, backlog int) error {
	sock, ok := n.fds[sockfd]
	if !ok {
		fmt.Println("wasip2: invalid socket fd") ///
		return errors.New("wasip2: invalid socket fd")
	}

	return sock.Listen(backlog)
}

func (n *wasip2Netdev) Accept(sockfd int) (int, netip.AddrPort, error) {
	sock, ok := n.fds[sockfd]
	if !ok {
		fmt.Println("wasip2: invalid socket fd") ///
		return -1, netip.AddrPort{}, errors.New("wasip2: invalid socket fd")
	}

	newSock, raddr, err := sock.Accept()
	if err != nil {
		return -1, netip.AddrPort{}, fmt.Errorf("failed to accept connection: %s", err.Error())
	}

	fd := n.getNextFD()
	n.fds[fd] = newSock

	return fd, WasiAddrToTinygo(*raddr), nil
}

func (n *wasip2Netdev) Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	sock, ok := n.fds[sockfd]
	if !ok {
		fmt.Println("wasip2: invalid socket fd") ///
		return -1, errors.New("wasip2: invalid socket fd")
	}

	return sock.Send(buf, flags, deadline)
}

func (n *wasip2Netdev) Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	sock, ok := n.fds[sockfd]
	if !ok {
		fmt.Println("wasip2: invalid socket fd") ///
		return -1, errors.New("wasip2: invalid socket fd")
	}

	return sock.Recv(buf, flags, deadline)
}

func (n *wasip2Netdev) Close(sockfd int) error {
	sock, ok := n.fds[sockfd]
	if !ok {
		fmt.Println("wasip2: invalid socket fd") ///
		return errors.New("wasip2: invalid socket fd")
	}

	delete(n.fds, sockfd)

	return sock.Close()
}

func (n *wasip2Netdev) SetSockOpt(sockfd int, level int, opt int, value interface{}) error {
	fmt.Println("wasip2 setsockopt (TODO)", sockfd, level, opt, value) ///
	return errors.New("wasip2 TODO set socket option")
}
