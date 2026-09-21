package web

import (
	"context"
	"path/filepath"
	"testing"

	"ed-assist/internal/input"
	"ed-assist/internal/mcpserver"
	"ed-assist/internal/parser"
	"ed-assist/internal/store"
)

type mockStatusProvider struct {
	status *parser.Status
}

func (m *mockStatusProvider) LastStatus() *parser.Status {
	return m.status
}

func (m *mockStatusProvider) ReadOnce() (*parser.Status, error) {
	return m.status, nil
}

func TestInProcessBridge(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer st.Close()

	provider := &mockStatusProvider{
		status: &parser.Status{
			Timestamp: "2024-03-01T12:00:00Z",
			Cargo:     12.5,
		},
	}
	ctrl := input.NewController("", 80, input.NewKeySender())

	mcpSrv := mcpserver.New(provider, st, ctrl)

	bridge, err := NewInProcessBridge(mcpSrv.Server())
	if err != nil {
		t.Fatalf("failed creating in-process bridge: %v", err)
	}
	defer bridge.Close()

	if bridge.Mode() != "inprocess" {
		t.Errorf("expected mode inprocess, got %s", bridge.Mode())
	}

	ctx := context.Background()
	tools, err := bridge.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed listing tools: %v", err)
	}

	if len(tools) == 0 {
		t.Fatalf("expected non-empty tool list from in-process bridge")
	}

	// Test calling get_cockpit
	res, err := bridge.CallTool(ctx, "get_cockpit", map[string]any{})
	if err != nil {
		t.Fatalf("failed calling get_cockpit: %v", err)
	}
	if res == "" {
		t.Errorf("expected non-empty tool output")
	}
}
