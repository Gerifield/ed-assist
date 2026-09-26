package tracker

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ed-assist/internal/parser"
	"ed-assist/internal/store"
	"ed-assist/internal/watcher"
)

// JournalEvent minimal structure for filtering journal lines.
type JournalEvent struct {
	Timestamp        string     `json:"timestamp"`
	Event            string     `json:"event"`
	StarSystem       string     `json:"StarSystem,omitempty"`
	SystemAddress    int64      `json:"SystemAddress,omitempty"`
	StarPos          [3]float64 `json:"StarPos,omitempty"`
	SystemAllegiance string     `json:"SystemAllegiance,omitempty"`
	SystemEconomy    string     `json:"SystemEconomy,omitempty"`
	SystemGovernment string     `json:"SystemGovernment,omitempty"`
	SystemSecurity   string     `json:"SystemSecurity,omitempty"`
	Population       *int64     `json:"Population,omitempty"`
	Body             string     `json:"Body,omitempty"`
	JumpDist         *float64   `json:"JumpDist,omitempty"`

	// Vehicle fields
	Ship                string `json:"Ship,omitempty"`
	ShipName            string `json:"ShipName,omitempty"`
	ShipType            string `json:"ShipType,omitempty"`
	VesselType          string `json:"VesselType,omitempty"`
	VesselTypeLocalised string `json:"VesselTypeLocalised,omitempty"`
	SRVType             string `json:"SRVType,omitempty"`
	SRVTypeLocalised    string `json:"SRVType_Localised,omitempty"`
	Loadout             string `json:"Loadout,omitempty"`
	PlayerControlled    *bool  `json:"PlayerControlled,omitempty"`
	To                  string `json:"To,omitempty"`
	SRV                 *bool  `json:"SRV,omitempty"`
	Taxi                *bool  `json:"Taxi,omitempty"`
	Multicrew           *bool  `json:"Multicrew,omitempty"`
}

// VehicleState holds the latest vehicle state inferred from Journal events with its exact timestamp.
type VehicleState struct {
	Mode      string    `json:"mode"`      // "Ship", "Nomad", "SRV", "Fighter", "On Foot"
	Event     string    `json:"event"`     // "DockSRV", "DockFighter", "DockVessel", "LaunchSRV", "LaunchFighter", "LaunchVessel", "Embark", "Disembark", etc.
	Timestamp time.Time `json:"timestamp"` // exact UTC event timestamp
}

// Tracker monitors Status.json updates and Journal logs to record visited and targeted systems,
// as well as authoritative player vehicle state (Ship, Nomad, SRV, Fighter, On Foot).
type Tracker struct {
	store      *store.Store
	journalDir string
	mu         sync.Mutex

	lastTargetedName string
	lastTargetedAddr int64

	latestVehicle   VehicleState
	deployedVehicle string // "Nomad", "SRV", "Fighter", or "" (docked in ship)

	currentJournalFile string
	journalOffset      int64
}

// isLanderOrNomad checks if strings contain markers for the Nomad SLV (Falcon DeLacy Lander01).
func isLanderOrNomad(strs ...string) bool {
	for _, s := range strs {
		if s == "" {
			continue
		}
		low := strings.ToLower(s)
		if strings.Contains(low, "nomad") || strings.Contains(low, "lander") || low == "base" || strings.Contains(low, "base_nomad") || strings.Contains(low, "base_lander") {
			return true
		}
	}
	return false
}

// isSRV checks if strings indicate a surface reconnaissance vehicle (Scarab or Scorpion).
func isSRV(strs ...string) bool {
	for _, s := range strs {
		if s == "" {
			continue
		}
		low := strings.ToLower(s)
		if strings.Contains(low, "buggy") || strings.Contains(low, "scarab") || strings.Contains(low, "scorpion") || strings.Contains(low, "srv") {
			return true
		}
	}
	return false
}

