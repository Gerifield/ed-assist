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

; Detection mode: "watch" (event-driven) or "poll" (periodic)
mode = watch

; Polling interval in ms (active when mode = poll)
poll_interval_ms = 250

; Quick retry attempts and delay (e.g. 25ms, 50ms)
max_retries = 3
retry_time_ms = 25

; Logging
loglevel = info
; logfile = ed-assist.log
```

---

## Project Structure

```
ed-assist/
├── cmd/
│   └── ed-assist/
│       └── main.go              # Application entrypoint, CLI flags, display output
├── internal/
│   ├── config/
│   │   ├── config.go            # config.ini parsing and path auto-detection
│   │   └── config_test.go
│   ├── flags/
│   │   └── flags.go             # Bitmask constants for Flags, Flags2, and GuiFocus
│   ├── parser/
│   │   ├── parser.go            # Status struct, flags unpacking, summary formatting
│   │   └── parser_test.go
│   ├── reader/
│   │   ├── reader.go            # Status reader (watch/poll) with quick retry logic
│   │   └── reader_test.go
│   └── watcher/
│       ├── watcher.go           # Cross-platform fsnotify watcher
│       └── watcher_test.go
├── Makefile                     # Build & Windows cross-compilation recipes
├── config.ini.example           # Example configuration template
├── go.mod
└── go.sum
```
