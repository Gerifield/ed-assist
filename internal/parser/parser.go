package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"ed-assist/internal/flags"
)

// Fuel contains fuel readouts for the ship in metric tons.
type Fuel struct {
	FuelMain      float64 `json:"FuelMain"`
	FuelReservoir float64 `json:"FuelReservoir"`
}

// Destination contains details about the currently targeted destination.
type Destination struct {
	System int64  `json:"System,omitempty"`
	Body   int    `json:"Body,omitempty"`
	Name   string `json:"Name,omitempty"`
}

// StatusFlags contains unpacked boolean flags describing the ship, SRV, and player state.
type StatusFlags struct {
	Docked                    bool
	Landed                    bool
	LandingGearDown           bool
	ShieldsUp                 bool
	Supercruise               bool
	FlightAssistOff           bool
	HardpointsDeployed        bool
	InWing                    bool
	LightsOn                  bool
	CargoScoopDeployed        bool
	SilentRunning             bool
	ScoopingFuel              bool
	SRVHandbrake              bool
	SRVTurret                 bool
	SRVUnderShip              bool
	SRVDriveAssist            bool
	FSDMassLocked             bool
	FSDCharging               bool
	FSDCooldown               bool
	LowFuel                   bool
	Overheating               bool
	HasLatLong                bool
	IsInDanger                bool
	BeingInterdicted          bool
	InMainShip                bool
	InFighter                 bool
	InSRV                     bool
	InAnalysisMode            bool
	NightVision               bool
	AltitudeFromAverageRadius bool
	FSDJump                   bool
	SRVHighBeam               bool
}

// StatusFlags2 contains unpacked boolean flags for Odyssey on-foot gameplay and additional states.
type StatusFlags2 struct {
	OnFoot                bool
	InTaxi                bool
	InMulticrew           bool
	OnFootInStation       bool
	OnFootOnPlanet        bool
	AimDownSight          bool
	LowOxygen             bool
	LowHealth             bool
	Cold                  bool
	Hot                   bool
	VeryCold              bool
	VeryHot               bool
	GlideMode             bool
	OnFootInHangar        bool
	OnFootSocialSpace     bool
	OnFootExterior        bool
	BreathableAtmosphere  bool
	TelepresenceMulticrew bool
	PhysicalMulticrew     bool
	FSDHyperdriveCharging bool
}

// Status represents the complete state payload of Status.json.
type Status struct {
	Timestamp               string        `json:"timestamp"`
	Event                   string        `json:"event"`
	RawFlags                uint32        `json:"Flags"`
	RawFlags2               uint32        `json:"Flags2,omitempty"`
	Flags                   StatusFlags   `json:"-"`
	Flags2                  StatusFlags2  `json:"-"`
	Pips                    [3]int        `json:"Pips,omitempty"`
	FireGroup               int           `json:"FireGroup,omitempty"`
	GuiFocus                uint32        `json:"GuiFocus,omitempty"`
	Fuel                    Fuel          `json:"Fuel,omitempty"`
	Cargo                   float64       `json:"Cargo,omitempty"`
	LegalState              string        `json:"LegalState,omitempty"`
	Latitude                *float64      `json:"Latitude,omitempty"`
	Longitude               *float64      `json:"Longitude,omitempty"`
	Altitude                *float64      `json:"Altitude,omitempty"`
	Heading                 *float64      `json:"Heading,omitempty"`
	BodyName                string        `json:"BodyName,omitempty"`
	PlanetRadius            *float64      `json:"PlanetRadius,omitempty"`
	Balance                 *int64        `json:"Balance,omitempty"`
	Destination             *Destination  `json:"Destination,omitempty"`
	Oxygen                  *float64      `json:"Oxygen,omitempty"`
	Health                  *float64      `json:"Health,omitempty"`
	Temperature             *float64      `json:"Temperature,omitempty"`
	SelectedWeapon          string        `json:"SelectedWeapon,omitempty"`
	SelectedWeaponLocalised string        `json:"SelectedWeapon_Localised,omitempty"`
	Gravity                 *float64      `json:"Gravity,omitempty"`
}