// isFighter checks if strings indicate a ship-launched combat fighter (Condor, Taipan, Imperial Fighter, etc.).
func isFighter(strs ...string) bool {
	for _, s := range strs {
		if s == "" {
			continue
		}
		low := strings.ToLower(s)
		if strings.Contains(low, "fighter") || strings.Contains(low, "condor") || strings.Contains(low, "taipan") || strings.Contains(low, "trident") || strings.Contains(low, "javelin") || strings.Contains(low, "lance") {
			return true
		}
	}
	return false
}

// New creates a new Tracker.
func New(s *store.Store, statusFilePath string) *Tracker {
	dir := filepath.Dir(statusFilePath)
	return &Tracker{
		store:      s,
		journalDir: dir,
	}
}

// ProcessStatus processes a newly parsed Status.json for targeted systems and location updates.
func (t *Tracker) ProcessStatus(st *parser.Status) {
	if st == nil {
		return
	}

	// 1. Check targeted destination
	if st.Destination != nil && st.Destination.Name != "" {
		t.mu.Lock()
		changed := st.Destination.Name != t.lastTargetedName || (st.Destination.System != 0 && st.Destination.System != t.lastTargetedAddr)
		if changed {
			t.lastTargetedName = st.Destination.Name
			t.lastTargetedAddr = st.Destination.System
		}
		t.mu.Unlock()

		if changed {
			targetedAt := time.Now().UTC()
			if pt, err := st.ParsedTime(); err == nil {
				targetedAt = pt
			}

			if t.store != nil {
				raw, _ := json.Marshal(st.Destination)
				_ = t.store.RecordTargeted(store.TargetedSystem{
					SystemName:    st.Destination.Name,
					SystemAddress: st.Destination.System,
					BodyID:        st.Destination.Body,
					TargetedAt:    targetedAt,
					RawJSON:       string(raw),
				})
			}
		}
	}
}

// Start begins monitoring Journal logs in journalDir to track visited systems and vehicle states.
func (t *Tracker) Start(ctx context.Context) {
	// 1. Initial backfill from existing journal files
	t.backfillRecentJournals()

	// 2. Set up watcher for new journal files or writes to journalDir
	w, err := watcher.New(t.journalDir)
	if err != nil {
		slog.Warn("could not watch journal directory, relying on status updates", "dir", t.journalDir, "error", err)
		return
	}
	defer w.Close()
	w.Watch()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	slog.Info("journal tracker active", "dir", t.journalDir)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if strings.HasPrefix(ev.Name, "Journal.") && strings.HasSuffix(ev.Name, ".log") {
				t.checkActiveJournal()
			}
		case <-ticker.C:
			// Periodic check in case filesystem event was missed
			t.checkActiveJournal()
		}
	}
}

func (t *Tracker) backfillRecentJournals() {
	files, err := filepath.Glob(filepath.Join(t.journalDir, "Journal.*.log"))
	if err != nil || len(files) == 0 {
		return
	}

	// Sort files by name (which has ISO date embedded) ascending
	sort.Strings(files)

	var latestDBTime time.Time
	if t.store != nil {
		latestDBTime = t.store.GetLatestVisitedTime()
	}

	// Take up to the last 5 journal files to backfill up to 100 systems
	startIdx := 0
	if len(files) > 5 {
		startIdx = len(files) - 5
	}

	for _, f := range files[startIdx:] {
		// If we already have recorded visits in DB and this journal file was last modified
		// before our newest DB entry, skip reading the whole file.
		if !latestDBTime.IsZero() {
			if fi, err := os.Stat(f); err == nil && fi.ModTime().Before(latestDBTime.Add(-10*time.Second)) {
				continue
			}
		}
		t.parseJournalFile(f, false)
	}

	// Point active journal file to the newest one
	t.currentJournalFile = files[len(files)-1]
	if fi, err := os.Stat(t.currentJournalFile); err == nil {
		t.journalOffset = fi.Size()
	}
}

