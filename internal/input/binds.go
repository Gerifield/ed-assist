package input

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// BindingKey stores the key and any modifier keys required to execute an action.
type BindingKey struct {
	Device            string     `json:"device"`
	Key               string     `json:"key"`
	Modifiers         []string   `json:"modifiers,omitempty"`
	ScanCode          ScanCode   `json:"scan_code"`
	ModifierScanCodes []ScanCode `json:"modifier_scan_codes,omitempty"`
}

// ActionBinding represents a configured Elite Dangerous action and its key binding.
type ActionBinding struct {
	ActionName string     `json:"action_name"`
	Binding    BindingKey `json:"binding"`
	Aliases    []string   `json:"aliases,omitempty"`
}

// ActionAliases maps canonical Elite Dangerous action names to friendly shorthand aliases.
var ActionAliases = map[string][]string{
	"LandingGearToggle":      {"landing_gear", "gear", "toggle_gear"},
	"ToggleCargoScoop":       {"cargo_scoop", "cargo", "scoop", "toggle_cargo"},
	"DeployHardpointToggle":  {"hardpoints", "deploy_hardpoints", "toggle_hardpoints", "weapons"},
	"ShipSpotLightToggle":    {"lights", "headlights", "spotlights", "toggle_lights"},
	"NightVisionToggle":      {"night_vision", "nv", "toggle_night_vision"},
	"ToggleFlightAssist":     {"flight_assist", "fa", "toggle_fa"},
	"HyperSuperCombination":  {"fsd", "frame_shift_drive", "jump"},
	"Hyperspace":             {"hyperspace", "system_jump"},
	"Supercruise":            {"supercruise"},
	"EngineBoost":            {"boost", "engine_boost"},
	"SelectTarget":           {"target", "select_target", "target_ahead"},
	"CycleNextTarget":        {"next_target", "cycle_next_target"},
	"CyclePreviousTarget":    {"prev_target", "previous_target"},
	"SelectHighestThreat":    {"highest_threat", "target_highest_threat"},
	"CycleNextHostileTarget": {"next_hostile", "cycle_hostile"},
	"IncreaseSystemsPower":   {"pips_sys", "sys_pips", "power_sys", "sys"},
	"IncreaseEnginesPower":   {"pips_eng", "eng_pips", "power_eng", "eng"},
	"IncreaseWeaponsPower":   {"pips_wep", "wep_pips", "power_wep", "wep"},
	"ResetPowerDistribution": {"pips_reset", "reset_pips", "balance_pips"},
	"DeployHeatSink":         {"heatsink", "deploy_heatsink"},
	"FireChaffLauncher":      {"chaff", "fire_chaff"},
	"UseShieldCellBank":      {"shield_cell", "scb"},
	"ChargeECM":              {"ecm", "charge_ecm"},
	"GalaxyMapOpen":          {"galaxy_map", "open_galaxy_map"},
	"SystemMapOpen":          {"system_map", "open_system_map"},
	"FocusLeftPanel":         {"ui_left", "target_panel", "left_panel"},
	"FocusRightPanel":        {"ui_right", "system_panel", "right_panel"},
	"FocusCommsPanel":        {"ui_comms", "comms_panel"},
	"FocusRadarPanel":        {"ui_radar", "role_panel", "srv_panel"},
	"UI_Select":              {"ui_select", "select"},
	"UI_Back":                {"ui_back", "back"},
	"UI_Up":                  {"ui_up"},
	"UI_Down":                {"ui_down"},
	"UI_Left":                {"ui_left_nav"},
	"UI_Right":               {"ui_right_nav"},
}

// BindsRegistry stores all active keyboard bindings parsed from a .binds file.
type BindsRegistry struct {
	BindsFilePath string
	PresetName    string
	Bindings      map[string]ActionBinding // Canonical action name (lowercase) -> ActionBinding
	AliasMap      map[string]string        // Alias (lowercase) -> Canonical action name (lowercase)
}

// NewBindsRegistry creates an empty registry.
func NewBindsRegistry() *BindsRegistry {
	return &BindsRegistry{
		Bindings: make(map[string]ActionBinding),
		AliasMap: make(map[string]string),
	}
}

// DetermineDefaultBindingsDir returns standard bindings path based on operating system.
func DetermineDefaultBindingsDir() string {
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, "Frontier Developments", "Elite Dangerous", "Options", "Bindings")
	}

	currUser, err := user.Current()
	if err == nil && currUser != nil && currUser.HomeDir != "" {
		// Windows path fallback or Linux Proton path check
		candidates := []string{
			filepath.Join(currUser.HomeDir, "AppData", "Local", "Frontier Developments", "Elite Dangerous", "Options", "Bindings"),
			filepath.Join(currUser.HomeDir, ".local", "share", "Steam", "steamapps", "compatdata", "359320", "pfx", "drive_c", "users", "steamuser", "AppData", "Local", "Frontier Developments", "Elite Dangerous", "Options", "Bindings"),
		}
		for _, cand := range candidates {
			if info, err := os.Stat(cand); err == nil && info.IsDir() {
				return cand
			}
		}
	}

	return ""
}

