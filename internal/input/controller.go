package input

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ExecutionResult contains details of an executed action.
type ExecutionResult struct {
	ActionName string   `json:"action_name"`
	Key        string   `json:"key"`
	Modifiers  []string `json:"modifiers,omitempty"`
	ScanCode   string   `json:"scan_code"`
	HoldMs     int      `json:"hold_ms"`
}

// Controller coordinates binds discovery, key lookup, and scancode execution.
type Controller struct {
	mu            sync.RWMutex
	bindingsPath  string
	defaultHoldMs int
	registry      *BindsRegistry
	sender        KeySender
}

// NewController creates a new GameController.
func NewController(bindingsPath string, defaultHoldMs int, sender KeySender) *Controller {
	if sender == nil {
		sender = NewKeySender()
	}
	if defaultHoldMs <= 0 {
		defaultHoldMs = 80
	}
	return &Controller{
		bindingsPath:  bindingsPath,
		defaultHoldMs: defaultHoldMs,
		registry:      NewBindsRegistry(),
		sender:        sender,
	}
}

// DefaultHoldMs returns the configured default key hold duration in ms.
func (c *Controller) DefaultHoldMs() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.defaultHoldMs
}

// SetDefaultHoldMs sets the default hold duration in ms.
func (c *Controller) SetDefaultHoldMs(ms int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ms > 0 {
		c.defaultHoldMs = ms
	}
}

// SetRegistry directly overrides the active binds registry (useful for tests or custom bindings).
func (c *Controller) SetRegistry(reg *BindsRegistry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if reg != nil {
		c.registry = reg
	}
}

// PresetName returns the active control scheme preset name.
func (c *Controller) PresetName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.registry != nil {
		return c.registry.PresetName
	}
	return ""
}

// Load loads or reloads the active Elite Dangerous .binds file.
// If no custom or game .binds file is found, it automatically falls back to the built-in standard default keyboard bindings.
func (c *Controller) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	bindsFile, err := FindActiveBindsFile(c.bindingsPath)
	if err != nil {
		slog.Warn("no .binds XML file found; activating built-in standard Elite Dangerous keyboard bindings (Keyboard & Mouse default)",
			"reason", err.Error(),
		)
		c.registry = NewDefaultBindsRegistry()
		slog.Info("loaded default elite dangerous key bindings",
			"preset", c.registry.PresetName,
			"bound_actions", len(c.registry.Bindings),
		)
		return nil
	}

	reg := NewBindsRegistry()
	if err := reg.ParseFile(bindsFile); err != nil {
		slog.Warn("failed parsing binds file; falling back to built-in default keyboard bindings",
			"file", bindsFile,
			"error", err,
		)
		c.registry = NewDefaultBindsRegistry()
		return nil
	}

	// Apply built-in default bindings for any actions not bound in the custom file
	reg.ApplyDefaults()

	c.registry = reg
	slog.Info("loaded elite dangerous key bindings",
		"file", bindsFile,
		"preset", reg.PresetName,
		"bound_actions", len(reg.Bindings),
	)

	return nil
}

// ExecuteAction looks up an action by canonical name or alias and sends the hardware scancodes.
func (c *Controller) ExecuteAction(action string, holdMs int) (*ExecutionResult, error) {
	c.mu.RLock()
	binding, found := c.registry.GetBinding(action)
	c.mu.RUnlock()

	if !found {
		return nil, fmt.Errorf("action %q not found or not bound to a keyboard key in .binds", action)
	}

	if holdMs <= 0 {
		c.mu.RLock()
		holdMs = c.defaultHoldMs
		c.mu.RUnlock()
	}
	if holdMs <= 0 {
		holdMs = 80 // Fallback safety
	}
	holdDuration := time.Duration(holdMs) * time.Millisecond

	err := c.sender.SendKey(
		binding.Binding.ScanCode,
		holdDuration,
		binding.Binding.ModifierScanCodes...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed executing key for %s: %w", binding.ActionName, err)
	}

	return &ExecutionResult{
		ActionName: binding.ActionName,
		Key:        binding.Binding.Key,
		Modifiers:  binding.Binding.Modifiers,
		ScanCode:   fmt.Sprintf("0x%02X", binding.Binding.ScanCode.Code),
		HoldMs:     holdMs,
	}, nil
}

// ListActions returns a list of all bound keyboard actions.
func (c *Controller) ListActions() []ActionBinding {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registry.ListActions()
}

// FormatActionsJSON formats all bound actions as a pretty JSON string.
func (c *Controller) FormatActionsJSON() (string, error) {
	actions := c.ListActions()
	bytes, err := json.MarshalIndent(actions, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
