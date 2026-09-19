//go:build !windows

package input

import (
	"fmt"
	"time"
)

// NonWindowsKeySender is a fallback stub when running natively on non-Windows platforms.
type NonWindowsKeySender struct{}

// NewKeySender returns a NonWindowsKeySender.
func NewKeySender() KeySender {
	return &NonWindowsKeySender{}
}

// SendKey returns an informative error on non-Windows systems.
func (s *NonWindowsKeySender) SendKey(scanCode ScanCode, holdDuration time.Duration, modifiers ...ScanCode) error {
	return fmt.Errorf("direct hardware keyboard scancode sending is only supported on Windows (or via Wine)")
}
