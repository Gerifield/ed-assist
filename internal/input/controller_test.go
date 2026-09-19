package input

import (
	"strings"
	"testing"
	"time"
)

type mockKeySender struct {
	sentKeys      []ScanCode
	sentModifiers [][]ScanCode
	durations     []time.Duration
}

func (m *mockKeySender) SendKey(scanCode ScanCode, holdDuration time.Duration, modifiers ...ScanCode) error {
	m.sentKeys = append(m.sentKeys, scanCode)
	m.sentModifiers = append(m.sentModifiers, modifiers)
	m.durations = append(m.durations, holdDuration)
	return nil
}

func TestControllerExecuteAction(t *testing.T) {
	mockSender := &mockKeySender{}
	ctrl := NewController("", 120, mockSender)

	reg := NewBindsRegistry()
	if err := reg.ParseReader(strings.NewReader(sampleBindsXML)); err != nil {
		t.Fatalf("failed parsing binds: %v", err)
	}
	ctrl.registry = reg

	// 1. Test executing by alias "landing_gear" with explicit holdMs (100ms)
	res, err := ctrl.ExecuteAction("landing_gear", 100)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.ActionName != "LandingGearToggle" || res.Key != "Key_L" {
		t.Errorf("unexpected result: %+v", res)
	}
	if len(mockSender.sentKeys) != 1 || mockSender.sentKeys[0].Code != 0x26 {
		t.Errorf("expected sent key 0x26, got %v", mockSender.sentKeys)
	}
	if mockSender.durations[0] != 100*time.Millisecond {
		t.Errorf("expected 100ms duration, got %v", mockSender.durations[0])
	}

	// 2. Test executing action with modifiers "hardpoints" with 0 (should use default 120ms)
	resMod, err := ctrl.ExecuteAction("hardpoints", 0)
	if err != nil {
		t.Fatalf("unexpected modifier action error: %v", err)
	}
	if resMod.ActionName != "DeployHardpointToggle" || resMod.Key != "Key_U" {
		t.Errorf("unexpected result: %+v", resMod)
	}
	if len(mockSender.sentModifiers[1]) != 1 || mockSender.sentModifiers[1][0].Code != 0x1D {
		t.Errorf("expected modifier 0x1D, got %v", mockSender.sentModifiers[1])
	}
	if resMod.HoldMs != 120 {
		t.Errorf("expected default 120ms, got %d", resMod.HoldMs)
	}
	if mockSender.durations[1] != 120*time.Millisecond {
		t.Errorf("expected 120ms duration sent, got %v", mockSender.durations[1])
	}

	// 3. Test non-existent action
	_, err = ctrl.ExecuteAction("non_existent_action_xyz", 80)
	if err == nil {
		t.Fatalf("expected error for non-existent action")
	}
}
