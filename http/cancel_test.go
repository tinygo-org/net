//go:build !js

package http

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func pipeTransport(t *testing.T) (*Transport, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	return &Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}}, server
}

func waitError(t *testing.T, result <-chan error, want error) {
	t.Helper()
	select {
	case err := <-result:
		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not stop")
	}
}

func TestClientCancellation(t *testing.T) {
	for _, mode := range []string{"context", "deadline", "client", "legacy", "default", "earlier"} {
		t.Run(mode, func(t *testing.T) {
			transport, server := pipeTransport(t)
			client := &Client{Transport: transport}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := error(context.Canceled)
			if mode == "deadline" || mode == "earlier" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 50*time.Millisecond)
				defer stop()
				want = context.DeadlineExceeded
				if mode == "earlier" {
					client.Timeout = time.Minute
				}
			}
			if mode == "client" {
				client.Timeout = 50 * time.Millisecond
				want = context.DeadlineExceeded
			}
			if mode == "default" {
				old := DefaultTransport
				DefaultTransport = transport
				defer func() { DefaultTransport = old }()
				client.Transport = nil
			}
			req, _ := NewRequestWithContext(ctx, "GET", "http://example.test/", nil)
			legacy := make(chan struct{})
			if mode == "legacy" {
				req.Cancel = legacy
			}
			peerDone := make(chan error, 1)
			go func() {
				_, err := ReadRequest(bufio.NewReader(server))
				if err == nil {
					if mode == "legacy" {
						close(legacy)
					} else if mode != "client" && mode != "deadline" && mode != "earlier" {
						cancel()
					}
					_, err = io.Copy(io.Discard, server)
				}
				peerDone <- err
			}()
			result := make(chan error, 1)
			go func() {
				_, err := client.Do(req)
				var uerr *url.Error
				if !errors.As(err, &uerr) {
					result <- errors.New("request error is not a URL error")
					return
				}
				if want == context.DeadlineExceeded && !uerr.Timeout() {
					result <- errors.New("timeout error does not report Timeout")
					return
				}
				result <- err
			}()
			waitError(t, result, want)
			waitError(t, peerDone, nil)
		})
	}
}

func TestCanceledRequestDoesNotDial(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := NewRequestWithContext(ctx, "GET", "http://example.test/", nil)
		if legacy {
			ch := make(chan struct{})
			close(ch)
			req.Cancel = ch
		} else {
			cancel()
		}
		called := false
		transport := &Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
			called = true
			return nil, errors.New("unexpected dial")
		}}
		_, err := transport.RoundTrip(req)
		cancel()
		if !errors.Is(err, context.Canceled) || called {
			t.Fatalf("err=%v dial=%v", err, called)
		}
	}
}

func TestDialRequestCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	result := make(chan error, 1)
	go func() {
		_, err := dialRequest(ctx, func() (net.Conn, error) {
			close(started)
			<-release
			return client, nil
		})
		result <- err
	}()
	<-started
	cancel()
	waitError(t, result, context.Canceled)
	releaseOnce.Do(func() { close(release) })
	closed := make(chan error, 1)
	go func() {
		var b [1]byte
		_, err := server.Read(b[:])
		closed <- err
	}()
	waitError(t, closed, io.EOF)
}

func TestResponseBodyCancellation(t *testing.T) {
	for _, mode := range []string{"context", "client", "close", "complete"} {
		t.Run(mode, func(t *testing.T) {
			transport, server := pipeTransport(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &Client{Transport: transport}
			if mode == "client" {
				client.Timeout = 100 * time.Millisecond
			}
			peerDone := make(chan error, 1)
			go func() {
				_, err := ReadRequest(bufio.NewReader(server))
				if err == nil {
					body := "a"
					if mode == "complete" {
						body = "ab"
					}
					_, err = io.WriteString(server, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"+body)
				}
				if err == nil {
					_, err = io.Copy(io.Discard, server)
				}
				peerDone <- err
			}()
			req, _ := NewRequestWithContext(ctx, "GET", "http://example.test/", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var b [1]byte
			if _, err := io.ReadFull(resp.Body, b[:]); err != nil {
				t.Fatal(err)
			}
			if mode == "context" {
				cancel()
			}
			result := make(chan error, 1)
			go func() {
				if mode == "close" {
					resp.Body.Close()
					result <- nil
					return
				}
				_, err := io.ReadAll(resp.Body)
				if mode == "complete" && err == nil {
					_, err = resp.Body.Read(b[:])
				}
				result <- err
			}()
			want := error(context.Canceled)
			switch mode {
			case "client":
				want = context.DeadlineExceeded
			case "close":
				want = nil
			case "complete":
				want = io.EOF
			}
			waitError(t, result, want)
			waitError(t, peerDone, nil)
		})
	}
}

func TestRequestWriteCancellation(t *testing.T) {
	transport, _ := pipeTransport(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, _ := NewRequestWithContext(ctx, "POST", "http://example.test/", strings.NewReader(strings.Repeat("x", 8192)))
	result := make(chan error, 1)
	go func() { _, err := transport.RoundTrip(req); result <- err }()
	waitError(t, result, context.DeadlineExceeded)
}

type blockedRequestBody struct {
	started chan struct{}
	closed  chan struct{}
}

func (b *blockedRequestBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.closed
	return 0, io.EOF
}

func (b *blockedRequestBody) Close() error {
	close(b.closed)
	return nil
}

func TestRequestBodyCancellation(t *testing.T) {
	transport, server := pipeTransport(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &blockedRequestBody{started: make(chan struct{}), closed: make(chan struct{})}
	req, _ := NewRequestWithContext(ctx, "POST", "http://example.test/", body)
	go io.Copy(io.Discard, server)
	result := make(chan error, 1)
	go func() { _, err := transport.RoundTrip(req); result <- err }()
	select {
	case <-body.started:
	case <-time.After(3 * time.Second):
		t.Fatal("request body was not read")
	}
	cancel()
	waitError(t, result, context.Canceled)
}

func TestResponseConnectionCleanup(t *testing.T) {
	for _, response := range []string{
		"HTTP/1.1 204 No Content\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n",
		"invalid response\r\n\r\n",
	} {
		transport, server := pipeTransport(t)
		peerDone := make(chan error, 1)
		go func() {
			_, err := ReadRequest(bufio.NewReader(server))
			if err == nil {
				_, err = io.WriteString(server, response)
			}
			if err == nil {
				_, err = io.Copy(io.Discard, server)
			}
			peerDone <- err
		}()
		req, _ := NewRequest("GET", "http://example.test/", nil)
		resp, err := transport.RoundTrip(req)
		if strings.HasPrefix(response, "invalid") {
			if err == nil {
				t.Fatal("invalid response did not return an error")
			}
		} else if err != nil || resp.Body != NoBody {
			t.Fatalf("response=%v err=%v", resp, err)
		}
		waitError(t, peerDone, nil)
	}
}
