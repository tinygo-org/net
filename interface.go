// TINYGO: The following is copied and modified from Go 1.21.4 official implementation.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"errors"
)

// BUG(mikio): On JS, methods and functions related to
// Interface are not implemented.

// BUG(mikio): On AIX, DragonFly BSD, NetBSD, OpenBSD, Plan 9 and
// Solaris, the MulticastAddrs method of Interface is not implemented.

var (
	errInvalidInterface         = errors.New("invalid network interface")
	errInvalidInterfaceIndex    = errors.New("invalid network interface index")
	errInvalidInterfaceName     = errors.New("invalid network interface name")
	errNoSuchInterface          = errors.New("no such network interface")
	errNoSuchMulticastInterface = errors.New("no such multicast network interface")
)

// Interface represents a mapping between network interface name
// and index. It also represents network interface facility
// information.
type Interface struct {
	Index        int          // positive integer that starts at one, zero is never used
	MTU          int          // maximum transmission unit
	Name         string       // e.g., "en0", "lo0", "eth0.100"
	HardwareAddr HardwareAddr // IEEE MAC-48, EUI-48 and EUI-64 form
	Flags        Flags        // e.g., FlagUp, FlagLoopback, FlagMulticast
}

type Flags uint

const (
	FlagUp           Flags = 1 << iota // interface is administratively up
	FlagBroadcast                      // interface supports broadcast access capability
	FlagLoopback                       // interface is a loopback interface
	FlagPointToPoint                   // interface belongs to a point-to-point link
	FlagMulticast                      // interface supports multicast access capability
	FlagRunning                        // interface is in running state
)

var flagNames = []string{
	"up",
	"broadcast",
	"loopback",
	"pointtopoint",
	"multicast",
	"running",
}

func (f Flags) String() string {
	s := ""
	for i, name := range flagNames {
		if f&(1<<uint(i)) != 0 {
			if s != "" {
				s += "|"
			}
			s += name
		}
	}
	if s == "" {
		s = "0"
	}
	return s
}

// Interfaces returns a list of the system's network interfaces.
func Interfaces() ([]Interface, error) {
	return nil, errors.New("Interfaces not implemented")
}

// InterfaceByIndex returns the interface specified by index.
//
// On Solaris, it returns one of the logical network interfaces
// sharing the logical data link; for more precision use
// InterfaceByName.
func InterfaceByIndex(index int) (*Interface, error) {
	return nil, errors.New("InterfaceByIndex not implemented")
}
