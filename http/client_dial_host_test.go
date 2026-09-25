// Copyright 2026 The TinyGo Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestClientDialsURLHostNotRequestHost checks tinygo-org/net#85: the native
// client must connect to Request.URL.Host and send Request.Host as the Host
// header, the same split net/http.Transport documents.
func TestClientDialsURLHostNotRequestHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()

	gotHost := make(chan string, 1)
	errc := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errc <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		br := bufio.NewReader(c)
		req, err := ReadRequest(br)
		if err != nil {
			errc <- err
			return
		}
		gotHost <- req.Host
		io.Copy(io.Discard, req.Body)
		req.Body.Close()
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
	}()

	req, err := NewRequest("GET", "http://"+ln.Addr().String()+"/notify", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "virtual.example"
	resp, err := DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do dialed Request.Host instead of URL.Host: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	select {
	case err := <-errc:
		t.Fatal(err)
	case host := <-gotHost:
		if host != "virtual.example" {
			t.Fatalf("Host header = %q, want virtual.example", host)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for accepted request")
	}
}

func TestClientDialsURLHostWhenRequestHostEmpty(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		br := bufio.NewReader(c)
		req, err := ReadRequest(br)
		if err != nil {
			done <- err
			return
		}
		io.Copy(io.Discard, req.Body)
		req.Body.Close()
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
		done <- nil
	}()

	req, err := NewRequest("GET", "http://"+ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = ""
	resp, err := DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do with empty Request.Host: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "ok") {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
