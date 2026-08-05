// Package ui handles terminal interaction: TTY detection, the confirmation
// menu, and the $EDITOR handoff.
package ui

import "os"

// IsTTY reports whether f is a character device, i.e. an interactive terminal.
// When stdout is not a TTY, gitia behaves as if --yes were passed.
func IsTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
