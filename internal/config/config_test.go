package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.EnableTracking {
		t.Errorf("expected EnableTracking to be false by default")
	}
	if cfg.PollInterval != 250*time.Millisecond {
		t.Errorf("expected 250ms poll interval, got %v", cfg.PollInterval)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected 3 retries, got %d", cfg.MaxRetries)
	}
	if cfg.StatusFilePath == "" {
		t.Errorf("expected non-empty status file path")
	}
	if !filepath.IsAbs(cfg.StatusFilePath) && cfg.StatusFilePath != "Status.json" {
		t.Errorf("expected absolute path or Status.json, got %s", cfg.StatusFilePath)
	}
}

func TestLoadWithINIFile(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")

	content := `
; Sample config
[general]
status_file = /tmp/custom/Status.json
poll_interval_ms = 500
max_retries = 5
loglevel = debug
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.StatusFilePath != filepath.Clean("/tmp/custom/Status.json") {
		t.Errorf("expected /tmp/custom/Status.json, got %s", cfg.StatusFilePath)
	}
	if cfg.PollInterval != 500*time.Millisecond {
		t.Errorf("expected 500ms, got %v", cfg.PollInterval)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("expected 5 retries, got %d", cfg.MaxRetries)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected debug log level, got %s", cfg.LogLevel)
	}
}

func TestLoadModeAndRetryTime(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")

	content := `
[general]
mode = poll
retry_time_ms = 45
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != "poll" {
		t.Errorf("expected mode poll, got %s", cfg.Mode)
	}
	if cfg.RetryDelay != 45*time.Millisecond {
		t.Errorf("expected retry delay 45ms, got %v", cfg.RetryDelay)
	}
}

func TestLoadMCPConfig(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")

	content := `
[general]
enable_mcp = true
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.EnableMCP {
		t.Errorf("expected EnableMCP to be true")
	}
}

func TestLoadTrackingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")

	content := `
[general]
enable_tracking = true
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.EnableTracking {
		t.Errorf("expected EnableTracking to be true")
	}
}

func TestLoadGameControlConfig(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")

	content := `
[general]
game_control = true
bindings_path = /tmp/custom/bindings
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.GameControl {
		t.Errorf("expected GameControl to be true")
	}
	if cfg.BindingsPath != filepath.Clean("/tmp/custom/bindings") {
		t.Errorf("expected /tmp/custom/bindings, got %s", cfg.BindingsPath)
	}
}
