package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// VisitedSystem represents a recorded visited star system.
type VisitedSystem struct {
	ID            int64     `json:"id"`
	SystemName    string    `json:"system_name"`
	SystemAddress int64     `json:"system_address"`
	StarPosX      *float64  `json:"star_pos_x,omitempty"`
	StarPosY      *float64  `json:"star_pos_y,omitempty"`
	StarPosZ      *float64  `json:"star_pos_z,omitempty"`
	Allegiance    string    `json:"allegiance,omitempty"`
	Economy       string    `json:"economy,omitempty"`
	Government    string    `json:"government,omitempty"`
	Security      string    `json:"security,omitempty"`
	Population    *int64    `json:"population,omitempty"`
	BodyName      string    `json:"body_name,omitempty"`
	JumpDist      *float64  `json:"jump_dist,omitempty"`
	VisitedAt     time.Time `json:"visited_at"`
	RawJSON       string    `json:"raw_json,omitempty"`
}

// TargetedSystem represents a recorded targeted destination / star system.
type TargetedSystem struct {
	ID            int64     `json:"id"`
	SystemName    string    `json:"system_name"`
	SystemAddress int64     `json:"system_address"`
	BodyID        int       `json:"body_id,omitempty"`
	TargetedAt    time.Time `json:"targeted_at"`
	RawJSON       string    `json:"raw_json,omitempty"`
}

// Store manages SQLite persistence for visited and targeted systems.
type Store struct {
	db     *sql.DB
	dbPath string
	mu     sync.Mutex

	lastVisitedAddr int64
	lastVisitedName string
	lastTargetAddr  int64
	lastTargetName  string
}

// DefaultDBPath returns the path to ed_assist.db located next to the binary.
func DefaultDBPath() string {
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		return filepath.Join(exeDir, "ed_assist.db")
	}
	// Fallback to current working directory
	return "ed_assist.db"
}

// New opens or creates the SQLite database at dbPath and initializes the schema.
func New(dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}

	// Ensure parent directory exists
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed creating db directory %s: %w", dir, err)
		}
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed opening sqlite db %s: %w", dbPath, err)
	}

	// Set connection pool limits
	db.SetMaxOpenConns(1) // SQLite works best with 1 writer connection

	s := &Store{
		db:     db,
		dbPath: dbPath,
	}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed initializing db schema: %w", err)
	}

	s.loadLastKnown()
	slog.Info("sqlite store initialized", "path", dbPath)
	return s, nil
}

