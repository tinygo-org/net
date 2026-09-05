package net

import (
	"context"
	"errors"
	"net/netip"
	"syscall"
	"testing"
	"time"
)

type listenConfigNetdev struct {
	nopNetdev
	lookup  func()
	sockets int
	bound   netip.AddrPort
	listens int
	closed  int
}

func (d *listenConfigNetdev) GetHostByName(string) (netip.Addr, error) {
	if d.lookup != nil {
		d.lookup()
	}
	return netip.MustParseAddr("127.0.0.1"), nil
}
func (d *listenConfigNetdev) Socket(int, int, int) (int, error) {
	d.sockets++
	return 42, nil
}
func (d *listenConfigNetdev) Bind(_ int, addr netip.AddrPort) error {
	d.bound = addr
	return nil
}
func (d *listenConfigNetdev) Listen(int, int) error {
	d.listens++
	return nil
}
func (d *listenConfigNetdev) Close(int) error {
	d.closed++
	return nil
}

func testListenConfigCall(lc *ListenConfig, ctx context.Context, network, address string) error {
	if network == "udp4" {
		c, err := lc.ListenPacket(ctx, network, address)
		if err != nil {
			return err
		}
		return c.Close()
	}
	l, err := lc.Listen(ctx, network, address)
	if err != nil {
		return err
	}
	return l.Close()
}

func TestListenConfigNetdev(t *testing.T) {
	previous := netdev
	defer func() { netdev = previous }()
	for _, network := range []string{"tcp4", "udp4"} {
		t.Run(network, func(t *testing.T) {
			d := &listenConfigNetdev{}
			netdev = d
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := testListenConfigCall(&ListenConfig{}, ctx, network, "test.invalid:12345"); err != nil {
				t.Fatal(err)
			}
			if d.sockets != 1 || d.bound.String() != "127.0.0.1:12345" || d.closed != 1 {
				t.Fatalf("socket=%d bound=%v close=%d", d.sockets, d.bound, d.closed)
			}
			wantListen := 0
			if network == "tcp4" {
				wantListen = 1
			}
			if d.listens != wantListen {
				t.Fatalf("listen=%d want %d", d.listens, wantListen)
			}
		})
	}
}

func TestListenConfigCancellation(t *testing.T) {
	previous := netdev
	defer func() { netdev = previous }()
	for _, network := range []string{"tcp4", "udp4"} {
		for _, duringLookup := range []bool{false, true} {
			d := &listenConfigNetdev{}
			netdev = d
			ctx, cancel := context.WithCancel(context.Background())
			if duringLookup {
				d.lookup = cancel
			} else {
				cancel()
			}
			err := testListenConfigCall(&ListenConfig{}, ctx, network, "test.invalid:12345")
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s duringLookup=%v error=%v", network, duringLookup, err)
			}
			if d.sockets != 0 {
				t.Fatalf("%s opened socket after cancellation", network)
			}
		}
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
		err := testListenConfigCall(&ListenConfig{}, ctx, network, "test.invalid:12345")
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("%s error=%v", network, err)
		}
	}
}

func TestListenConfigUnsupportedOptions(t *testing.T) {
	previous := netdev
	defer func() { netdev = previous }()
	called := false
	control := func(string, string, syscall.RawConn) error { called = true; return nil }
	for _, network := range []string{"tcp4", "udp4"} {
		d := &listenConfigNetdev{}
		netdev = d
		err := testListenConfigCall(&ListenConfig{Control: control}, context.Background(), network, "test.invalid:12345")
		if !errors.Is(err, errors.ErrUnsupported) || called || d.sockets != 0 {
			t.Fatalf("%s error=%v called=%v sockets=%d", network, err, called, d.sockets)
		}
	}
	for _, lc := range []ListenConfig{{KeepAlive: time.Second}, {KeepAlive: -1}, {KeepAliveConfig: KeepAliveConfig{Enable: true}}} {
		err := testListenConfigCall(&lc, context.Background(), "tcp4", "test.invalid:12345")
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("error=%v", err)
		}
	}
	if err := testListenConfigCall(&ListenConfig{KeepAlive: time.Second}, context.Background(), "udp4", "test.invalid:12345"); err != nil {
		t.Fatal(err)
	}
}