// FindActiveBindsFile resolves the active .binds XML file given a path to a directory or direct file.
func FindActiveBindsFile(path string) (string, error) {
	if path == "" {
		path = DetermineDefaultBindingsDir()
	}
	if path == "" {
		return "", fmt.Errorf("no bindings directory found; please specify bindings_path in config.ini")
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("bindings path not accessible: %w", err)
	}

	// If direct file provided
	if !info.IsDir() {
		return path, nil
	}

	// If directory, first check StartPreset.*.start files
	presetFiles := []string{
		filepath.Join(path, "StartPreset.4.start"),
		filepath.Join(path, "StartPreset.3.start"),
		filepath.Join(path, "StartPreset.start"),
	}

	for _, pf := range presetFiles {
		if data, err := os.ReadFile(pf); err == nil {
			presetName := strings.TrimSpace(string(data))
			if presetName != "" {
				candidate := filepath.Join(path, presetName+".binds")
				if _, err := os.Stat(candidate); err == nil {
					return candidate, nil
				}
				// Also try with .4.0 or .3.0 suffix if presetName was "Custom"
				candidate4 := filepath.Join(path, presetName+".4.0.binds")
				if _, err := os.Stat(candidate4); err == nil {
					return candidate4, nil
				}
			}
		}
	}

	// Fallback: look for any .binds files in the directory
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed reading bindings directory: %w", err)
	}

	var latestBinds string
	var latestTime int64

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".binds") {
			continue
		}
		entryPath := filepath.Join(path, entry.Name())
		if fi, err := entry.Info(); err == nil {
			if fi.ModTime().UnixNano() > latestTime {
				latestTime = fi.ModTime().UnixNano()
				latestBinds = entryPath
			}
		}
	}

	if latestBinds != "" {
		return latestBinds, nil
	}

	return "", fmt.Errorf("no .binds XML files found in %s", path)
}

// rawBindingNode is used to decode the XML structure of an action binding.
type rawBindingNode struct {
	Device    string           `xml:"Device,attr"`
	Key       string           `xml:"Key,attr"`
	Modifiers []rawModifierNode `xml:"Modifier"`
}

type rawModifierNode struct {
	Device string `xml:"Device,attr"`
	Key    string `xml:"Key,attr"`
}

// ParseReader parses an Elite Dangerous .binds XML stream.
func (r *BindsRegistry) ParseReader(reader io.Reader) error {
	decoder := xml.NewDecoder(reader)

	r.Bindings = make(map[string]ActionBinding)
	r.AliasMap = make(map[string]string)

	// Populate aliases lookup
	for canonical, aliases := range ActionAliases {
		cLower := strings.ToLower(canonical)
		for _, alias := range aliases {
			r.AliasMap[strings.ToLower(alias)] = cLower
		}
		r.AliasMap[cLower] = cLower
	}

	var currentAction string

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("xml decoding error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "Root" {
				for _, attr := range elem.Attr {
					if attr.Name.Local == "PresetName" {
						r.PresetName = attr.Value
					}
				}
				continue
			}

			// Top-level action element under Root
			if currentAction == "" {
				currentAction = elem.Name.Local
				continue
			}

			// Sub-element (e.g. Primary or Secondary)
			if elem.Name.Local == "Primary" || elem.Name.Local == "Secondary" {
				var node rawBindingNode
				if err := decoder.DecodeElement(&node, &elem); err == nil {
					// Only bind if device is Keyboard and key is set
					if strings.EqualFold(node.Device, "Keyboard") && node.Key != "" {
						cLower := strings.ToLower(currentAction)

						// Check if we already have a valid Primary binding
						if _, exists := r.Bindings[cLower]; !exists {
							sc, err := LookupScanCode(node.Key)
							if err == nil {
								bk := BindingKey{
									Device:   node.Device,
									Key:      node.Key,
									ScanCode: sc,
								}
								for _, mod := range node.Modifiers {
									if strings.EqualFold(mod.Device, "Keyboard") && mod.Key != "" {
										if modSC, err := LookupScanCode(mod.Key); err == nil {
											bk.Modifiers = append(bk.Modifiers, mod.Key)
											bk.ModifierScanCodes = append(bk.ModifierScanCodes, modSC)
										}
									}
								}

								ab := ActionBinding{
									ActionName: currentAction,
									Binding:    bk,
									Aliases:    ActionAliases[currentAction],
								}
								r.Bindings[cLower] = ab
								r.AliasMap[cLower] = cLower
							}
						}
					}
				}
			}

		case xml.EndElement:
			if elem.Name.Local == currentAction {
				currentAction = ""
			}
		}
	}

	return nil
}

// ParseFile loads and parses a .binds file from disk.
func (r *BindsRegistry) ParseFile(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed opening binds file %s: %w", filePath, err)
	}
	defer f.Close()

	r.BindsFilePath = filePath
	return r.ParseReader(f)
}

// GetBinding looks up an action binding by exact action name or alias (case-insensitive).
func (r *BindsRegistry) GetBinding(actionOrAlias string) (*ActionBinding, bool) {
	key := strings.ToLower(strings.TrimSpace(actionOrAlias))

	// 1. Direct or alias lookup
	if canonical, ok := r.AliasMap[key]; ok {
		if b, exists := r.Bindings[canonical]; exists {
			return &b, true
		}
	}

	// 2. Direct map check
	if b, exists := r.Bindings[key]; exists {
		return &b, true
	}

	return nil, false
}

// ListActions returns all bound keyboard actions.
func (r *BindsRegistry) ListActions() []ActionBinding {
	list := make([]ActionBinding, 0, len(r.Bindings))
	for _, b := range r.Bindings {
		list = append(list, b)
	}
	return list
}
