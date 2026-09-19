# ed-assist

A lightweight, cross-platform Go helper for **Elite Dangerous** that monitors, reads, and parses `Status.json` in real time.

---

## Features

- **Event-Driven & Polling Modes**:
  - **Watch Mode (`watch`, default)**: Uses OS filesystem notifications (`fsnotify` / `ReadDirectoryChangesW` on Windows, `inotify` on Linux). Consumes **0% CPU and zero disk I/O** when idle, docked, or paused.
  - **Poll Mode (`poll`)**: Periodically reads status every 1/4 second (250ms) or a customized interval.
- **Windows File-Lock Resilience**:
  - Quick-retry loop with configurable delay (e.g. 25ms, up to 3 retries) automatically handles transient `ERROR_SHARING_VIOLATION` locks and in-flight file writes by the game.
- **Full Status Parsing**:
  - **Ship & SRV Status**: Unpacks all 32 `Flags` (Docked, Landed, Landing Gear, Shields Up, Supercruise, Flight Assist, Hardpoints, Silent Running, Fuel Scooping, FSD Charging/Cooldown, SRV states, etc.).
  - **Odyssey On-Foot Status**: Unpacks `Flags2` (On-Foot, In Taxi, In Multicrew, Oxygen, Health, Temperature, Gravity, Selected Weapon).
  - **Cockpit Systems**: Power distribution (`Pips`: SYS, ENG, WEP in half-pips), `Fuel` (Main and Reservoir in tons), `Cargo` mass, and `GuiFocus` menus.
  - **Navigation & Planetary**: Latitude, Longitude, Altitude, Heading, Body Name, Balance, and Destination target.
- **System & Destination Tracking (SQLite)**:
  - Automatically records the **latest 100 visited star systems** (with coordinates, economy, allegiance, population, jump distance from journal logs).
  - Automatically records the **latest 100 targeted systems/destinations** from cockpit nav targets.
  - Persisted locally in an **SQLite database (`ed_assist.db`)** located next to the binary with automatic pruning.
- **Model Context Protocol (MCP) Server**:
  - Exposes all Elite Dangerous game status via standard MCP (JSON-RPC 2.0 on stdio).
  - Works with AI assistants and IDEs (Claude Desktop, Cursor, Antigravity, etc.).
  - Exposes dedicated tools for full status, ship/SRV flags, navigation, cockpit systems, and Odyssey on-foot states, plus MCP resources.
- **Flexible Configuration**:
  - Configurable via `config.ini` located next to the binary or in the working directory.
  - Automatically determines the default Elite Dangerous path on Windows and Linux if unconfigured.
  - Full CLI flag overrides.

---

## Installation & Building

Requires **Go 1.24+** (and GNU `make` optionally).

### Build with `make`

```bash
# Build native binary for host OS (into bin/ed-assist)
make build

# Cross-compile 64-bit Windows binary (bin/ed-assist.exe)
make windows

# Cross-compile Windows ARM64 binary (bin/ed-assist-arm64.exe)
make windows-arm64

# Build for Linux 64-bit (bin/ed-assist-linux-amd64)
make linux

# Run unit tests
make test

# Clean build artifacts
make clean
```

### Direct Go Build

```bash
# Native build
go build -o ed-assist ./cmd/ed-assist

# Windows cross-compilation
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ed-assist.exe ./cmd/ed-assist
```

---

## Usage

Run `ed-assist` directly:

```bash
./ed-assist
```

On startup, `ed-assist` logs its configuration and the resolved path to `Status.json`:

```text
time=... level=INFO msg="starting ed-assist" mode=watch poll_interval=250ms max_retries=3 retry_delay=25ms
time=... level=INFO msg="expected status file path" path="..." source="auto-determined"
```

When an update is detected, it displays a formatted summary:

```text
--------------------------------------------------------------------------------
[2024-03-01T12:00:00Z] Mode: Ship | GUI: Station Services | FireGroup: 0
  Pips: [SYS: 2.0 | ENG: 2.0 | WEP: 2.0] | Fuel: Main 32.00t / Res 0.85t | Cargo: 12.5t
  Legal State: Clean | Balance: 542000100 CR
  Destination: Sol (Body: 1, System: 12345678)
  Flags: Docked, LandingGearDown, ShieldsUp, FSDMassLocked, InMainShip
--------------------------------------------------------------------------------
```

---

## Configuration

