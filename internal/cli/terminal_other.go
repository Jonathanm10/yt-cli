//go:build !darwin && !windows && !linux

package cli

import (
	"errors"
	"os"
)

func setTerminalEcho(_ *os.File, _ bool) error {
	return errors.New("terminal echo control is not implemented on this platform")
}
