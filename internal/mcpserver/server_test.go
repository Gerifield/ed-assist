package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ed-assist/internal/edapi"
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

func TestMCPServerExternalAPITools(t *testing.T) {
	tsEDSM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/api-v1/system"):
			fmt.Fprint(w, `{"name":"Sol","coords":{"x":0,"y":0,"z":0},"information":{"allegiance":"Federation","population":18000000000}}`)
		case strings.Contains(path, "/api-v1/sphere-systems"):
			fmt.Fprint(w, `[{"distance":4.38,"name":"Alpha Centauri","coords":{"x":3,"y":0,"z":3},"information":{"population":100000}}]`)
		case strings.Contains(path, "/stations/market"):
			fmt.Fprint(w, `{"id":1,"name":"Daedalus","commodities":[{"id":"tritium","name":"Tritium","buyPrice":50000,"sellPrice":49000}]}`)
		case strings.Contains(path, "/stations"):
			fmt.Fprint(w, `{"name":"Sol","stations":[{"name":"Daedalus","type":"Coriolis Starport"}]}`)
		case strings.Contains(path, "/factions"):
			fmt.Fprint(w, `{"name":"Sol","factions":[{"name":"Mother Gaia","influence":0.31}]}`)
		case strings.Contains(path, "/bodies"):
			fmt.Fprint(w, `{"name":"Sol","bodies":[{"name":"Earth","type":"Planet","isLandable":false}]}`)
		case strings.Contains(path, "/elite-server"):
			fmt.Fprint(w, `{"status":1,"message":"Good"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer tsEDSM.Close()

	tsSpansh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/route") {
			fmt.Fprint(w, `{"job":"job-abc","status":"queued"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/results/job-abc") {
			fmt.Fprint(w, `{"status":"completed","result":{"distance":22000,"system_jumps":[{"system":"Sol"},{"system":"Colonia"}]}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer tsSpansh.Close()

	edClient := edapi.NewClient(
		edapi.WithBaseEDSMURL(tsEDSM.URL),
		edapi.WithBaseSpanshURL(tsSpansh.URL),
		edapi.WithCacheTTL(8*time.Hour),
	)

	st := makeSampleStatus()
	mock := &mockProvider{status: st}
	s := New(mock, nil, nil, WithEDAPIClient(edClient))
	ctx := context.Background()

	// 1. search_system (omitted name defaults to destination "Sol")
	res, err := s.server.GetTool("search_system").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "search_system"},
	})
	if err != nil || res.IsError {
		t.Fatalf("search_system failed: %v, res: %v", err, res)
	}

	// 2. nearest_systems
	res, err = s.server.GetTool("nearest_systems").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "nearest_systems",
			Arguments: map[string]any{
				"system_name": "Sol",
				"radius":      20.0,
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("nearest_systems failed: %v, res: %v", err, res)
	}

	// 3. system_stations
	res, err = s.server.GetTool("system_stations").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "system_stations"},
	})
	if err != nil || res.IsError {
		t.Fatalf("system_stations failed: %v, res: %v", err, res)
	}

	// 4. station_market
	res, err = s.server.GetTool("station_market").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "station_market",
			Arguments: map[string]any{
				"system_name":      "Sol",
				"station_name":     "Daedalus",
				"filter_commodity": "tritium",
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("station_market failed: %v, res: %v", err, res)
	}

	// 5. system_factions
	res, err = s.server.GetTool("system_factions").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "system_factions"},
	})
	if err != nil || res.IsError {
		t.Fatalf("system_factions failed: %v, res: %v", err, res)
	}

	// 6. system_bodies
	res, err = s.server.GetTool("system_bodies").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "system_bodies"},
	})
	if err != nil || res.IsError {
		t.Fatalf("system_bodies failed: %v, res: %v", err, res)
	}

	// 7. plot_neutron_route
	res, err = s.server.GetTool("plot_neutron_route").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "plot_neutron_route",
			Arguments: map[string]any{
				"from":  "Sol",
				"to":    "Colonia",
				"range": 50.0,
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("plot_neutron_route failed: %v, res: %v", err, res)
	}

	// 8. get_server_status
	res, err = s.server.GetTool("get_server_status").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "get_server_status"},
	})
	if err != nil || res.IsError {
		t.Fatalf("get_server_status failed: %v, res: %v", err, res)
	}
}

func TestResolveSystemNameNilDestination(t *testing.T) {
	// Status without a targeted destination (Destination is nil)
	st := makeSampleStatus()
	st.Destination = nil
	mock := &mockProvider{status: st}

	s := New(mock, nil, nil)
	// Must not panic when Destination is nil
	resolved := s.resolveSystemName("")
	if resolved != "" {
		t.Errorf("expected empty string when no destination and no store, got %q", resolved)
	}

	// Tool call search_system without system_name should cleanly return an error result, not panic
	ctx := context.Background()
	res, err := s.server.GetTool("search_system").Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "search_system"},
	})
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected tool result error when system cannot be resolved")
	}

	// 2. Test fallback to visited system from store
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dbStore, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed initializing test store: %v", err)
	}
	defer dbStore.Close()

	_ = dbStore.RecordVisited(store.VisitedSystem{
		SystemName:    "Alpha Centauri",
		SystemAddress: 11112222,
		VisitedAt:     time.Now().UTC(),
	})

	sWithStore := New(mock, dbStore, nil)
	resolvedVisited := sWithStore.resolveSystemName("")
	if resolvedVisited != "Alpha Centauri" {
		t.Errorf("expected Alpha Centauri from visited history, got %q", resolvedVisited)
	}

	// 3. Test fallback to journal file
	journalDir := t.TempDir()
	journalFile := filepath.Join(journalDir, "Journal.2026-09-22T000000.01.log")
	journalContent := `{"timestamp":"2026-09-22T00:10:00Z","event":"Location","StarSystem":"Shinrarta Dezhra"}
`
	if err := os.WriteFile(journalFile, []byte(journalContent), 0644); err != nil {
		t.Fatalf("failed writing test journal: %v", err)
	}

	statusMock := &mockFilePathProvider{
		mockProvider: mockProvider{status: st},
		filePath:     filepath.Join(journalDir, "Status.json"),
	}
	sWithJournal := New(statusMock, nil, nil)
	resolvedJournal := sWithJournal.resolveSystemName("")
	if resolvedJournal != "Shinrarta Dezhra" {
		t.Errorf("expected Shinrarta Dezhra from journal fallback, got %q", resolvedJournal)
	}
}

type mockFilePathProvider struct {
	mockProvider
	filePath string
}

func (m *mockFilePathProvider) FilePath() string {
	return m.filePath
}

type mockVehicleProvider struct {
	mode      string
	event     string
	timestamp time.Time
	ok        bool
}

func (m *mockVehicleProvider) LatestVehicleState() (string, string, time.Time, bool) {
	return m.mode, m.event, m.timestamp, m.ok
}

func TestMCPServerJournalVehicleModeOverride(t *testing.T) {
	// Status.json says player is in an SRV at 16:00:00Z
	statusJSON := `{
		"timestamp": "2026-09-24T16:00:00Z",
		"event": "Status",
		"Flags": 67108864,
		"Pips": [4, 4, 4]
	}`
	st, err := parser.Parse([]byte(statusJSON))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	p := &mockProvider{status: st}

	// 1. Without vehicle provider, mode is SRV
	sNoVP := New(p, nil, nil)
	gotStatus, err := sNoVP.getStatus()
	if err != nil || gotStatus.Mode() != "SRV" {
		t.Fatalf("expected initial mode SRV, got %s (err: %v)", gotStatus.Mode(), err)
	}

	// 2. With vehicle provider reporting DockSRV, mode MUST be Ship even if Status.json updates later
	dockTime, _ := time.Parse(time.RFC3339, "2026-09-24T16:05:00Z")
	vp := &mockVehicleProvider{
		mode:      "Ship",
		event:     "DockSRV",
		timestamp: dockTime,
		ok:        true,
	}

	sWithVP := New(p, nil, nil, WithVehicleProvider(vp))
	gotOverridden, err := sWithVP.getStatus()
	if err != nil {
		t.Fatalf("getStatus failed: %v", err)
	}
	if gotOverridden.Mode() != "Ship" {
		t.Errorf("expected mode to be overridden to 'Ship', got %s", gotOverridden.Mode())
	}

	// 3. With vehicle provider reporting Nomad, mode MUST be Nomad
	vpNomad := &mockVehicleProvider{
		mode:      "Nomad",
		event:     "LaunchVessel",
		timestamp: dockTime,
		ok:        true,
	}
	sWithVPNomad := New(p, nil, nil, WithVehicleProvider(vpNomad))
	gotNomad, err := sWithVPNomad.getStatus()
	if err != nil {
		t.Fatalf("getStatus failed: %v", err)
	}
	if gotNomad.Mode() != "Nomad" {
		t.Errorf("expected mode to be overridden to 'Nomad', got %s", gotNomad.Mode())
	}
}


