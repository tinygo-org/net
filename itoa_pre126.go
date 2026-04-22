//go:build !go1.26

package net

import "internal/itoa"

func netItoa(i int) string { return itoa.Itoa(i) }
