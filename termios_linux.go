//go:build linux

package snek

import "syscall"

const (
	termiosGetRequest = syscall.TCGETS
	termiosSetRequest = syscall.TCSETS
)
