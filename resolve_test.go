package net

import "testing"

func TestResolveEmptyAddress(t *testing.T) {
	u, err := ResolveUDPAddr("udp", "")
	if err != nil || u.Port != 0 {
		t.Errorf("ResolveUDPAddr(\"udp\", \"\") = %v, %v; want port 0, nil", u, err)
	}
	a, err := ResolveTCPAddr("tcp", "")
	if err != nil || a.Port != 0 {
		t.Errorf("ResolveTCPAddr(\"tcp\", \"\") = %v, %v; want port 0, nil", a, err)
	}
}
