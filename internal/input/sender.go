package input

import (
	"time"
)

// KeySender defines an interface for sending low-level hardware keyboard scancodes to the OS/game.
type KeySender interface {
	SendKey(scanCode ScanCode, holdDuration time.Duration, modifiers ...ScanCode) error
}
