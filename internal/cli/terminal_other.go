//go:build !darwin && !windows && !linux

package cli

import (
	"fmt"
	"os"
)

func setTerminalEcho(file *os.File, enabled bool) error {
	_ = file
	_ = enabled
	return fmt.Errorf("terminal echo control is not implemented on this platform")
}
