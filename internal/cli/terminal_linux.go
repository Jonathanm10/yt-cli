//go:build linux

package cli

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func setTerminalEcho(file *os.File, enabled bool) error {
	var termios syscall.Termios
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&termios)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("get terminal settings: %w", errno)
	}
	if enabled {
		termios.Lflag |= syscall.ECHO
	} else {
		termios.Lflag &^= syscall.ECHO
	}
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&termios)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("set terminal settings: %w", errno)
	}
	return nil
}