func (t *Tracker) checkActiveJournal() {
	files, err := filepath.Glob(filepath.Join(t.journalDir, "Journal.*.log"))
	if err != nil || len(files) == 0 {
		return
	}

	sort.Strings(files)
	latest := files[len(files)-1]

	t.mu.Lock()
	if latest != t.currentJournalFile {
		// Elite Dangerous rolled over to a new journal file
		t.currentJournalFile = latest
		t.journalOffset = 0
	}
	targetFile := t.currentJournalFile
	offset := t.journalOffset
	t.mu.Unlock()

	newOffset := t.tailJournalFile(targetFile, offset)

	t.mu.Lock()
	t.journalOffset = newOffset
	t.mu.Unlock()
}

func (t *Tracker) tailJournalFile(path string, startOffset int64) int64 {
	f, err := os.Open(path)
	if err != nil {
		return startOffset
	}
	defer f.Close()

	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		return startOffset
	}

	scanner := bufio.NewScanner(f)
	currentOffset := startOffset

	for scanner.Scan() {
		line := scanner.Bytes()
		lineLen := int64(len(line) + 1) // +1 for newline
		currentOffset += lineLen

		t.handleJournalLine(line)
	}

	return currentOffset
}

func (t *Tracker) parseJournalFile(path string, isTail bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		t.handleJournalLine(scanner.Bytes())
	}
}

