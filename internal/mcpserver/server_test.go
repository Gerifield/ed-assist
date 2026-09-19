package mcpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ed-assist/internal/input"
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

type mockKeySender struct{}

func (m *mockKeySender) SendKey(scanCode input.ScanCode, holdDuration time.Duration, modifiers ...input.ScanCode) error {
	return nil
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

	ctrl := input.NewController("", 80, &mockKeySender{})
	reg := input.NewBindsRegistry()
	_ = reg.ParseReader(strings.NewReader(`<?xml version="1.0" encoding="UTF-8" ?>
<Root PresetName="Custom">
	<LandingGearToggle>
		<Primary Device="Keyboard" Key="Key_L" />
	</LandingGearToggle>
</Root>`))
	ctrl.SetRegistry(reg)

	st := makeSampleStatus()
	mock := &mockProvider{status: st}
	s := New(mock, dbStore, ctrl)

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
		"list_game_commands",
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

	// Test send_game_command tool
	sendTool := s.server.GetTool("send_game_command")
	if sendTool == nil {
		t.Fatalf("send_game_command tool not registered")
	}
	sendRes, err := sendTool.Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "send_game_command",
			Arguments: map[string]interface{}{
				"action": "landing_gear",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected send_game_command error: %v", err)
	}
	if sendRes.IsError {
		t.Fatalf("expected send_game_command success, got error: %+v", sendRes)
	}
}

func TestMCPServerNoStatus(t *testing.T) {
	mock := &mockProvider{status: nil, err: errors.New("file not found")}
	s := New(mock, nil, nil)

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

func TestMCPServerTrackingDisabled(t *testing.T) {
	st := makeSampleStatus()
	mock := &mockProvider{status: st}
	s := New(mock, nil, nil)

	ctx := context.Background()

	for _, toolName := range []string{"get_visited_systems", "get_targeted_systems"} {
		tool := s.server.GetTool(toolName)
		if tool == nil {
			t.Fatalf("tool %s not registered", toolName)
		}
		res, err := tool.Handler(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: toolName},
		})
		if err != nil {
			t.Fatalf("unexpected call error: %v", err)
		}
		if !res.IsError {
			t.Errorf("expected error result when tracking is disabled for %s", toolName)
		}
		tc, ok := mcp.AsTextContent(res.Content[0])
		if !ok || !strings.Contains(tc.Text, "database tracking is not enabled") {
			t.Errorf("expected disabled message, got %v", res.Content[0])
		}
	}
}

func TestMCPServerGameControlDisabled(t *testing.T) {
	st := makeSampleStatus()
	mock := &mockProvider{status: st}
	s := New(mock, nil, nil)

	ctx := context.Background()

	for _, toolName := range []string{"send_game_command", "list_game_commands"} {
		tool := s.server.GetTool(toolName)
		if tool == nil {
			t.Fatalf("tool %s not registered", toolName)
		}
		res, err := tool.Handler(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: toolName},
		})
		if err != nil {
			t.Fatalf("unexpected call error: %v", err)
		}
		if !res.IsError {
			t.Errorf("expected error result when control is disabled for %s", toolName)
		}
		tc, ok := mcp.AsTextContent(res.Content[0])
		if !ok || !strings.Contains(tc.Text, "game control is not enabled") {
			t.Errorf("expected disabled message, got %v", res.Content[0])
		}
	}
}

func TestMCPServerServeHTTP(t *testing.T) {
	st := makeSampleStatus()
	mock := &mockProvider{status: st}
	s := New(mock, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get a free random local port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.ServeHTTP(ctx, addr)
	}()

	// Wait briefly for server to start listening
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed HTTP GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("unexpected server error on shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown timed out")
	}
}