### Command Line Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `-mode` | `string` | `watch` | Update mode: `watch` (event-driven) or `poll` (ticker) |
| `-interval` | `duration` | `250ms` | Polling interval when `-mode poll` is selected |
| `-retries` | `int` | `3` | Maximum quick retries on read/parse failure |
| `-retry-delay` | `duration` | `25ms` | Delay time between quick retries (e.g. `25ms`, `50ms`) |
| `-db` | `string` | `""` | Path to SQLite database file (default: `ed_assist.db` next to binary) |
| `-mcp` | `bool` | `false` | Enable MCP (Model Context Protocol) server on stdio |
| `-status` | `string` | `""` | Direct override for the `Status.json` path |
| `-config` | `string` | `""` | Path to custom `config.ini` |
| `-loglevel` | `string` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `-logfile` | `string` | `""` | Optional file to mirror log output to |

### `config.ini`

Place a `config.ini` next to the binary or in the current working directory (see [`config.ini.example`](file:///home/gerifield/ed-assist/config.ini.example)):

```ini
[general]
; Custom path to Status.json (leave empty for auto-detection)
status_file = 

; SQLite database path for visited and targeted tracking (default: ed_assist.db next to binary)
db_path = 

; Detection mode: "watch" (event-driven) or "poll" (periodic)
mode = watch

; Polling interval in ms (active when mode = poll)
poll_interval_ms = 250

; Quick retry attempts and delay (e.g. 25ms, 50ms)
max_retries = 3
retry_time_ms = 25

; Enable MCP server mode
enable_mcp = false

; Logging
loglevel = info
; logfile = ed-assist.log
```

---

## Model Context Protocol (MCP) Server

When started with `-mcp`, `ed-assist` runs an MCP server over standard I/O (`stdio`). This allows AI assistants and IDEs to inspect your real-time Elite Dangerous status.

### Running with MCP
```bash
./ed-assist -mcp
```

### Client Configuration Example (e.g. Claude Desktop / Cursor)

Add to your MCP settings configuration:

```json
{
  "mcpServers": {
    "ed-assist": {
      "command": "/path/to/ed-assist",
      "args": ["-mcp"]
    }
  }
}
```

### Exposed MCP Tools

| Tool | Description |
|---|---|
| `get_status` | Returns the complete status as JSON (ship, cockpit, coordinates, destination, balance, flags). |
| `get_status_summary` | Returns a human-readable formatted text summary of the current state. |
| `get_ship_status` | Returns flight mode (Ship/SRV/Fighter/On Foot), legal state, and all active status flags. |
| `get_cockpit` | Returns power pips (SYS, ENG, WEP), fuel (Main and Reservoir in tons), cargo mass, and GUI focus screen. |
| `get_navigation` | Returns body name, latitude, longitude, altitude, heading, planet radius, and targeted destination. |
| `get_on_foot` | Returns Odyssey on-foot metrics: health, oxygen, temperature, gravity, weapon, and on-foot flags. |
| `get_visited_systems` | Returns the latest visited star systems history (up to 100) from the SQLite database. |
| `get_targeted_systems` | Returns the latest targeted star systems/destinations history (up to 100) from the SQLite database. |

### Exposed MCP Resources

| Resource URI | MIME Type | Description |
|---|---|---|
| `ed://status/current` | `application/json` | Raw JSON of latest status snapshot. |
| `ed://status/summary` | `text/plain` | Clean text summary of current status. |
| `ed://systems/visited` | `application/json` | JSON list of up to 100 latest visited star systems. |
| `ed://systems/targeted` | `application/json` | JSON list of up to 100 latest targeted destinations. |

---

## Project Structure

```
ed-assist/
├── cmd/
│   └── ed-assist/
│       └── main.go              # Application entrypoint, CLI flags, display output & MCP runner
├── internal/
│   ├── config/
│   │   ├── config.go            # config.ini parsing and path auto-detection
│   │   └── config_test.go
│   ├── flags/
│   │   └── flags.go             # Bitmask constants for Flags, Flags2, and GuiFocus
│   ├── mcpserver/
│   │   ├── server.go            # MCP server implementation, tools, and resources
│   │   └── server_test.go
│   ├── parser/
│   │   ├── parser.go            # Status struct, flags unpacking, summary formatting
│   │   └── parser_test.go
│   ├── reader/
│   │   ├── reader.go            # Status reader (watch/poll) with quick retry logic
│   │   └── reader_test.go
│   ├── store/
│   │   ├── store.go             # SQLite store for visited and targeted systems
│   │   └── store_test.go
│   ├── tracker/
│   │   ├── tracker.go           # Journal & Status.json event tracking for systems
│   │   └── tracker_test.go
│   └── watcher/
│       ├── watcher.go           # Cross-platform fsnotify watcher
│       └── watcher_test.go
├── Makefile                     # Build & Windows cross-compilation recipes
├── config.ini.example           # Example configuration template
├── go.mod
└── go.sum
```
