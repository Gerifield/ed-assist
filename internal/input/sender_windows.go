//go:build windows

package input

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const (
	inputKeyboard         = 1
	keyeventfExtendedKey  = 0x0001
	keyeventfKeyUp        = 0x0002
	keyeventfScanCode     = 0x0008
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procSendInput           = user32.NewProc("SendInput")
	procKeybdEvent          = user32.NewProc("keybd_event")
	procFindWindow          = user32.NewProc("FindWindowW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type inputEvent struct {
	inputType uint32
	_         [4]byte // 64-bit alignment padding
	ki        keybdInput
	_         [8]byte // Union size padding to match MOUSEINPUT (total 40 bytes on amd64)
}

// WindowsKeySender implements KeySender on Windows using user32 SendInput with hardware scancodes.
type WindowsKeySender struct {
	autoFocus bool
}

// NewKeySender creates a WindowsKeySender.
func NewKeySender() KeySender {
	return &WindowsKeySender{autoFocus: true}
}

func (s *WindowsKeySender) focusEliteWindow() {
	if !s.autoFocus {
		return
	}
	className, err := syscall.UTF16PtrFromString("FrontierDevelopmentsAppWinClass")
	if err == nil {
		hwnd, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(className)), 0)
		if hwnd != 0 {
			procSetForegroundWindow.Call(hwnd)
			time.Sleep(25 * time.Millisecond)
		}
	}
}

func (s *WindowsKeySender) sendRawKey(code uint16, extended, keyUp bool) error {
	var flags uint32 = keyeventfScanCode
	if extended {
		flags |= keyeventfExtendedKey
	}
	if keyUp {
		flags |= keyeventfKeyUp
	}

	if procSendInput.Find() == nil {
		var in inputEvent
		in.inputType = inputKeyboard
		in.ki.wScan = code
		in.ki.dwFlags = flags
		ret, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), uintptr(unsafe.Sizeof(in)))
		if ret != 0 {
			return nil
		}
		// If SendInput returned 0, fall back to keybd_event
		_ = err
	}

	// Fallback to keybd_event
	if procKeybdEvent.Find() == nil {
		procKeybdEvent.Call(0, uintptr(code), uintptr(flags), 0)
		return nil
	}

	return fmt.Errorf("user32 input functions not available")
}

// SendKey executes a key press and release sequence with hold duration and optional modifiers.
func (s *WindowsKeySender) SendKey(scanCode ScanCode, holdDuration time.Duration, modifiers ...ScanCode) error {
	s.focusEliteWindow()

	if holdDuration <= 0 {
		holdDuration = 80 * time.Millisecond
	}

	// 1. Press all modifiers in order
	for _, mod := range modifiers {
		if err := s.sendRawKey(mod.Code, mod.Extended, false); err != nil {
			return fmt.Errorf("failed pressing modifier scancode 0x%X: %w", mod.Code, err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 2. Press main key down
	if err := s.sendRawKey(scanCode.Code, scanCode.Extended, false); err != nil {
		return fmt.Errorf("failed pressing key scancode 0x%X: %w", scanCode.Code, err)
	}

	// 3. Hold key down for specified duration (~80ms minimum for game loop detection)
	time.Sleep(holdDuration)

	// 4. Release main key
	if err := s.sendRawKey(scanCode.Code, scanCode.Extended, true); err != nil {
		return fmt.Errorf("failed releasing key scancode 0x%X: %w", scanCode.Code, err)
	}

	// 5. Release modifiers in reverse order
	for i := len(modifiers) - 1; i >= 0; i-- {
		mod := modifiers[i]
		time.Sleep(20 * time.Millisecond)
		_ = s.sendRawKey(mod.Code, mod.Extended, true)
	}

	return nil
}
