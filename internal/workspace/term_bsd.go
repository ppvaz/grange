//go:build darwin || freebsd || netbsd || openbsd

package workspace

import "syscall"

const ioctlGetTermios = syscall.TIOCGETA
