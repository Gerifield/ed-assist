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
	mu           sync.RWMutex
	bindingsPath string
	registry     *BindsRegistry
	sender       KeySender
}

// NewController creates a new GameController.
func NewController(bindingsPath string, sender KeySender) *Controller {
	if sender == nil {
		sender = NewKeySender()
	}
	return &Controller{
		bindingsPath: bindingsPath,
		registry:     NewBindsRegistry(),
		sender:       sender,
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

// Load loads or reloads the active Elite Dangerous .binds file.
func (c *Controller) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	bindsFile, err := FindActiveBindsFile(c.bindingsPath)
	if err != nil {
		return fmt.Errorf("failed locating active binds: %w", err)
	}

	reg := NewBindsRegistry()
	if err := reg.ParseFile(bindsFile); err != nil {
		return fmt.Errorf("failed parsing binds file %s: %w", bindsFile, err)
	}

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
		holdMs = 80 // Default to 80ms
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
