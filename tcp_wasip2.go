//go:build wasip2

// WASI preview 2 TCP

package net

import (
	"fmt"
	"time"

	"internal/cm"
	"internal/wasi/io/v0.2.0/streams"
	instancenetwork "internal/wasi/sockets/v0.2.0/instance-network"
	"internal/wasi/sockets/v0.2.0/network"
	"internal/wasi/sockets/v0.2.0/tcp"
	tcpcreatesocket "internal/wasi/sockets/v0.2.0/tcp-create-socket"
)

type wasip2TcpSocket struct {
	tcpcreatesocket.TCPSocket
	tcp.Pollable
	*streams.InputStream
	*streams.OutputStream
}

func createTCPSocket(af network.IPAddressFamily) (wasip2Socket, error) {
	res := tcpcreatesocket.CreateTCPSocket(af)
	if res.IsErr() {
		return nil, fmt.Errorf("failed to create TCP socket: %s", res.Err().String())
	}

	sock := res.OK()
	return &wasip2TcpSocket{
		TCPSocket: *sock,
		Pollable:  sock.Subscribe(),
	}, nil
}

func (s *wasip2TcpSocket) Bind(globalNetwork instancenetwork.Network, addr network.IPSocketAddress) error {
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

func (s *wasip2TcpSocket) Listen(backlog int) error {
	res := s.StartListen()
	if res.IsErr() {
		return fmt.Errorf("failed to start listening on socket: %s", res.Err().String())
	}

	res = s.FinishListen()
	if res.IsErr() {
		return fmt.Errorf("failed to finish listening on socket: %s", res.Err().String())
	}

	return nil
}

func (s *wasip2TcpSocket) Accept() (wasip2Socket, *network.IPSocketAddress, error) {
	var clientSocket *tcpcreatesocket.TCPSocket
	var inStream *streams.InputStream
	var outStream *streams.OutputStream

	for {
		res := s.TCPSocket.Accept()
		if res.IsOK() {
			clientSocket, inStream, outStream = &res.OK().F0, &res.OK().F1, &res.OK().F2
			break
		}

		if *res.Err() == network.ErrorCodeWouldBlock {
			// FIXME: a proper way is to use Pollable.Block()
			// But this seems to cause the single threaded runtime to block indefinitely
			for {
				if s.Pollable.Ready() {
					break
				}

				// HACK: Make sure to yield the execution to other goroutines
				time.Sleep(100 * time.Millisecond)
			}
			continue
		}

		return nil, nil, fmt.Errorf("failed to accept connection: %s", res.Err().String())

	}

	raddrRes := clientSocket.RemoteAddress()
	if raddrRes.IsErr() {
		return nil, nil, fmt.Errorf("failed to get remote address: %s", raddrRes.Err().String())
	}

	return &wasip2TcpSocket{
		TCPSocket:    *clientSocket,
		Pollable:     clientSocket.Subscribe(),
		InputStream:  inStream,
		OutputStream: outStream,
	}, raddrRes.OK(), nil
}

func (s *wasip2TcpSocket) Connect(globalNetwork instancenetwork.Network, host string, ip network.IPSocketAddress) error {
	res := s.StartConnect(globalNetwork, ip)
	if res.IsErr() {
		return fmt.Errorf("failed to start connecting socket: %s", res.Err().String())
	}

	for {
		connRes := s.FinishConnect()
		if connRes.IsOK() {
			s.InputStream, s.OutputStream = &connRes.OK().F0, &connRes.OK().F1
			return nil
		}

		if *connRes.Err() == network.ErrorCodeWouldBlock {
			s.Block()
			continue
		}

		return fmt.Errorf("failed to finish connecting socket: %s", connRes.Err().String())
	}
}

func (c wasip2TcpSocket) Send(buf []byte, flags int, deadline time.Time) (int, error) {
	if flags != 0 {
		fmt.Println("wasip2 TCP send flags TODO:", flags) ///
	}

	if c.OutputStream == nil {
		return -1, fmt.Errorf("send called on a socket without open streams")
	}

	res := c.BlockingWriteAndFlush(cm.ToList([]uint8(buf)))
	if res.IsErr() {
		return -1, fmt.Errorf("failed to write to output stream: %s", res.Err().String())
	}

	return len(buf), nil
}

func (c wasip2TcpSocket) Recv(buf []byte, flags int, deadline time.Time) (int, error) {
	if flags != 0 {
		fmt.Println("wasip2 TCP recv flags TODO:", flags) ///
	}

	if c.InputStream == nil {
		return -1, fmt.Errorf("recv called on a socket without open streams")
	}

	res := c.BlockingRead(uint64(len(buf)))
	if res.IsErr() {
		return -1, fmt.Errorf("failed to read from input stream: %s", res.Err().String())
	}

	return copy(buf, res.OK().Slice()), nil
}

func (c wasip2TcpSocket) Close() error {
	res := c.TCPSocket.Shutdown(tcp.ShutdownTypeBoth)
	if res.IsErr() {
		return fmt.Errorf("failed to shutdown client socket: %s", res.Err().String())
	}

	if c.InputStream != nil {
		c.InputStream.ResourceDrop()
	}
	if c.OutputStream != nil {
		c.OutputStream.ResourceDrop()
	}
	c.Pollable.ResourceDrop()
	c.TCPSocket.ResourceDrop()

	return nil
}
