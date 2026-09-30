//go:build linux

package workspace

import "syscall"

const ioctlGetTermios = syscall.TCGETS
