// TINYGO: The following is copied and modified from Go 1.21.4 official implementation.

// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"errors"
)

// LookupPort looks up the port for the given network and service.
//
// LookupPort uses context.Background internally; to specify the context, use
// Resolver.LookupPort.
func LookupPort(network, service string) (port int, err error) {
	return 0, errors.New("net:LookupPort not implemented")
}
