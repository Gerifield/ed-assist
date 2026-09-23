package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreVisitedAndTargeted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer s.Close()

	posX := 0.0
	posY := 0.0
	posZ := 0.0

	// 1. Test record visited
	v1 := VisitedSystem{
		SystemName:    "Sol",
		SystemAddress: 1234567,
		StarPosX:      &posX,
		StarPosY:      &posY,
		StarPosZ:      &posZ,
		Allegiance:    "Federation",
		Economy:       "Refinery",
		Government:    "Democracy",
		VisitedAt:     time.Now().UTC(),
	}
	if err := s.RecordVisited(v1); err != nil {
		t.Fatalf("failed recording visited system: %v", err)
	}

	// Immediate duplicate insertion should be skipped
	if err := s.RecordVisited(v1); err != nil {
		t.Fatalf("unexpected error on duplicate: %v", err)
	}

	// Insert second system
	v2 := VisitedSystem{
		SystemName:    "Alpha Centauri",
		SystemAddress: 7654321,
		VisitedAt:     time.Now().UTC().Add(time.Minute),
	}
	if err := s.RecordVisited(v2); err != nil {
		t.Fatalf("failed recording visited system 2: %v", err)
	}

	visited, err := s.GetVisited(10)
	if err != nil {
		t.Fatalf("failed getting visited systems: %v", err)
	}
	if len(visited) != 2 {
		t.Fatalf("expected 2 visited systems, got %d", len(visited))
	}
	if visited[0].SystemName != "Alpha Centauri" {
		t.Errorf("expected most recent to be Alpha Centauri, got %s", visited[0].SystemName)
	}

	// 2. Test record targeted
	t1 := TargetedSystem{
		SystemName:    "Epsilon Indi",
		SystemAddress: 998877,
		BodyID:        2,
		TargetedAt:    time.Now().UTC(),
	}
	if err := s.RecordTargeted(t1); err != nil {
		t.Fatalf("failed recording targeted system: %v", err)
	}

	targeted, err := s.GetTargeted(10)
	if err != nil {
		t.Fatalf("failed getting targeted systems: %v", err)
	}
	if len(targeted) != 1 {
		t.Fatalf("expected 1 targeted system, got %d", len(targeted))
	}
	if targeted[0].SystemName != "Epsilon Indi" {
		t.Errorf("expected targeted to be Epsilon Indi, got %s", targeted[0].SystemName)
	}
}

