package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherDetectsWrite(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "Status.json")

	// Pre-create the file
	if err := os.WriteFile(targetFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}

	w, err := New(targetFile)
	if err != nil {
		t.Fatalf("failed to initialize watcher: %v", err)
	}
	defer w.Close()

	w.Watch()

	// Give watcher a moment to register
	time.Sleep(50 * time.Millisecond)

	// Write update
	if err := os.WriteFile(targetFile, []byte(`{"event":"Status"}`), 0644); err != nil {
		t.Fatalf("failed writing update: %v", err)
	}

	select {
	case event, ok := <-w.Events:
		if !ok {
			t.Fatal("events channel closed prematurely")
		}
		if event.Name != "Status.json" {
			t.Errorf("expected Status.json event, got: %s", event.Name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for write event")
	}
}