// ExpandFlags unpacks bitmasks RawFlags and RawFlags2 into boolean structs Flags and Flags2.
func (s *Status) ExpandFlags() {
	s.Flags = StatusFlags{
		Docked:                    s.RawFlags&flags.Docked != 0,
		Landed:                    s.RawFlags&flags.Landed != 0,
		LandingGearDown:           s.RawFlags&flags.LandingGearDown != 0,
		ShieldsUp:                 s.RawFlags&flags.ShieldsUp != 0,
		Supercruise:               s.RawFlags&flags.Supercruise != 0,
		FlightAssistOff:           s.RawFlags&flags.FlightAssistOff != 0,
		HardpointsDeployed:        s.RawFlags&flags.HardpointsDeployed != 0,
		InWing:                    s.RawFlags&flags.InWing != 0,
		LightsOn:                  s.RawFlags&flags.LightsOn != 0,
		CargoScoopDeployed:        s.RawFlags&flags.CargoScoopDeployed != 0,
		SilentRunning:             s.RawFlags&flags.SilentRunning != 0,
		ScoopingFuel:              s.RawFlags&flags.ScoopingFuel != 0,
		SRVHandbrake:              s.RawFlags&flags.SRVHandbrake != 0,
		SRVTurret:                 s.RawFlags&flags.SRVTurret != 0,
		SRVUnderShip:              s.RawFlags&flags.SRVUnderShip != 0,
		SRVDriveAssist:            s.RawFlags&flags.SRVDriveAssist != 0,
		FSDMassLocked:             s.RawFlags&flags.FSDMassLocked != 0,
		FSDCharging:               s.RawFlags&flags.FSDCharging != 0,
		FSDCooldown:               s.RawFlags&flags.FSDCooldown != 0,
		LowFuel:                   s.RawFlags&flags.LowFuel != 0,
		Overheating:               s.RawFlags&flags.Overheating != 0,
		HasLatLong:                s.RawFlags&flags.HasLatLong != 0,
		IsInDanger:                s.RawFlags&flags.IsInDanger != 0,
		BeingInterdicted:          s.RawFlags&flags.BeingInterdicted != 0,
		InMainShip:                s.RawFlags&flags.InMainShip != 0,
		InFighter:                 s.RawFlags&flags.InFighter != 0,
		InSRV:                     s.RawFlags&flags.InSRV != 0,
		InAnalysisMode:            s.RawFlags&flags.InAnalysisMode != 0,
		NightVision:               s.RawFlags&flags.NightVision != 0,
		AltitudeFromAverageRadius: s.RawFlags&flags.AltitudeFromAverageRadius != 0,
		FSDJump:                   s.RawFlags&flags.FSDJump != 0,
		SRVHighBeam:               s.RawFlags&flags.SRVHighBeam != 0,
	}

	s.Flags2 = StatusFlags2{
		OnFoot:                s.RawFlags2&flags.OnFoot != 0,
		InTaxi:                s.RawFlags2&flags.InTaxi != 0,
		InMulticrew:           s.RawFlags2&flags.InMulticrew != 0,
		OnFootInStation:       s.RawFlags2&flags.OnFootInStation != 0,
		OnFootOnPlanet:        s.RawFlags2&flags.OnFootOnPlanet != 0,
		AimDownSight:          s.RawFlags2&flags.AimDownSight != 0,
		LowOxygen:             s.RawFlags2&flags.LowOxygen != 0,
		LowHealth:             s.RawFlags2&flags.LowHealth != 0,
		Cold:                  s.RawFlags2&flags.Cold != 0,
		Hot:                   s.RawFlags2&flags.Hot != 0,
		VeryCold:              s.RawFlags2&flags.VeryCold != 0,
		VeryHot:               s.RawFlags2&flags.VeryHot != 0,
		GlideMode:             s.RawFlags2&flags.GlideMode != 0,
		OnFootInHangar:        s.RawFlags2&flags.OnFootInHangar != 0,
		OnFootSocialSpace:     s.RawFlags2&flags.OnFootSocialSpace != 0,
		OnFootExterior:        s.RawFlags2&flags.OnFootExterior != 0,
		BreathableAtmosphere:  s.RawFlags2&flags.BreathableAtmosphere != 0,
		TelepresenceMulticrew: s.RawFlags2&flags.TelepresenceMulticrew != 0,
		PhysicalMulticrew:     s.RawFlags2&flags.PhysicalMulticrew != 0,
		FSDHyperdriveCharging: s.RawFlags2&flags.FSDHyperdriveCharging != 0,
	}
}

