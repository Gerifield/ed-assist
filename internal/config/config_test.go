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
	if cfg.GeminiModel != "gemini-flash-lite-latest" {
		t.Errorf("expected default GeminiModel gemini-flash-lite-latest, got %s", cfg.GeminiModel)
	}
	if cfg.KeyHoldMs != 80 {
		t.Errorf("expected default KeyHoldMs 80, got %d", cfg.KeyHoldMs)
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
mcp_transport = http
mcp_addr = 8088
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
	if cfg.MCPTransport != "http" {
		t.Errorf("expected MCPTransport to be http, got %s", cfg.MCPTransport)
	}
	if cfg.MCPAddr != "127.0.0.1:8088" {
		t.Errorf("expected MCPAddr 127.0.0.1:8088, got %s", cfg.MCPAddr)
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
key_hold_ms = 120
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
	if cfg.KeyHoldMs != 120 {
		t.Errorf("expected KeyHoldMs 120, got %d", cfg.KeyHoldMs)
	}
	if cfg.BindingsPath != filepath.Clean("/tmp/custom/bindings") {
		t.Errorf("expected /tmp/custom/bindings, got %s", cfg.BindingsPath)
	}
}

func TestLoadWebAndGeminiConfig(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")

	content := `
[general]
web_addr = 4000
gemini_api_key = test-key-123
gemini_model = gemini-3.8-flash-lite
gemini_mcp_mode = stdio
gemini_mcp_endpoint = /path/to/ed-assist
voice_gate_threshold = 45
voice_silence_ms = 1500
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WebAddr != "127.0.0.1:4000" {
		t.Errorf("expected WebAddr 127.0.0.1:4000, got %s", cfg.WebAddr)
	}
	if cfg.GeminiAPIKey != "test-key-123" {
		t.Errorf("expected GeminiAPIKey test-key-123, got %s", cfg.GeminiAPIKey)
	}
	if cfg.GeminiModel != "gemini-3.8-flash-lite" {
		t.Errorf("expected GeminiModel gemini-3.8-flash-lite, got %s", cfg.GeminiModel)
	}
	if cfg.GeminiMCPMode != "stdio" {
		t.Errorf("expected GeminiMCPMode stdio, got %s", cfg.GeminiMCPMode)
	}
	if cfg.GeminiMCPEndpoint != "/path/to/ed-assist" {
		t.Errorf("expected GeminiMCPEndpoint /path/to/ed-assist, got %s", cfg.GeminiMCPEndpoint)
	}
	if cfg.VoiceGateThreshold != 45 {
		t.Errorf("expected VoiceGateThreshold 45, got %d", cfg.VoiceGateThreshold)
	}
	if cfg.VoiceSilenceMs != 1500 {
		t.Errorf("expected VoiceSilenceMs 1500, got %d", cfg.VoiceSilenceMs)
	}
}

func TestLoadSystemCacheConfig(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")

	content := `
[general]
system_cache_hours = 12
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SystemCacheHours != 12 {
		t.Errorf("expected SystemCacheHours 12, got %d", cfg.SystemCacheHours)
	}
	if cfg.SystemCacheTTL != 12*time.Hour {
		t.Errorf("expected SystemCacheTTL 12h, got %v", cfg.SystemCacheTTL)
	}
}

func TestLoadVoiceEchoProtection(t *testing.T) {
	// 1. Default should be true
	cfgDef := DefaultConfig()
	if !cfgDef.VoiceEchoProtection {
		t.Errorf("expected default VoiceEchoProtection to be true")
	}

	// 2. Explicitly false
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "config.ini")
	content := `
[general]
voice_echo_protection = false
`
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test ini file: %v", err)
	}

	cfg, err := Load(iniPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VoiceEchoProtection {
		t.Errorf("expected VoiceEchoProtection false, got true")
	}
}

