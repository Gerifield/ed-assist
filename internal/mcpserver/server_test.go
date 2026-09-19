package mcpserver

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ed-assist/internal/parser"
	"ed-assist/internal/store"

	"github.com/mark3labs/mcp-go/mcp"
)

type mockProvider struct {
	status *parser.Status
	err    error
}

func (m *mockProvider) LastStatus() *parser.Status {
	return m.status
}

func (m *mockProvider) ReadOnce() (*parser.Status, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.status, nil
}

func makeSampleStatus() *parser.Status {
	rawJSON := `{
		"timestamp": "2024-03-01T12:00:00Z",
		"event": "Status",
		"Flags": 16842765,
		"Flags2": 1,
		"Pips": [4, 4, 4],
		"FireGroup": 0,
		"Fuel": {
			"FuelMain": 32.0,
			"FuelReservoir": 0.85
		},
		"Cargo": 12.5,
		"GuiFocus": 5,
		"LegalState": "Clean",
		"Balance": 542000100,
		"Latitude": -28.58,
		"Longitude": 6.82,
		"Altitude": 404.0,
		"Heading": 109.0,
		"BodyName": "Earth",
		"Destination": {
			"System": 12345678,
			"Body": 1,
			"Name": "Sol"
		},
		"Oxygen": 0.95,
		"Health": 1.0,
		"Temperature": 288.15,
		"SelectedWeapon_Localised": "Karma AR-50",
		"Gravity": 1.0
	}`
	st, _ := parser.Parse([]byte(rawJSON))
	return st
}

func TestMCPServerTools(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dbStore, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed initializing db store: %v", err)
	}
	defer dbStore.Close()

	_ = dbStore.RecordVisited(store.VisitedSystem{
		SystemName:    "Sol",
		SystemAddress: 12345678,
		VisitedAt:     time.Now().UTC(),
	})
	_ = dbStore.RecordTargeted(store.TargetedSystem{
		SystemName:    "Alpha Centauri",
		SystemAddress: 87654321,
		TargetedAt:    time.Now().UTC(),
	})

	st := makeSampleStatus()
	mock := &mockProvider{status: st}
	s := New(mock, dbStore)

	ctx := context.Background()

	toolNames := []string{
		"get_status",
		"get_status_summary",
		"get_ship_status",
		"get_cockpit",
		"get_navigation",
		"get_on_foot",
		"get_visited_systems",
		"get_targeted_systems",
	}

	for _, name := range toolNames {
		st := s.server.GetTool(name)
		if st == nil {
			t.Fatalf("tool %s not registered", name)
		}

		res, err := st.Handler(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: name,
			},
		})
		if err != nil {
			t.Fatalf("tool %s returned Go error: %v", name, err)
		}
		if res == nil {
			t.Fatalf("tool %s returned nil result", name)
		}
		if res.IsError {
			t.Fatalf("tool %s returned error result: %+v", name, res)
		}
		if len(res.Content) == 0 {
			t.Fatalf("tool %s returned empty content", name)
		}
	}
}

func TestMCPServerNoStatus(t *testing.T) {
	mock := &mockProvider{status: nil, err: errors.New("file not found")}
	s := New(mock, nil)

	ctx := context.Background()
	st := s.server.GetTool("get_status")
	if st == nil {
		t.Fatal("get_status tool not registered")
	}

	res, err := st.Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_status",
		},
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result when status is unavailable")
	}

	firstContent := res.Content[0]
	if tc, ok := mcp.AsTextContent(firstContent); !ok || !strings.Contains(tc.Text, "not yet available") {
		t.Errorf("unexpected error text: %+v", firstContent)
	}
}
