// TINYGO: The following is copied and modified from Go 1.26.2 official implementation.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(js || wasip1)

package net

import (
	"errors"
)

// TINYGO: not implemented; netdev exposes neither an interface table nor a
// per-interface address list.

func interfaceTable(ifindex int) ([]Interface, error) {
	return nil, errors.New("Interfaces not implemented")
}

func interfaceAddrTable(ifi *Interface) ([]Addr, error) {
	return nil, errors.New("InterfaceAddrs not implemented")
}

func interfaceMulticastAddrTable(ifi *Interface) ([]Addr, error) {
	return nil, errors.New("Interface.MulticastAddrs not implemented")
}