func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS visited_systems (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		system_name TEXT NOT NULL,
		system_address INTEGER NOT NULL DEFAULT 0,
		star_pos_x REAL,
		star_pos_y REAL,
		star_pos_z REAL,
		allegiance TEXT DEFAULT '',
		economy TEXT DEFAULT '',
		government TEXT DEFAULT '',
		security TEXT DEFAULT '',
		population INTEGER,
		body_name TEXT DEFAULT '',
		jump_dist REAL,
		visited_at TEXT NOT NULL,
		raw_json TEXT DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_visited_at ON visited_systems(visited_at DESC, id DESC);
	CREATE INDEX IF NOT EXISTS idx_visited_addr ON visited_systems(system_address);

	CREATE TABLE IF NOT EXISTS targeted_systems (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		system_name TEXT NOT NULL,
		system_address INTEGER NOT NULL DEFAULT 0,
		body_id INTEGER NOT NULL DEFAULT 0,
		targeted_at TEXT NOT NULL,
		raw_json TEXT DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_targeted_at ON targeted_systems(targeted_at DESC, id DESC);
	CREATE INDEX IF NOT EXISTS idx_targeted_addr ON targeted_systems(system_address);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) loadLastKnown() {
	// Query the most recent visited system to prevent duplicate recording on restart
	row := s.db.QueryRow("SELECT system_name, system_address FROM visited_systems ORDER BY visited_at DESC, id DESC LIMIT 1")
	_ = row.Scan(&s.lastVisitedName, &s.lastVisitedAddr)

	// Query the most recent targeted system
	row = s.db.QueryRow("SELECT system_name, system_address FROM targeted_systems ORDER BY targeted_at DESC, id DESC LIMIT 1")
	_ = row.Scan(&s.lastTargetName, &s.lastTargetAddr)
}

// RecordVisited stores a visited system and prunes the table to the latest 100 entries.
func (s *Store) RecordVisited(v VisitedSystem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v.SystemName == "" && v.SystemAddress == 0 {
		return nil
	}

	// Avoid duplicate consecutive insertions of the same system
	if (v.SystemAddress != 0 && v.SystemAddress == s.lastVisitedAddr) ||
		(v.SystemName != "" && v.SystemName == s.lastVisitedName) {
		return nil
	}

	if v.VisitedAt.IsZero() {
		v.VisitedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO visited_systems (
		system_name, system_address, star_pos_x, star_pos_y, star_pos_z,
		allegiance, economy, government, security, population, body_name,
		jump_dist, visited_at, raw_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	visitedAtStr := v.VisitedAt.Format(time.RFC3339)
	res, err := s.db.Exec(query,
		v.SystemName, v.SystemAddress, v.StarPosX, v.StarPosY, v.StarPosZ,
		v.Allegiance, v.Economy, v.Government, v.Security, v.Population, v.BodyName,
		v.JumpDist, visitedAtStr, v.RawJSON,
	)
	if err != nil {
		return fmt.Errorf("failed inserting visited system: %w", err)
	}

	id, _ := res.LastInsertId()
	s.lastVisitedAddr = v.SystemAddress
	s.lastVisitedName = v.SystemName

	slog.Info("recorded visited system", "system", v.SystemName, "address", v.SystemAddress, "id", id)

	// Prune table to keep only the latest 100 entries
	pruneQuery := `
	DELETE FROM visited_systems WHERE id NOT IN (
		SELECT id FROM visited_systems ORDER BY visited_at DESC, id DESC LIMIT 100
	);
	`
	_, _ = s.db.Exec(pruneQuery)
	return nil
}

// RecordTargeted stores a targeted destination and prunes the table to the latest 100 entries.
func (s *Store) RecordTargeted(t TargetedSystem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.SystemName == "" && t.SystemAddress == 0 {
		return nil
	}

	// Avoid duplicate consecutive insertions of the same target
	if (t.SystemAddress != 0 && t.SystemAddress == s.lastTargetAddr) ||
		(t.SystemName != "" && t.SystemName == s.lastTargetName) {
		return nil
	}

	if t.TargetedAt.IsZero() {
		t.TargetedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO targeted_systems (
		system_name, system_address, body_id, targeted_at, raw_json
	) VALUES (?, ?, ?, ?, ?);
	`
	targetedAtStr := t.TargetedAt.Format(time.RFC3339)
	res, err := s.db.Exec(query, t.SystemName, t.SystemAddress, t.BodyID, targetedAtStr, t.RawJSON)
	if err != nil {
		return fmt.Errorf("failed inserting targeted system: %w", err)
	}

	id, _ := res.LastInsertId()
	s.lastTargetAddr = t.SystemAddress
	s.lastTargetName = t.SystemName

	slog.Info("recorded targeted system", "system", t.SystemName, "address", t.SystemAddress, "id", id)

	// Prune table to keep only the latest 100 entries
	pruneQuery := `
	DELETE FROM targeted_systems WHERE id NOT IN (
		SELECT id FROM targeted_systems ORDER BY targeted_at DESC, id DESC LIMIT 100
	);
	`
	_, _ = s.db.Exec(pruneQuery)
	return nil
}

// GetVisited returns the latest visited systems up to limit (max 100).
func (s *Store) GetVisited(limit int) ([]VisitedSystem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 100 {
		limit = 100
	}

	query := `
	SELECT id, system_name, system_address, star_pos_x, star_pos_y, star_pos_z,
	       allegiance, economy, government, security, population, body_name,
	       jump_dist, visited_at, raw_json
	FROM visited_systems
	ORDER BY visited_at DESC, id DESC
	LIMIT ?;
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed querying visited systems: %w", err)
	}
	defer rows.Close()

	var results []VisitedSystem
	for rows.Next() {
		var v VisitedSystem
		var visitedAtStr string
		err := rows.Scan(
			&v.ID, &v.SystemName, &v.SystemAddress, &v.StarPosX, &v.StarPosY, &v.StarPosZ,
			&v.Allegiance, &v.Economy, &v.Government, &v.Security, &v.Population, &v.BodyName,
			&v.JumpDist, &visitedAtStr, &v.RawJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed scanning visited row: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, visitedAtStr); err == nil {
			v.VisitedAt = t
		}
		results = append(results, v)
	}
	return results, rows.Err()
}

// GetTargeted returns the latest targeted systems up to limit (max 100).
func (s *Store) GetTargeted(limit int) ([]TargetedSystem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 100 {
		limit = 100
	}

	query := `
	SELECT id, system_name, system_address, body_id, targeted_at, raw_json
	FROM targeted_systems
	ORDER BY targeted_at DESC, id DESC
	LIMIT ?;
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed querying targeted systems: %w", err)
	}
	defer rows.Close()

	var results []TargetedSystem
	for rows.Next() {
		var t TargetedSystem
		var targetedAtStr string
		err := rows.Scan(
			&t.ID, &t.SystemName, &t.SystemAddress, &t.BodyID, &targetedAtStr, &t.RawJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed scanning targeted row: %w", err)
		}
		if ts, err := time.Parse(time.RFC3339, targetedAtStr); err == nil {
			t.TargetedAt = ts
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

// Close closes the underlying SQLite database.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// FormatVisitedJSON marshals VisitedSystem slice to JSON.
func FormatVisitedJSON(systems []VisitedSystem) (string, error) {
	b, err := json.MarshalIndent(systems, "", "  ")
	return string(b), err
}

// FormatTargetedJSON marshals TargetedSystem slice to JSON.
func FormatTargetedJSON(systems []TargetedSystem) (string, error) {
	b, err := json.MarshalIndent(systems, "", "  ")
	return string(b), err
}
