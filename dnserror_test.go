// TINYGO test for DNSError.

package net

import (
	"errors"
	"testing"
)

func TestDNSErrorError(t *testing.T) {
	tests := []struct {
		name string
		e    *DNSError
		want string
	}{
		{"nil", nil, "<nil>"},
		{"no server", &DNSError{Err: "no such host", Name: "example.com"},
			"lookup example.com: no such host"},
		{"with server", &DNSError{Err: "timeout", Name: "example.com", Server: "8.8.8.8:53"},
			"lookup example.com on 8.8.8.8:53: timeout"},
	}
	for _, tt := range tests {
		if got := tt.e.Error(); got != tt.want {
			t.Errorf("%s: Error() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestDNSErrorUnwrap(t *testing.T) {
	inner := errors.New("boom")
	e := &DNSError{Err: "x", UnwrapErr: inner}
	if got := e.Unwrap(); got != inner {
		t.Errorf("Unwrap() = %v, want %v", got, inner)
	}
	if !errors.Is(e, inner) {
		t.Errorf("errors.Is(e, inner) = false, want true")
	}

	if (&DNSError{Err: "x"}).Unwrap() != nil {
		t.Errorf("Unwrap() with no UnwrapErr should be nil")
	}
}