// ActiveFlags returns names of all active Flags.
func (s *Status) ActiveFlags() []string {
	var active []string
	if s.Flags.Docked {
		active = append(active, "Docked")
	}
	if s.Flags.Landed {
		active = append(active, "Landed")
	}
	if s.Flags.LandingGearDown {
		active = append(active, "LandingGearDown")
	}
	if s.Flags.ShieldsUp {
		active = append(active, "ShieldsUp")
	}
	if s.Flags.Supercruise {
		active = append(active, "Supercruise")
	}
	if s.Flags.FlightAssistOff {
		active = append(active, "FAOff")
	}
	if s.Flags.HardpointsDeployed {
		active = append(active, "HardpointsDeployed")
	}
	if s.Flags.InWing {
		active = append(active, "InWing")
	}
	if s.Flags.LightsOn {
		active = append(active, "LightsOn")
	}
	if s.Flags.CargoScoopDeployed {
		active = append(active, "CargoScoopDeployed")
	}
	if s.Flags.SilentRunning {
		active = append(active, "SilentRunning")
	}
	if s.Flags.ScoopingFuel {
		active = append(active, "ScoopingFuel")
	}
	if s.Flags.SRVHandbrake {
		active = append(active, "SRVHandbrake")
	}
	if s.Flags.SRVTurret {
		active = append(active, "SRVTurret")
	}
	if s.Flags.SRVUnderShip {
		active = append(active, "SRVUnderShip")
	}
	if s.Flags.SRVDriveAssist {
		active = append(active, "SRVDriveAssist")
	}
	if s.Flags.FSDMassLocked {
		active = append(active, "FSDMassLocked")
	}
	if s.Flags.FSDCharging {
		active = append(active, "FSDCharging")
	}
	if s.Flags.FSDCooldown {
		active = append(active, "FSDCooldown")
	}
	if s.Flags.LowFuel {
		active = append(active, "LowFuel")
	}
	if s.Flags.Overheating {
		active = append(active, "Overheating")
	}
	if s.Flags.HasLatLong {
		active = append(active, "HasLatLong")
	}
	if s.Flags.IsInDanger {
		active = append(active, "IsInDanger")
	}
	if s.Flags.BeingInterdicted {
		active = append(active, "BeingInterdicted")
	}
	if s.Flags.InMainShip {
		active = append(active, "InMainShip")
	}
	if s.Flags.InFighter {
		active = append(active, "InFighter")
	}
	if s.Flags.InSRV {
		active = append(active, "InSRV")
	}
	if s.Flags.InAnalysisMode {
		active = append(active, "AnalysisMode")
	}
	if s.Flags.NightVision {
		active = append(active, "NightVision")
	}
	if s.Flags.AltitudeFromAverageRadius {
		active = append(active, "AltAvgRadius")
	}
	if s.Flags.FSDJump {
		active = append(active, "FSDJump")
	}
	if s.Flags.SRVHighBeam {
		active = append(active, "SRVHighBeam")
	}
	return active
}

// ActiveFlags2 returns names of all active Flags2 (Odyssey/on-foot).
func (s *Status) ActiveFlags2() []string {
	var active []string
	if s.Flags2.OnFoot {
		active = append(active, "OnFoot")
	}
	if s.Flags2.InTaxi {
		active = append(active, "InTaxi")
	}
	if s.Flags2.InMulticrew {
		active = append(active, "InMulticrew")
	}
	if s.Flags2.OnFootInStation {
		active = append(active, "OnFootInStation")
	}
	if s.Flags2.OnFootOnPlanet {
		active = append(active, "OnFootOnPlanet")
	}
	if s.Flags2.AimDownSight {
		active = append(active, "AimDownSight")
	}
	if s.Flags2.LowOxygen {
		active = append(active, "LowOxygen")
	}
	if s.Flags2.LowHealth {
		active = append(active, "LowHealth")
	}
	if s.Flags2.Cold {
		active = append(active, "Cold")
	}
	if s.Flags2.Hot {
		active = append(active, "Hot")
	}
	if s.Flags2.VeryCold {
		active = append(active, "VeryCold")
	}
	if s.Flags2.VeryHot {
		active = append(active, "VeryHot")
	}
	if s.Flags2.GlideMode {
		active = append(active, "GlideMode")
	}
	if s.Flags2.OnFootInHangar {
		active = append(active, "OnFootInHangar")
	}
	if s.Flags2.OnFootSocialSpace {
		active = append(active, "OnFootSocialSpace")
	}
	if s.Flags2.OnFootExterior {
		active = append(active, "OnFootExterior")
	}
	if s.Flags2.BreathableAtmosphere {
		active = append(active, "BreathableAtmosphere")
	}
	if s.Flags2.TelepresenceMulticrew {
		active = append(active, "TelepresenceMulticrew")
	}
	if s.Flags2.PhysicalMulticrew {
		active = append(active, "PhysicalMulticrew")
	}
	if s.Flags2.FSDHyperdriveCharging {
		active = append(active, "FSDHyperdriveCharging")
	}
	return active
}

// PipsSys returns system pips (each half-pip is 0.5).
func (s *Status) PipsSys() float64 {
	return float64(s.Pips[0]) / 2.0
}

// PipsEng returns engine pips.
func (s *Status) PipsEng() float64 {
	return float64(s.Pips[1]) / 2.0
}

// PipsWep returns weapon pips.
func (s *Status) PipsWep() float64 {
	return float64(s.Pips[2]) / 2.0
}

