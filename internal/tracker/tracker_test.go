package tracker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ed-assist/internal/parser"
	"ed-assist/internal/store"
)

func TestTrackerTargetedFromStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	statusPath := filepath.Join(tmpDir, "Status.json")

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer st.Close()

	tr := New(st, statusPath)

	parsedStatus := &parser.Status{
		Timestamp: "2024-03-01T12:00:00Z",
		Destination: &parser.Destination{
			System: 1234567,
			Body:   1,
			Name:   "Sol",
		},
	}

	tr.ProcessStatus(parsedStatus)

	targeted, err := st.GetTargeted(10)
	if err != nil {
		t.Fatalf("failed getting targeted: %v", err)
	}
	if len(targeted) != 1 {
		t.Fatalf("expected 1 targeted system, got %d", len(targeted))
	}
	if targeted[0].SystemName != "Sol" {
		t.Errorf("expected Sol, got %s", targeted[0].SystemName)
	}
	if targeted[0].SystemAddress != 1234567 {
		t.Errorf("expected 1234567, got %d", targeted[0].SystemAddress)
	}
}

func TestTrackerVisitedFromJournal(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	statusPath := filepath.Join(tmpDir, "Status.json")
	journalFile := filepath.Join(tmpDir, "Journal.2024-03-01T120000.01.log")

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer st.Close()

	// Write initial journal entries
	initialEntries := `{"timestamp":"2024-03-01T12:00:00Z", "event":"Location", "StarSystem":"Sol", "SystemAddress":1234567, "StarPos":[0.0,0.0,0.0]}
{"timestamp":"2024-03-01T12:05:00Z", "event":"FSDJump", "StarSystem":"Alpha Centauri", "SystemAddress":7654321, "StarPos":[3.0,1.0,-2.0], "JumpDist":4.37}
`
	if err := os.WriteFile(journalFile, []byte(initialEntries), 0644); err != nil {
		t.Fatalf("failed writing journal file: %v", err)
	}

	tr := New(st, statusPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tr.Start(ctx)

	// Give tracker moment to read initial journal
	time.Sleep(100 * time.Millisecond)

	visited, err := st.GetVisited(10)
	if err != nil {
		t.Fatalf("failed getting visited systems: %v", err)
	}
	if len(visited) != 2 {
		t.Fatalf("expected 2 visited systems, got %d", len(visited))
	}
	if visited[0].SystemName != "Alpha Centauri" {
		t.Errorf("expected Alpha Centauri first, got %s", visited[0].SystemName)
	}
	if visited[1].SystemName != "Sol" {
		t.Errorf("expected Sol second, got %s", visited[1].SystemName)
	}

	// Append a new jump while tracker is running
	newJump := `{"timestamp":"2024-03-01T12:10:00Z", "event":"FSDJump", "StarSystem":"Barnard's Star", "SystemAddress":998877, "StarPos":[1.0,2.0,3.0], "JumpDist":5.9}
`
	f, err := os.OpenFile(journalFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed opening journal file for append: %v", err)
	}
	_, _ = f.WriteString(newJump)
	f.Close()

	// Wait for tracker to detect update
	time.Sleep(300 * time.Millisecond)

	visited, err = st.GetVisited(10)
	if err != nil {
		t.Fatalf("failed getting visited systems: %v", err)
	}
	if len(visited) != 3 {
		t.Fatalf("expected 3 visited systems, got %d", len(visited))
	}
	if visited[0].SystemName != "Barnard's Star" {
		t.Errorf("expected Barnard's Star, got %s", visited[0].SystemName)
	}
}
