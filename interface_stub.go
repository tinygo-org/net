// TINYGO: The following is copied and modified from Go 1.26.2 official implementation.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build js || wasip1

package net

// TINYGO: mirrors upstream net/interface_stub.go. These platforms have no
// interface table at all, so enumeration yields an empty list and no error,
// rather than failing. Callers that merely enumerate then see zero interfaces
// instead of a hard error; InterfaceByIndex/InterfaceByName still report
// errNoSuchInterface, since an empty table contains nothing.

func interfaceTable(ifindex int) ([]Interface, error) {
	return nil, nil
}

func interfaceAddrTable(ifi *Interface) ([]Addr, error) {
	return nil, nil
}

func interfaceMulticastAddrTable(ifi *Interface) ([]Addr, error) {
	return nil, nil
}
