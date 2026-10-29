//go:build linux && !baremetal && !tinygo.wasm

package net

import "testing"

func TestTCPConnLocalAddr(t *testing.T) {
	l, err := Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	client, err := Dial("tcp4", "127.0.0.1:"+netItoa(l.Addr().(*TCPAddr).Port))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	if la, _ := client.LocalAddr().(*TCPAddr); la == nil || la.String() != server.RemoteAddr().String() {
		t.Errorf("dialed LocalAddr %v, peer sees %v", client.LocalAddr(), server.RemoteAddr())
	}
	if la := server.LocalAddr().String(); la != client.RemoteAddr().String() {
		t.Errorf("accepted LocalAddr %v, peer dialed %v", la, client.RemoteAddr())
	}
}
