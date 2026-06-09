//go:build darwin

package snek

import "syscall"

const (
	termiosGetRequest = syscall.TIOCGETA
	termiosSetRequest = syscall.TIOCSETA
)
