// TINYGO tests for TCPConn.ReadFrom and its generic copy helper.

package net

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestGenericReadFrom(t *testing.T) {
	var dst bytes.Buffer
	src := strings.NewReader("payload bytes")

	n, err := genericReadFrom(&dst, src)
	if err != nil {
		t.Fatalf("genericReadFrom: %v", err)
	}
	if n != int64(len("payload bytes")) {
		t.Errorf("n = %d, want %d", n, len("payload bytes"))
	}
	if dst.String() != "payload bytes" {
		t.Errorf("copied %q, want %q", dst.String(), "payload bytes")
	}
}

// readerFromSpy is an io.Writer that also implements io.ReaderFrom. If
// io.Copy ever routes through ReadFrom, the spy records it — genericReadFrom
// must hide it (via onlyWriter) to avoid recursing back into TCPConn.ReadFrom.
type readerFromSpy struct {
	bytes.Buffer
	readFromCalled bool
}

func (s *readerFromSpy) ReadFrom(r io.Reader) (int64, error) {
	s.readFromCalled = true
	return s.Buffer.ReadFrom(r)
}

func TestGenericReadFromHidesReaderFrom(t *testing.T) {
	spy := &readerFromSpy{}
	if _, err := genericReadFrom(spy, strings.NewReader("abc")); err != nil {
		t.Fatalf("genericReadFrom: %v", err)
	}
	if spy.readFromCalled {
		t.Error("onlyWriter failed to hide ReadFrom; io.Copy recursed via ReaderFrom")
	}
	if spy.String() != "abc" {
		t.Errorf("copied %q, want abc", spy.String())
	}
}

func TestTCPConnReadFrom(t *testing.T) {
	f := &fakeNetdev{}
	withNetdev(t, f)

	c := &TCPConn{fd: 1, net: "tcp"}
	src := strings.NewReader("streamed over tcp")
	n, err := c.ReadFrom(src)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if n != int64(len("streamed over tcp")) {
		t.Errorf("n = %d, want %d", n, len("streamed over tcp"))
	}
	if string(f.sent) != "streamed over tcp" {
		t.Errorf("netdev received %q, want %q", f.sent, "streamed over tcp")
	}
}
