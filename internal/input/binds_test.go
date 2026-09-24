package input

import (
	"strings"
	"testing"
)

const sampleBindsXML = `<?xml version="1.0" encoding="UTF-8" ?>
<Root PresetName="Custom" MajorVersion="4" MinorVersion="0">
	<KeyboardLayout>en-US</KeyboardLayout>
	<LandingGearToggle>
		<Primary Device="Keyboard" Key="Key_L" />
		<Secondary Device="{NoDevice}" Key="" />
	</LandingGearToggle>
	<ToggleCargoScoop>
		<Primary Device="Keyboard" Key="Key_Home" />
		<Secondary Device="{NoDevice}" Key="" />
	</ToggleCargoScoop>
	<DeployHardpointToggle>
		<Primary Device="Keyboard" Key="Key_U">
			<Modifier Device="Keyboard" Key="Key_LeftControl" />
		</Primary>
		<Secondary Device="{NoDevice}" Key="" />
	</DeployHardpointToggle>
	<ShipSpotLightToggle>
		<Primary Device="GamePad" Key="Pad_A" />
		<Secondary Device="Keyboard" Key="Key_Insert" />
	</ShipSpotLightToggle>
	<HyperSuperCombination>
		<Primary Device="Keyboard" Key="Key_J" />
		<Secondary Device="{NoDevice}" Key="" />
	</HyperSuperCombination>
	<ResetPowerDistribution>
		<Primary Device="Keyboard" Key="Key_DownArrow" />
		<Secondary Device="{NoDevice}" Key="" />
	</ResetPowerDistribution>
</Root>`

func TestBindsRegistryParse(t *testing.T) {
	registry := NewBindsRegistry()
	err := registry.ParseReader(strings.NewReader(sampleBindsXML))
	if err != nil {
		t.Fatalf("unexpected error parsing binds xml: %v", err)
	}

	if registry.PresetName != "Custom" {
		t.Errorf("expected PresetName 'Custom', got '%s'", registry.PresetName)
	}

	// 1. Direct LandingGearToggle test
	gear, found := registry.GetBinding("LandingGearToggle")
	if !found {
		t.Fatalf("LandingGearToggle binding not found")
	}
	if gear.Binding.Key != "Key_L" {
		t.Errorf("expected Key_L, got %s", gear.Binding.Key)
	}
	if gear.Binding.ScanCode.Code != 0x26 {
		t.Errorf("expected scancode 0x26, got 0x%X", gear.Binding.ScanCode.Code)
	}

	// 2. Alias lookup test: "landing_gear" and "gear"
	aliasGear, found := registry.GetBinding("landing_gear")
	if !found || aliasGear.ActionName != "LandingGearToggle" {
		t.Errorf("failed looking up LandingGearToggle by alias 'landing_gear'")
	}

	aliasGearShort, found := registry.GetBinding("gear")
	if !found || aliasGearShort.ActionName != "LandingGearToggle" {
		t.Errorf("failed looking up LandingGearToggle by alias 'gear'")
	}

	// 3. Modifier test: DeployHardpointToggle with Key_LeftControl
	hardpoints, found := registry.GetBinding("hardpoints")
	if !found {
		t.Fatalf("hardpoints binding not found")
	}
	if hardpoints.Binding.Key != "Key_U" {
		t.Errorf("expected Key_U, got %s", hardpoints.Binding.Key)
	}
	if len(hardpoints.Binding.Modifiers) != 1 || hardpoints.Binding.Modifiers[0] != "Key_LeftControl" {
		t.Errorf("expected 1 modifier 'Key_LeftControl', got %v", hardpoints.Binding.Modifiers)
	}
	if len(hardpoints.Binding.ModifierScanCodes) != 1 || hardpoints.Binding.ModifierScanCodes[0].Code != 0x1D {
		t.Errorf("expected modifier scancode 0x1D, got %v", hardpoints.Binding.ModifierScanCodes)
	}

	// 4. Secondary keyboard fallback: ShipSpotLightToggle has GamePad in Primary, Keyboard in Secondary
	lights, found := registry.GetBinding("lights")
	if !found {
		t.Fatalf("lights binding not found from Secondary keyboard")
	}
	if lights.Binding.Key != "Key_Insert" {
		t.Errorf("expected Key_Insert, got %s", lights.Binding.Key)
	}
	if !lights.Binding.ScanCode.Extended {
		t.Errorf("expected Key_Insert to be marked as extended key")
	}

	// 5. Extended arrow key: ResetPowerDistribution with Key_DownArrow
	pipsReset, found := registry.GetBinding("pips_reset")
	if !found {
		t.Fatalf("pips_reset binding not found")
	}
	if !pipsReset.Binding.ScanCode.Extended {
		t.Errorf("expected Key_DownArrow to be marked as extended key")
	}

	// 6. ListActions returns all bound actions
	actions := registry.ListActions()
	if len(actions) != 6 {
		t.Errorf("expected 6 actions, got %d", len(actions))
	}
}

func TestDefaultBindsRegistry(t *testing.T) {
	reg := NewDefaultBindsRegistry()
	if reg.PresetName == "" {
		t.Errorf("expected non-empty preset name")
	}

	// Verify standard keys exist
	expectedKeys := map[string]string{
		"landing_gear": "Key_L",
		"boost":        "Key_Tab",
		"pips_sys":     "Key_LeftArrow",
		"pips_eng":     "Key_UpArrow",
		"pips_wep":     "Key_RightArrow",
		"pips_reset":   "Key_DownArrow",
		"hardpoints":   "Key_U",
		"cargo_scoop":  "Key_Home",
		"fsd":          "Key_J",
		"target":       "Key_T",
	}

	for alias, expectedKey := range expectedKeys {
		b, found := reg.GetBinding(alias)
		if !found {
			t.Errorf("expected default binding for %s to be found", alias)
			continue
		}
		if b.Binding.Key != expectedKey {
			t.Errorf("expected key %s for %s, got %s", expectedKey, alias, b.Binding.Key)
		}
		if b.Binding.ScanCode.Code == 0 {
			t.Errorf("expected non-zero scancode for %s", alias)
		}
	}
}

func TestApplyDefaultsToPartialRegistry(t *testing.T) {
	reg := NewBindsRegistry()
	_ = reg.ParseReader(strings.NewReader(sampleBindsXML))

	// In sampleBindsXML, EngineBoost (boost) is NOT bound
	if _, found := reg.GetBinding("boost"); found {
		t.Errorf("expected boost to not be in sample XML")
	}

	// Apply defaults
	reg.ApplyDefaults()

	// Now boost should be populated with default Key_Tab
	boost, found := reg.GetBinding("boost")
	if !found {
		t.Fatalf("expected boost to be populated by ApplyDefaults")
	}
	if boost.Binding.Key != "Key_Tab" {
		t.Errorf("expected Key_Tab, got %s", boost.Binding.Key)
	}

	// LandingGearToggle from the XML (Key_L) should NOT have been overwritten
	gear, found := reg.GetBinding("gear")
	if !found || gear.Binding.Key != "Key_L" {
		t.Errorf("expected custom landing gear to remain intact")
	}
}
