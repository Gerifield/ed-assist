package parser

import (
	"testing"
)

func TestParseShipStatus(t *testing.T) {
	rawJSON := `{
		"timestamp": "2017-12-07T10:31:37Z",
		"event": "Status",
		"Flags": 16842765,
		"Pips": [2, 8, 2],
		"FireGroup": 0,
		"Fuel": {
			"FuelMain": 15.146626,
			"FuelReservoir": 0.382796
		},
		"GuiFocus": 5
	}`

	status, err := Parse([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error parsing status: %v", err)
	}

	if status.Timestamp != "2017-12-07T10:31:37Z" {
		t.Errorf("expected timestamp '2017-12-07T10:31:37Z', got '%s'", status.Timestamp)
	}

	if status.Event != "Status" {
		t.Errorf("expected event 'Status', got '%s'", status.Event)
	}

	// 16842765 = 0x0101000d -> Docked (0), LandingGearDown (2), ShieldsUp (3), FSDMassLocked (16), InMainShip (24)
	if !status.Flags.Docked {
		t.Errorf("expected Docked to be true")
	}
	if !status.Flags.LandingGearDown {
		t.Errorf("expected LandingGearDown to be true")
	}
	if !status.Flags.ShieldsUp {
		t.Errorf("expected ShieldsUp to be true")
	}
	if !status.Flags.FSDMassLocked {
		t.Errorf("expected FSDMassLocked to be true")
	}
	if !status.Flags.InMainShip {
		t.Errorf("expected InMainShip to be true")
	}
	if status.Flags.Supercruise {
		t.Errorf("expected Supercruise to be false")
	}

	if status.PipsSys() != 1.0 {
		t.Errorf("expected SYS pips 1.0, got %f", status.PipsSys())
	}
	if status.PipsEng() != 4.0 {
		t.Errorf("expected ENG pips 4.0, got %f", status.PipsEng())
	}
	if status.PipsWep() != 1.0 {
		t.Errorf("expected WEP pips 1.0, got %f", status.PipsWep())
	}

	if status.GuiFocusName() != "Station Services" {
		t.Errorf("expected 'Station Services', got '%s'", status.GuiFocusName())
	}

	if status.Mode() != "Ship" {
		t.Errorf("expected mode 'Ship', got '%s'", status.Mode())
	}

	summary := status.Summary()
	if len(summary) == 0 {
		t.Errorf("expected non-empty summary")
	}
}

func TestParseSurfaceAndOdyssey(t *testing.T) {
	rawJSON := `{
		"timestamp": "2024-03-01T12:00:00Z",
		"event": "Status",
		"Flags": 2097152,
		"Flags2": 1,
		"Pips": [0, 0, 0],
		"FireGroup": 1,
		"GuiFocus": 0,
		"Latitude": -28.584963,
		"Longitude": 6.826313,
		"Heading": 109.0,
		"Altitude": 404.0,
		"BodyName": "Achenar 3",
		"Balance": 125000000,
		"LegalState": "Clean",
		"Oxygen": 0.95,
		"Health": 1.0,
		"Temperature": 288.15,
		"SelectedWeapon": "$wpn_m_assaultrifle_plasma_name;",
		"SelectedWeapon_Localised": "Karma AR-50",
		"Gravity": 0.98,
		"Destination": {
			"System": 123456789,
			"Body": 2,
			"Name": "Outpost Station"
		}
	}`

	status, err := Parse([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error parsing status: %v", err)
	}

	if !status.Flags.HasLatLong {
		t.Errorf("expected HasLatLong to be true")
	}
	if !status.Flags2.OnFoot {
		t.Errorf("expected OnFoot to be true")
	}
	if status.Mode() != "On Foot" {
		t.Errorf("expected mode 'On Foot', got '%s'", status.Mode())
	}

	if status.Latitude == nil || *status.Latitude != -28.584963 {
		t.Errorf("latitude mismatch")
	}
	if status.BodyName != "Achenar 3" {
		t.Errorf("body name mismatch")
	}
	if status.Destination == nil || status.Destination.Name != "Outpost Station" {
		t.Errorf("destination mismatch")
	}
	if status.SelectedWeaponLocalised != "Karma AR-50" {
		t.Errorf("weapon mismatch")
	}
}

func TestParseEmptyOrInvalid(t *testing.T) {
	_, err := Parse([]byte(""))
	if err == nil {
		t.Errorf("expected error for empty byte slice")
	}

	_, err = Parse([]byte("{invalid-json"))
	if err == nil {
		t.Errorf("expected error for invalid json")
	}
}
