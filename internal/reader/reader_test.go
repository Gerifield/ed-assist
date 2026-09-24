package reader

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const sampleStatus1 = `{
	"timestamp": "2024-03-01T10:00:00Z",
	"event": "Status",
	"Flags": 16842765,
	"Pips": [2, 8, 2],
	"FireGroup": 0,
	"Fuel": {"FuelMain": 15.0, "FuelReservoir": 0.5},
	"GuiFocus": 0
}`

const sampleStatus2 = `{
	"timestamp": "2024-03-01T10:00:05Z",
	"event": "Status",
	"Flags": 16,
	"Pips": [4, 4, 4],
	"FireGroup": 0,
	"Fuel": {"FuelMain": 14.9, "FuelReservoir": 0.4},
	"GuiFocus": 0
}`

func TestReaderReadOnce(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Status.json")

	if err := os.WriteFile(filePath, []byte(sampleStatus1), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	r := New(filePath)
	status, err := r.ReadOnce()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Timestamp != "2024-03-01T10:00:00Z" {
		t.Errorf("timestamp mismatch: got %s", status.Timestamp)
	}
	if !status.Flags.Docked {
		t.Errorf("expected Docked to be true")
	}
}

func TestReaderQuickRetry(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Status.json")

	// Start with empty file to simulate mid-write
	if err := os.WriteFile(filePath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	// Configure generous retry window (15 retries * 40ms = 600ms) to withstand CI scheduling delays
	r := New(filePath, WithRetries(15, 40*time.Millisecond))

	// In a separate goroutine, simulate game finishing the write shortly
	go func() {
		time.Sleep(15 * time.Millisecond)
		_ = os.WriteFile(filePath, []byte(sampleStatus1), 0644)
	}()

	status, err := r.ReadOnce()
	if err != nil {
		t.Fatalf("expected retry to succeed, but got error: %v", err)
	}
	if status == nil || status.Timestamp != "2024-03-01T10:00:00Z" {
		t.Errorf("unexpected status returned: %+v", status)
	}
}

func TestReaderPollingUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Status.json")

	if err := os.WriteFile(filePath, []byte(sampleStatus1), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(filePath, WithMode(ModePoll), WithPollInterval(50*time.Millisecond))
	go r.Start(ctx)

	// Wait for first status
	select {
	case s := <-r.StatusChan():
		if s.Timestamp != "2024-03-01T10:00:00Z" {
			t.Errorf("expected timestamp 1, got %s", s.Timestamp)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for initial status")
	}

	// Update file
	if err := os.WriteFile(filePath, []byte(sampleStatus2), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// Wait for second status
	select {
	case s := <-r.StatusChan():
		if s.Timestamp != "2024-03-01T10:00:05Z" {
			t.Errorf("expected timestamp 2, got %s", s.Timestamp)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for updated status")
	}

	cancel()
	r.Wait()
}

func TestReaderWatchUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Status.json")

	if err := os.WriteFile(filePath, []byte(sampleStatus1), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(filePath, WithMode(ModeWatch))
	go r.Start(ctx)

	// Wait for first status (read on startup)
	select {
	case s := <-r.StatusChan():
		if s.Timestamp != "2024-03-01T10:00:00Z" {
			t.Errorf("expected timestamp 1, got %s", s.Timestamp)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for initial status")
	}

	// Trigger watch update by modifying file
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(filePath, []byte(sampleStatus2), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// Wait for second status triggered by fsnotify
	select {
	case s := <-r.StatusChan():
		if s.Timestamp != "2024-03-01T10:00:05Z" {
			t.Errorf("expected timestamp 2, got %s", s.Timestamp)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for watch-triggered status update")
	}

	cancel()
	r.Wait()
}

func TestReaderFileNotFoundDoesNotCrash(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "NonExistentStatus.json")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(filePath, WithPollInterval(30*time.Millisecond))
	go r.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	var received atomic.Int32
	select {
	case <-r.StatusChan():
		received.Add(1)
	default:
	}

	if received.Load() != 0 {
		t.Errorf("expected 0 events when file does not exist")
	}

	cancel()
	r.Wait()
}

func TestReaderIgnoreStaleTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Status.json")

	// 1. Write newer status (10:00:05Z)
	if err := os.WriteFile(filePath, []byte(sampleStatus2), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	r := New(filePath)
	st, err := r.ReadOnce()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Timestamp != "2024-03-01T10:00:05Z" {
		t.Fatalf("expected initial timestamp 10:00:05Z, got %s", st.Timestamp)
	}

	// 2. Overwrite file with an older status (10:00:00Z)
	if err := os.WriteFile(filePath, []byte(sampleStatus1), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 3. ReadOnce returns the parsed content of the file, but r.LastStatus() must retain the newer status
	_, err = r.ReadOnce()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	last := r.LastStatus()
	if last == nil || last.Timestamp != "2024-03-01T10:00:05Z" {
		t.Errorf("expected LastStatus() to retain newer timestamp 10:00:05Z, got %v", last.Timestamp)
	}
}
