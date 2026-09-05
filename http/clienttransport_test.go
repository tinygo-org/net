package http

import (
	"io"
	"strings"
	"testing"
)

type recordingTransport struct {
	calls int
	req   *Request
}

func (t *recordingTransport) RoundTrip(req *Request) (*Response, error) {
	t.calls++
	t.req = req
	return &Response{
		Status:     "200 OK",
		StatusCode: 200,
		Header:     Header{},
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    req,
	}, nil
}

// A client with no Transport must dispatch through DefaultTransport, whose
// RoundTrip is build-tagged: the fetch API on js/wasm, the netdev dial
// elsewhere. Dispatching to the package-level dialer directly is issue #66.
func TestClientUsesDefaultTransport(t *testing.T) {
	previous := DefaultTransport
	defer func() { DefaultTransport = previous }()
	rt := &recordingTransport{}
	DefaultTransport = rt

	c := &Client{}
	resp, err := c.Get("http://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if rt.calls != 1 {
		t.Fatalf("DefaultTransport.RoundTrip calls = %d, want 1", rt.calls)
	}
	if rt.req.URL.Host != "example.com" {
		t.Fatalf("request URL = %v", rt.req.URL)
	}
}

func TestClientUsesExplicitTransport(t *testing.T) {
	rt := &recordingTransport{}
	c := &Client{Transport: rt}
	resp, err := c.Get("http://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if rt.calls != 1 {
		t.Fatalf("Transport.RoundTrip calls = %d, want 1", rt.calls)
	}
}

func TestClientNoTransport(t *testing.T) {
	previous := DefaultTransport
	defer func() { DefaultTransport = previous }()
	DefaultTransport = nil

	c := &Client{}
	_, err := c.Get("http://example.com/")
	if err == nil || !strings.Contains(err.Error(), "no Client.Transport or DefaultTransport") {
		t.Fatalf("error = %v, want no-transport error", err)
	}
}