func TestStorePruningMax100(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_prune.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer s.Close()

	// Insert 120 visited systems
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 120; i++ {
		err := s.RecordVisited(VisitedSystem{
			SystemName:    fmt.Sprintf("System-%03d", i),
			SystemAddress: int64(1000 + i),
			VisitedAt:     baseTime.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("failed inserting system %d: %v", i, err)
		}
	}

	visited, err := s.GetVisited(200)
	if err != nil {
		t.Fatalf("failed getting visited systems: %v", err)
	}
	if len(visited) != 100 {
		t.Fatalf("expected exactly 100 visited systems due to pruning, got %d", len(visited))
	}
	// The oldest remaining should be System-021 (since 1..20 pruned)
	if visited[len(visited)-1].SystemName != "System-021" {
		t.Errorf("expected oldest to be System-021, got %s", visited[len(visited)-1].SystemName)
	}
	// The newest should be System-120
	if visited[0].SystemName != "System-120" {
		t.Errorf("expected newest to be System-120, got %s", visited[0].SystemName)
	}

	// Insert 120 targeted systems
	for i := 1; i <= 120; i++ {
		err := s.RecordTargeted(TargetedSystem{
			SystemName:    fmt.Sprintf("Target-%03d", i),
			SystemAddress: int64(5000 + i),
			TargetedAt:    baseTime.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("failed inserting target %d: %v", i, err)
		}
	}

	targeted, err := s.GetTargeted(200)
	if err != nil {
		t.Fatalf("failed getting targeted systems: %v", err)
	}
	if len(targeted) != 100 {
		t.Fatalf("expected exactly 100 targeted systems due to pruning, got %d", len(targeted))
	}
}

func TestStoreDuplicateTimestampSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_dedup.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer s.Close()

	visitTime := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	v1 := VisitedSystem{
		SystemName:    "Sol",
		SystemAddress: 12345,
		VisitedAt:     visitTime,
	}

	if err := s.RecordVisited(v1); err != nil {
		t.Fatalf("failed recording initial visit: %v", err)
	}

	// Insert intermediate visit
	v2 := VisitedSystem{
		SystemName:    "Alpha Centauri",
		SystemAddress: 67890,
		VisitedAt:     visitTime.Add(time.Hour),
	}
	if err := s.RecordVisited(v2); err != nil {
		t.Fatalf("failed recording second visit: %v", err)
	}

	// Now try to re-record v1 (e.g. from backfill re-reading journal)
	if err := s.RecordVisited(v1); err != nil {
		t.Fatalf("unexpected error recording duplicate: %v", err)
	}

	visited, err := s.GetVisited(10)
	if err != nil {
		t.Fatalf("failed getting visited systems: %v", err)
	}
	if len(visited) != 2 {
		t.Fatalf("expected 2 visited systems (duplicate skipped), got %d", len(visited))
	}

	latestTime := s.GetLatestVisitedTime()
	if !latestTime.Equal(v2.VisitedAt) {
		t.Errorf("expected latest time %v, got %v", v2.VisitedAt, latestTime)
	}
}

func TestAPICache(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_cache.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer s.Close()

	ctx := t.Context()

	// 1. Miss initially
	val, ok, err := s.GetAPICache(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error on cache get: %v", err)
	}
	if ok || val != "" {
		t.Fatalf("expected cache miss, got ok=%v, val=%s", ok, val)
	}

	// 2. Set active key
	err = s.SetAPICache(ctx, "edsm:system:Sol", `{"name":"Sol"}`, 8*time.Hour)
	if err != nil {
		t.Fatalf("failed setting cache: %v", err)
	}

	val, ok, err = s.GetAPICache(ctx, "edsm:system:Sol")
	if err != nil {
		t.Fatalf("unexpected error getting cache: %v", err)
	}
	if !ok || val != `{"name":"Sol"}` {
		t.Fatalf("expected cache hit with Sol, got ok=%v, val=%s", ok, val)
	}

	// 3. Expired key
	err = s.SetAPICache(ctx, "expired_key", "old", -1*time.Second)
	if err != nil {
		t.Fatalf("failed setting expired cache: %v", err)
	}

	val, ok, err = s.GetAPICache(ctx, "expired_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected expired key to miss, got ok=true, val=%s", val)
	}
}

func TestStoreCustomPruning(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_custom_prune.db")

	s, err := New(dbPath, WithMaxVisited(15), WithMaxTargeted(10))
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	defer s.Close()

	if s.MaxVisited() != 15 {
		t.Errorf("expected MaxVisited 15, got %d", s.MaxVisited())
	}
	if s.MaxTargeted() != 10 {
		t.Errorf("expected MaxTargeted 10, got %d", s.MaxTargeted())
	}

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 30; i++ {
		_ = s.RecordVisited(VisitedSystem{
			SystemName:    fmt.Sprintf("Visited-%02d", i),
			SystemAddress: int64(1000 + i),
			VisitedAt:     baseTime.Add(time.Duration(i) * time.Minute),
		})
		_ = s.RecordTargeted(TargetedSystem{
			SystemName:    fmt.Sprintf("Target-%02d", i),
			SystemAddress: int64(2000 + i),
			TargetedAt:    baseTime.Add(time.Duration(i) * time.Minute),
		})
	}

	visited, err := s.GetVisited(50)
	if err != nil {
		t.Fatalf("failed getting visited: %v", err)
	}
	if len(visited) != 15 {
		t.Errorf("expected 15 visited due to custom limit, got %d", len(visited))
	}
	if visited[0].SystemName != "Visited-30" {
		t.Errorf("expected newest visited to be Visited-30, got %s", visited[0].SystemName)
	}
	if visited[len(visited)-1].SystemName != "Visited-16" {
		t.Errorf("expected oldest visited to be Visited-16, got %s", visited[len(visited)-1].SystemName)
	}

	targeted, err := s.GetTargeted(50)
	if err != nil {
		t.Fatalf("failed getting targeted: %v", err)
	}
	if len(targeted) != 10 {
		t.Errorf("expected 10 targeted due to custom limit, got %d", len(targeted))
	}
	if targeted[0].SystemName != "Target-30" {
		t.Errorf("expected newest targeted to be Target-30, got %s", targeted[0].SystemName)
	}
	if targeted[len(targeted)-1].SystemName != "Target-21" {
		t.Errorf("expected oldest targeted to be Target-21, got %s", targeted[len(targeted)-1].SystemName)
	}
}


