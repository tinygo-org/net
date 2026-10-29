//go:build linux && !baremetal && !tinygo.wasm

package http

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"syscall"
	"testing"
)

// freePort returns a loopback port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	return sa.(*syscall.SockaddrInet4).Port
}

// upgradeServer answers one request with a 101 and the given early bytes in
// the same write, then echoes whatever the client sends prefixed with "echo:".
func upgradeServer(t *testing.T, early string) string {
	t.Helper()
	addr := "127.0.0.1:" + strconv.Itoa(freePort(t))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		if _, err := ReadRequest(br); err != nil {
			return
		}
		io.WriteString(c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"+early)
		buf := make([]byte, 64)
		for {
			n, err := br.Read(buf)
			if err != nil {
				return
			}
			c.Write(append([]byte("echo:"), buf[:n]...))
		}
	}()
	return addr
}

func TestClientProtocolSwitchBodyIsReadWriteCloser(t *testing.T) {
	addr := upgradeServer(t, "hello")
	req, err := NewRequest("GET", "http://"+addr+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	resp, err := (&Transport{}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != StatusSwitchingProtocols {
		t.Fatalf("status %d, want 101", resp.StatusCode)
	}
	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		t.Fatalf("101 response body %T is not an io.ReadWriteCloser", resp.Body)
	}
	defer rwc.Close()

	readN := func(want string) {
		t.Helper()
		got := make([]byte, len(want))
		if _, err := io.ReadFull(rwc, got); err != nil {
			t.Fatalf("read %q: %v", want, err)
		}
		if string(got) != want {
			t.Fatalf("read %q, want %q", got, want)
		}
	}
	readN("hello")
	if _, err := io.WriteString(rwc, "ping"); err != nil {
		t.Fatal(err)
	}
	readN("echo:ping")
}
