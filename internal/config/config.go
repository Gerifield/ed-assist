package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration options for ed-assist.
type Config struct {
	EnableTracking bool          `json:"enable_tracking"`
	DBPath         string        `json:"db_path"`
	StatusFilePath string        `json:"status_file_path"`
	ConfigSource   string        `json:"config_source"`
	EnableMCP      bool          `json:"enable_mcp"`
	Mode           string        `json:"mode"`
	PollInterval   time.Duration `json:"poll_interval"`
	MaxRetries     int           `json:"max_retries"`
	RetryDelay     time.Duration `json:"retry_delay"`
	LogLevel       string        `json:"log_level"`
	LogFile        string        `json:"log_file"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	defaultPath := DetermineDefaultStatusPath()
	return &Config{
		EnableTracking: false,
		DBPath:         DetermineDefaultDBPath(),
		StatusFilePath: defaultPath,
		ConfigSource:   "auto-determined",
		EnableMCP:      false,
		Mode:           "watch",                // Event-driven by default
		PollInterval:   250 * time.Millisecond, // 1/4 second when polling
		MaxRetries:     3,
		RetryDelay:     25 * time.Millisecond,
		LogLevel:       "info",
	}
}

// DetermineDefaultDBPath returns the path to ed_assist.db next to the binary.
func DetermineDefaultDBPath() string {
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		return filepath.Join(exeDir, "ed_assist.db")
	}
	return "ed_assist.db"
}

// DetermineDefaultStatusPath returns the standard Elite Dangerous Status.json path.
func DetermineDefaultStatusPath() string {
	currUser, err := user.Current()
	if err != nil || currUser == nil || currUser.HomeDir == "" {
		return "Status.json"
	}
	basePath := filepath.FromSlash(currUser.HomeDir + "/Saved Games/Frontier Developments/Elite Dangerous")
	return filepath.Join(basePath, "Status.json")
}

// FindConfigFile searches for config.ini next to the executable or in the current working directory.
func FindConfigFile(overridePath string) (string, bool) {
	if overridePath != "" {
		if _, err := os.Stat(overridePath); err == nil {
			return overridePath, true
		}
	}

	// 1. Check directory next to executable
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidate := filepath.Join(exeDir, "config.ini")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	// 2. Check current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "config.ini")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

// Load loads configuration from config.ini if available, or falls back to defaults.
func Load(configFileOverride string) (*Config, error) {
	cfg := DefaultConfig()

	iniPath, found := FindConfigFile(configFileOverride)
	if !found {
		slog.Debug("no config.ini found, using auto-determined defaults", "expected_path", cfg.StatusFilePath)
		return cfg, nil
	}

	slog.Debug("reading config file", "path", iniPath)
	props, err := parseINIFile(iniPath)
	if err != nil {
		return cfg, fmt.Errorf("failed parsing ini file %s: %w", iniPath, err)
	}

	statusFile := lookupProp(props, "status_file", "status_path", "file", "path")
	if statusFile != "" {
		statusFile = expandPath(statusFile)
		// If path points to a directory, append Status.json
		if info, err := os.Stat(statusFile); err == nil && info.IsDir() {
			statusFile = filepath.Join(statusFile, "Status.json")
		} else if !strings.HasSuffix(strings.ToLower(statusFile), ".json") {
			// If not ending in .json, treat as directory
			statusFile = filepath.Join(statusFile, "Status.json")
		}
		cfg.StatusFilePath = statusFile
		cfg.ConfigSource = fmt.Sprintf("config.ini (%s)", iniPath)
	}

	if db := lookupProp(props, "db_path", "db", "database", "sqlite_path"); db != "" {
		cfg.DBPath = expandPath(db)
	}

	if trackStr := lookupProp(props, "enable_tracking", "tracking", "track"); trackStr != "" {
		trackLower := strings.ToLower(trackStr)
		cfg.EnableTracking = trackLower == "true" || trackLower == "1" || trackLower == "yes" || trackLower == "on"
	}

	if m := lookupProp(props, "mode"); m != "" {
		cfg.Mode = strings.ToLower(m)
	}

	if mcpStr := lookupProp(props, "enable_mcp", "mcp"); mcpStr != "" {
		mcpLower := strings.ToLower(mcpStr)
		cfg.EnableMCP = mcpLower == "true" || mcpLower == "1" || mcpLower == "yes" || mcpLower == "on"
	}

	if intervalStr := lookupProp(props, "poll_interval_ms", "poll_interval"); intervalStr != "" {
		if ms, err := strconv.Atoi(intervalStr); err == nil && ms > 0 {
			cfg.PollInterval = time.Duration(ms) * time.Millisecond
		} else if d, err := time.ParseDuration(intervalStr); err == nil && d > 0 {
			cfg.PollInterval = d
		}
	}

	if retriesStr := lookupProp(props, "max_retries", "retries"); retriesStr != "" {
		if r, err := strconv.Atoi(retriesStr); err == nil && r >= 0 {
			cfg.MaxRetries = r
		}
	}

	if delayStr := lookupProp(props, "retry_delay_ms", "retry_delay", "retry_time", "retry_time_ms"); delayStr != "" {
		if ms, err := strconv.Atoi(delayStr); err == nil && ms > 0 {
			cfg.RetryDelay = time.Duration(ms) * time.Millisecond
		} else if d, err := time.ParseDuration(delayStr); err == nil && d > 0 {
			cfg.RetryDelay = d
		}
	}

	if lvl := lookupProp(props, "loglevel", "log_level"); lvl != "" {
		cfg.LogLevel = strings.ToLower(lvl)
	}

	if lf := lookupProp(props, "logfile", "log_file"); lf != "" {
		cfg.LogFile = lf
	}

	return cfg, nil
}

func expandPath(path string) string {
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if u, err := user.Current(); err == nil && u.HomeDir != "" {
			return filepath.Join(u.HomeDir, path[2:])
		}
	}
	return filepath.Clean(path)
}

func lookupProp(props map[string]string, keys ...string) string {
	for _, k := range keys {
		if val, ok := props[strings.ToLower(k)]; ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// parseINIFile parses a simple INI file into a key-value map.
// Section prefixes are stripped or joined, allowing lookup by key directly.
func parseINIFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	currentSection := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || line[0] == ';' || line[0] == '#' {
			continue
		}

		// Check for section [section_name]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}

		// Split on = or :
		var key, val string
		if idx := strings.IndexAny(line, "=:"); idx != -1 {
			key = strings.TrimSpace(line[:idx])
			val = strings.TrimSpace(line[idx+1:])
			// Strip surrounding quotes if present
			val = strings.Trim(val, `"'`)
		} else {
			continue
		}

		keyLower := strings.ToLower(key)
		result[keyLower] = val
		if currentSection != "" {
			result[strings.ToLower(currentSection+"."+key)] = val
		}
	}

	return result, scanner.Err()
}
