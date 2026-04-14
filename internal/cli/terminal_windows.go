//go:build windows

package cli

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode  = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode  = kernel32.NewProc("SetConsoleMode")
	enableEchoInputMode = uint32(0x0004)
)

func setTerminalEcho(file *os.File, enabled bool) error {
	handle := syscall.Handle(file.Fd())
	var mode uint32
	if r1, _, err := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); r1 == 0 {
		return fmt.Errorf("get console mode: %w", err)
	}
	if enabled {
		mode |= enableEchoInputMode
	} else {
		mode &^= enableEchoInputMode
	}
	if r1, _, err := procSetConsoleMode.Call(uintptr(handle), uintptr(mode)); r1 == 0 {
		return fmt.Errorf("set console mode: %w", err)
	}
	return nil
}
