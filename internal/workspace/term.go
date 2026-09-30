package workspace

import (
	"os"
	"syscall"
	"unsafe"
)

// IsTerminal reports whether f is a terminal. A character-device check isn't
// enough: /dev/null is one too, and dashboards start grange with it as stdin.
func IsTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlGetTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
