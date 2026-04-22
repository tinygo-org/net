//go:build go1.26

package net

import "internal/strconv"

func netItoa(i int) string { return strconv.Itoa(i) }
