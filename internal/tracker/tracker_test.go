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

func TestTrackerBackfillRestartDoesNotDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	statusPath := filepath.Join(tmpDir, "Status.json")
	journalFile := filepath.Join(tmpDir, "Journal.2024-03-01T120000.01.log")

	// 1. Setup initial journal
	initialEntries := `{"timestamp":"2024-03-01T12:00:00Z", "event":"Location", "StarSystem":"Sol", "SystemAddress":1234567, "StarPos":[0.0,0.0,0.0]}
{"timestamp":"2024-03-01T12:05:00Z", "event":"FSDJump", "StarSystem":"Alpha Centauri", "SystemAddress":7654321, "StarPos":[3.0,1.0,-2.0], "JumpDist":4.37}
`
	if err := os.WriteFile(journalFile, []byte(initialEntries), 0644); err != nil {
		t.Fatalf("failed writing journal file: %v", err)
	}

	// 2. First run of application
	st1, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store 1: %v", err)
	}
	tr1 := New(st1, statusPath)

	ctx1, cancel1 := context.WithCancel(context.Background())
	go tr1.Start(ctx1)
	time.Sleep(100 * time.Millisecond)
	cancel1()

	visited1, err := st1.GetVisited(10)
	if err != nil || len(visited1) != 2 {
		t.Fatalf("expected 2 visited systems on run 1, got %d (err: %v)", len(visited1), err)
	}
	st1.Close()

	// 3. Second run (simulate restart of application)
	st2, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store 2: %v", err)
	}
	defer st2.Close()

	tr2 := New(st2, statusPath)
	ctx2, cancel2 := context.WithCancel(context.Background())
	go tr2.Start(ctx2)
	time.Sleep(100 * time.Millisecond)

	visited2, err := st2.GetVisited(10)
	if err != nil || len(visited2) != 2 {
		t.Fatalf("expected STILL 2 visited systems after restart (no duplicate backfill), got %d (err: %v)", len(visited2), err)
	}

	// 4. Append new jump during run 2
	newJump := `{"timestamp":"2024-03-01T12:15:00Z", "event":"FSDJump", "StarSystem":"Sirius", "SystemAddress":554433, "StarPos":[1.0,2.0,3.0], "JumpDist":8.6}
`
	f, err := os.OpenFile(journalFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed opening journal file for append: %v", err)
	}
	_, _ = f.WriteString(newJump)
	f.Close()

	time.Sleep(300 * time.Millisecond)
	cancel2()

	visited3, err := st2.GetVisited(10)
	if err != nil || len(visited3) != 3 {
		t.Fatalf("expected 3 visited systems after new jump, got %d (err: %v)", len(visited3), err)
	}
	if visited3[0].SystemName != "Sirius" {
		t.Errorf("expected newest to be Sirius, got %s", visited3[0].SystemName)
	}
}

func TestTrackerVehicleEvents(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	statusPath := filepath.Join(tmpDir, "Status.json")

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer st.Close()

	tr := New(st, statusPath)

	// Initially no vehicle state
	_, _, _, ok := tr.LatestVehicleState()
	if ok {
		t.Errorf("expected no initial vehicle state")
	}

	// 1. Process LaunchSRV event
	tr.handleJournalLine([]byte(`{"timestamp":"2026-09-24T16:00:00Z", "event":"LaunchSRV", "PlayerControlled":true}`))
	mode, event, ts, ok := tr.LatestVehicleState()
	if !ok || mode != "SRV" || event != "LaunchSRV" {
		t.Fatalf("expected SRV mode, got mode=%s, event=%s", mode, event)
	}
	expectedTime, _ := time.Parse(time.RFC3339, "2026-09-24T16:00:00Z")
	if !ts.Equal(expectedTime) {
		t.Errorf("timestamp mismatch: got %v", ts)
	}

	// 2. Process older event (should not overwrite newer)
	tr.handleJournalLine([]byte(`{"timestamp":"2026-09-24T15:50:00Z", "event":"LaunchFighter"}`))
	mode, _, ts, _ = tr.LatestVehicleState()
	if mode != "SRV" || !ts.Equal(expectedTime) {
		t.Errorf("older event incorrectly overwrote newer vehicle state")
	}

	// 3. Process DockSRV event (returning to ship)
	tr.handleJournalLine([]byte(`{"timestamp":"2026-09-24T16:05:00Z", "event":"DockSRV"}`))
	mode, event, ts, ok = tr.LatestVehicleState()
	if !ok || mode != "Ship" || event != "DockSRV" {
		t.Fatalf("expected Ship mode after DockSRV, got mode=%s, event=%s", mode, event)
	}
	dockTime, _ := time.Parse(time.RFC3339, "2026-09-24T16:05:00Z")
	if !ts.Equal(dockTime) {
		t.Errorf("expected dockTime 16:05:00, got %v", ts)
	}
}