func (t *Tracker) handleJournalLine(line []byte) {
	trimmed := strings.TrimSpace(string(line))
	if len(trimmed) == 0 {
		return
	}

	var ev JournalEvent
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
		return
	}

	evAt := time.Now().UTC()
	if ev.Timestamp != "" {
		if pt, err := time.Parse(time.RFC3339, ev.Timestamp); err == nil {
			evAt = pt
		}
	}

	// 1. Authoritative vehicle tracking across all vehicle launch, dock, embark, disembark, and jump events
	var vehicleMode string
	switch ev.Event {
	case "LaunchVessel":
		// Dedicated Frontier journal event for the Nomad SLV exploration vessel
		if isLanderOrNomad(ev.VesselType, ev.VesselTypeLocalised, ev.Loadout) || ev.VesselType == "" {
			vehicleMode = "Nomad"
		} else {
			vehicleMode = "Fighter"
		}

	case "LaunchFighter":
		// If loadout or vehicle specifies Nomad / Lander, it's the Nomad SLV
		if isLanderOrNomad(ev.Loadout, ev.VesselType, ev.VesselTypeLocalised) {
			vehicleMode = "Nomad"
		} else {
			vehicleMode = "Fighter"
		}

	case "LaunchSRV":
		// Check for potential Nomad / Lander loadout or SRV variant
		if isLanderOrNomad(ev.SRVType, ev.SRVTypeLocalised, ev.Loadout) {
			vehicleMode = "Nomad"
		} else {
			vehicleMode = "SRV"
		}

	case "DockSRV", "DockFighter", "DockVessel":
		// Docked back into mothership
		vehicleMode = "Ship"

	case "VehicleSwitch":
		if strings.EqualFold(ev.To, "Mothership") {
			vehicleMode = "Ship"
		} else if strings.EqualFold(ev.To, "Fighter") {
			t.mu.Lock()
			dep := t.deployedVehicle
			t.mu.Unlock()
			if dep == "Nomad" {
				vehicleMode = "Nomad"
			} else {
				vehicleMode = "Fighter"
			}
		}

	case "SupercruiseEntry", "SupercruiseExit", "FSDJump", "CarrierJump", "Undocked":
		// Supercruise / hyperspace can only occur in the main ship
		vehicleMode = "Ship"

	case "Disembark":
		// Commander stepped out on foot
		vehicleMode = "On Foot"

	case "Embark":
		t.mu.Lock()
		dep := t.deployedVehicle
		t.mu.Unlock()

		if (ev.SRV != nil && *ev.SRV) || strings.Contains(trimmed, `"SRV":true`) || strings.Contains(trimmed, `"SRV": true`) {
			// Frontier puts SRV: true when embarking into Nomad on planet surface!
			if dep == "Nomad" || isLanderOrNomad(ev.SRVType, ev.SRVTypeLocalised) {
				vehicleMode = "Nomad"
			} else {
				vehicleMode = "SRV"
			}
		} else if ev.Taxi != nil && *ev.Taxi {
			vehicleMode = "Ship"
		} else {
			// Re-embarked into mothership
			vehicleMode = "Ship"
		}

	case "Loadout", "LoadGame":
		if ev.Ship != "" {
			if isLanderOrNomad(ev.Ship, ev.ShipName, ev.ShipType) {
				vehicleMode = "Nomad"
			} else if isSRV(ev.Ship, ev.ShipType) {
				vehicleMode = "SRV"
			} else if isFighter(ev.Ship, ev.ShipType) {
				vehicleMode = "Fighter"
			} else if !strings.Contains(strings.ToLower(ev.Ship), "suit") && !strings.Contains(strings.ToLower(ev.Ship), "taxi") {
				vehicleMode = "Ship"
			}
		}

	case "SRVDestroyed", "FighterDestroyed":
		vehicleMode = "Ship"
	}

	if vehicleMode != "" {
		t.mu.Lock()
		if t.latestVehicle.Timestamp.IsZero() || !evAt.Before(t.latestVehicle.Timestamp) {
			t.latestVehicle = VehicleState{
				Mode:      vehicleMode,
				Event:     ev.Event,
				Timestamp: evAt,
			}
			// Maintain deployed vehicle identity across disembark / embark loops
			switch vehicleMode {
			case "Nomad":
				t.deployedVehicle = "Nomad"
			case "SRV":
				t.deployedVehicle = "SRV"
			case "Fighter":
				t.deployedVehicle = "Fighter"
			case "Ship":
				t.deployedVehicle = ""
			case "On Foot":
				// Retain deployedVehicle so Embark can recover whether player was in Nomad or SRV
			}
			slog.Debug("journal vehicle state updated",
				"mode", vehicleMode,
				"event", ev.Event,
				"deployed", t.deployedVehicle,
				"timestamp", evAt,
			)
		}
		t.mu.Unlock()
	}

	// 2. Look for system visit events: FSDJump, Location (startup location), CarrierJump
	if (ev.Event == "FSDJump" || ev.Event == "Location" || ev.Event == "CarrierJump") && t.store != nil {
		if ev.StarSystem == "" && ev.SystemAddress == 0 {
			return
		}

		visitedAt := evAt

		var posX, posY, posZ *float64
		if ev.StarPos != [3]float64{0, 0, 0} {
			x, y, z := ev.StarPos[0], ev.StarPos[1], ev.StarPos[2]
			posX, posY, posZ = &x, &y, &z
		}

		_ = t.store.RecordVisited(store.VisitedSystem{
			SystemName:    ev.StarSystem,
			SystemAddress: ev.SystemAddress,
			StarPosX:      posX,
			StarPosY:      posY,
			StarPosZ:      posZ,
			Allegiance:    ev.SystemAllegiance,
			Economy:       ev.SystemEconomy,
			Government:    ev.SystemGovernment,
			Security:      ev.SystemSecurity,
			Population:    ev.Population,
			BodyName:      ev.Body,
			JumpDist:      ev.JumpDist,
			VisitedAt:     visitedAt,
			RawJSON:       trimmed,
		})
	}
}

// LatestVehicleState returns the most recent vehicle state recorded from Journal events.
func (t *Tracker) LatestVehicleState() (mode string, event string, timestamp time.Time, ok bool) {
	if t == nil {
		return "", "", time.Time{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.latestVehicle.Timestamp.IsZero() {
		return "", "", time.Time{}, false
	}
	return t.latestVehicle.Mode, t.latestVehicle.Event, t.latestVehicle.Timestamp, true
}

// Store returns the underlying sqlite store.
func (t *Tracker) Store() *store.Store {
	return t.store
}
