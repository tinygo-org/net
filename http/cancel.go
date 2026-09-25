package http

import (
	"context"
	"io"
	"net"
	"sync"
)

// Netdev dialing does not use the context yet.
// See https://github.com/tinygo-org/net/blob/70037cf71f1ae14175781e162c9c76d1ac730ece/dial.go#L154-L158.
func dialRequest(ctx context.Context, dial func() (net.Conn, error)) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		conn net.Conn
		err  error
	}
	ready := make(chan result)
	go func() {
		conn, err := dial()
		if err != nil && conn != nil {
			conn.Close()
			conn = nil
		}
		select {
		case ready <- result{conn, err}:
		case <-ctx.Done():
			if conn != nil {
				conn.Close()
			}
		}
	}()
	select {
	case r := <-ready:
		if err := ctx.Err(); err != nil {
			if r.conn != nil {
				r.conn.Close()
			}
			return nil, err
		}
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type onceCloseBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

func (b *onceCloseBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

type cancelBody struct {
	io.ReadCloser
	ctx  context.Context
	stop func()
	err  error
}

func (b *cancelBody) Read(p []byte) (n int, err error) {
	if b.err != nil {
		return 0, b.err
	}
	if err = b.ctx.Err(); err == nil {
		n, err = b.ReadCloser.Read(p)
		if err != nil && b.ctx.Err() != nil {
			err = b.ctx.Err()
		}
	}
	if err != nil {
		b.err = err
		b.stop()
	}
	return n, err
}

func (b *cancelBody) Close() error {
	b.stop()
	return b.ReadCloser.Close()
}