// GuiFocusName returns the human-readable name of GuiFocus.
func (s *Status) GuiFocusName() string {
	return flags.GuiFocusString(s.GuiFocus)
}

// ParsedTime returns the timestamp as time.Time.
func (s *Status) ParsedTime() (time.Time, error) {
	return time.Parse(time.RFC3339, s.Timestamp)
}

// Mode returns a general description of player mode (Ship, SRV, OnFoot, Fighter).
func (s *Status) Mode() string {
	if s.Flags2.OnFoot {
		return "On Foot"
	}
	if s.Flags.InSRV {
		return "SRV"
	}
	if s.Flags.InFighter {
		return "Fighter"
	}
	if s.Flags.InMainShip {
		return "Ship"
	}
	return "Unknown"
}

// Summary returns a formatted multiline summary of current status.
func (s *Status) Summary() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] Mode: %s | GUI: %s | FireGroup: %d\n", s.Timestamp, s.Mode(), s.GuiFocusName(), s.FireGroup))
	sb.WriteString(fmt.Sprintf("  Pips: [SYS: %.1f | ENG: %.1f | WEP: %.1f] | Fuel: Main %.2ft / Res %.2ft | Cargo: %.1ft\n",
		s.PipsSys(), s.PipsEng(), s.PipsWep(), s.Fuel.FuelMain, s.Fuel.FuelReservoir, s.Cargo))

	if s.LegalState != "" {
		sb.WriteString(fmt.Sprintf("  Legal State: %s", s.LegalState))
	}
	if s.Balance != nil {
		sb.WriteString(fmt.Sprintf(" | Balance: %d CR", *s.Balance))
	}
	if s.LegalState != "" || s.Balance != nil {
		sb.WriteString("\n")
	}

	if s.BodyName != "" || s.Latitude != nil || s.Altitude != nil {
		coords := []string{}
		if s.BodyName != "" {
			coords = append(coords, fmt.Sprintf("Body: %s", s.BodyName))
		}
		if s.Latitude != nil && s.Longitude != nil {
			coords = append(coords, fmt.Sprintf("Pos: (%.4f, %.4f)", *s.Latitude, *s.Longitude))
		}
		if s.Altitude != nil {
			coords = append(coords, fmt.Sprintf("Alt: %.0fm", *s.Altitude))
		}
		if s.Heading != nil {
			coords = append(coords, fmt.Sprintf("Heading: %.0f°", *s.Heading))
		}
		sb.WriteString(fmt.Sprintf("  Location: %s\n", strings.Join(coords, " | ")))
	}

	if s.Destination != nil && s.Destination.Name != "" {
		sb.WriteString(fmt.Sprintf("  Destination: %s (Body: %d, System: %d)\n", s.Destination.Name, s.Destination.Body, s.Destination.System))
	}

	if s.Flags2.OnFoot {
		var footInfo []string
		if s.Oxygen != nil {
			footInfo = append(footInfo, fmt.Sprintf("O2: %.0f%%", *s.Oxygen*100))
		}
		if s.Health != nil {
			footInfo = append(footInfo, fmt.Sprintf("Health: %.0f%%", *s.Health*100))
		}
		if s.Temperature != nil {
			footInfo = append(footInfo, fmt.Sprintf("Temp: %.1fK", *s.Temperature))
		}
		if s.Gravity != nil {
			footInfo = append(footInfo, fmt.Sprintf("Gravity: %.2fG", *s.Gravity))
		}
		weapon := s.SelectedWeaponLocalised
		if weapon == "" {
			weapon = s.SelectedWeapon
		}
		if weapon != "" {
			footInfo = append(footInfo, fmt.Sprintf("Weapon: %s", weapon))
		}
		if len(footInfo) > 0 {
			sb.WriteString(fmt.Sprintf("  On-Foot: %s\n", strings.Join(footInfo, " | ")))
		}
	}

	activeFlags := s.ActiveFlags()
	if len(activeFlags) > 0 {
		sb.WriteString(fmt.Sprintf("  Flags: %s\n", strings.Join(activeFlags, ", ")))
	}
	activeFlags2 := s.ActiveFlags2()
	if len(activeFlags2) > 0 {
		sb.WriteString(fmt.Sprintf("  Flags2: %s\n", strings.Join(activeFlags2, ", ")))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// Parse takes byte contents of Status.json and unmarshals it into a Status struct.
func Parse(data []byte) (*Status, error) {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 {
		return nil, errors.New("empty status content")
	}

	var status Status
	if err := json.Unmarshal([]byte(trimmed), &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status JSON: %w", err)
	}

	status.ExpandFlags()
	return &status, nil
}

// ParseFile reads a file and parses its contents as Status.
func ParseFile(filePath string) (*Status, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read status file: %w", err)
	}
	return Parse(data)
}
