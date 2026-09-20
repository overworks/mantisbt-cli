package cli

import (
	"syscall"
	"unsafe"
)

func isTerminal(fd uintptr) bool {
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGETA, uintptr(unsafe.Pointer(&state)))
	return errno == 0
}
