// TINYGO: The following is copied and modified from Go 1.22.0 official implementation.

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"context"
	"errors"
)

// lookupProtocol looks up IP protocol name and returns correspondent protocol number.
func lookupProtocol(ctx context.Context, name string) (int, error) {
	return 0, errors.New("net:lookupProtocol not implemented")
}
