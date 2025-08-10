//go:build wasip2

// WASI preview 2 UDP

package net

import (
	"fmt"
	"time"

	"internal/cm"
	instancenetwork "internal/wasi/sockets/v0.2.0/instance-network"
	"internal/wasi/sockets/v0.2.0/network"
	"internal/wasi/sockets/v0.2.0/udp"
	udpcreatesocket "internal/wasi/sockets/v0.2.0/udp-create-socket"
)

type wasip2UdpSocket struct {
	udpcreatesocket.UDPSocket
	udp.Pollable
	*udp.IncomingDatagramStream
	*udp.OutgoingDatagramStream
}

func createUDPSocket(af network.IPAddressFamily) (wasip2Socket, error) {
	res := udpcreatesocket.CreateUDPSocket(af)
	if res.IsErr() {
		return nil, fmt.Errorf("failed to create UDP socket: %s", res.Err().String())
	}

	sock := res.OK()
	return &wasip2UdpSocket{
		UDPSocket: *sock,
		Pollable:  sock.Subscribe(),
	}, nil
}

func (s *wasip2UdpSocket) Bind(globalNetwork instancenetwork.Network, addr network.IPSocketAddress) error {
	res := s.StartBind(globalNetwork, addr)
	if res.IsErr() {
		return fmt.Errorf("failed to start binding socket: %s", res.Err().String())
	}

	res = s.FinishBind()
	if res.IsErr() {
		return fmt.Errorf("failed to finish binding socket: %s", res.Err().String())
	}

	return nil
}

func (s *wasip2UdpSocket) Listen(backlog int) error {
	fmt.Println("wasip2 UDP listen TODO:", backlog) ///
	return nil
}

func (s *wasip2UdpSocket) Accept() (wasip2Socket, *network.IPSocketAddress, error) {
	return nil, nil, fmt.Errorf("wasip2 UDP sockets do not support Accept")
}

func (s *wasip2UdpSocket) Connect(globalNetwork instancenetwork.Network, host string, ip network.IPSocketAddress) error {
	res := s.UDPSocket.Stream(cm.Some(ip))

	if res.IsErr() {
		return fmt.Errorf("failed to connect UDP socket: %s", res.Err().String())
	}

	s.IncomingDatagramStream, s.OutgoingDatagramStream = &res.OK().F0, &res.OK().F1

	return nil
}

func (c wasip2UdpSocket) Send(buf []byte, flags int, deadline time.Time) (int, error) {
	if flags != 0 {
		fmt.Println("wasip2 UDP send flags TODO:", flags) ///
	}

	if c.OutgoingDatagramStream == nil {
		return -1, fmt.Errorf("send called on a socket without open streams")
	}

	res := c.OutgoingDatagramStream.CheckSend()
	if res.IsErr() {
		return -1, fmt.Errorf("failed to write to output stream: %s", res.Err().String())
	}

	if *res.OK() == 0 {
		c.Block()
	}

	a := []udp.OutgoingDatagram{
		{
			Data: cm.NewList(&buf[0], len(buf)),
			// RemoteAddress: cm.Some(c.raddr),
		},
	}

	c.OutgoingDatagramStream.Send(cm.NewList(&a[0], 1))

	return len(buf), nil
}

func (c wasip2UdpSocket) Recv(buf []byte, flags int, deadline time.Time) (int, error) {
	if flags != 0 {
		fmt.Println("wasip2 UDP recv flags TODO:", flags) ///
	}

	if c.IncomingDatagramStream == nil {
		return -1, fmt.Errorf("recv called on a socket without open streams")
	}

	res := c.Receive(1)
	if res.IsErr() {
		return -1, fmt.Errorf("failed to read from input stream: %s", res.Err().String())
	}

	if res.OK().Len() != 1 {
		return -1, fmt.Errorf("expected 1 datagram, got %d", res.OK().Len())
	}

	return copy(buf, res.OK().Data().Data.Slice()), nil
}

func (c wasip2UdpSocket) Close() error {
	if c.IncomingDatagramStream != nil {
		c.IncomingDatagramStream.ResourceDrop()
	}
	if c.OutgoingDatagramStream != nil {
		c.OutgoingDatagramStream.ResourceDrop()
	}
	c.Pollable.ResourceDrop()
	c.UDPSocket.ResourceDrop()

	return nil
}
