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
  - Exposes all Elite Dangerous game status via standard MCP.
  - Supports dual transport modes: **`stdio`** (for Claude Desktop local commands) and **`http` (Server-Sent Events / SSE)** with concurrent live terminal event logging.
  - Works out-of-the-box with AI assistants and IDEs (Cursor, Claude Desktop, Antigravity, custom agents).
  - Exposes 10 tools and 5 resources covering telemetry, navigation, on-foot metrics, travel history, and ship controls.
- **In-Game Ship & Cockpit Control**:
  - Automatically parses player's active `.binds` XML file to resolve key mappings and modifiers.
  - Sends DirectInput hardware scancodes via Windows `SendInput` with configurable hold duration (`key_hold_ms`).
  - Allows AI copilots to toggle landing gear, hardpoints, lights, night vision, boost, FSD, targeting, and power distribution (pips).
- **Flexible Configuration**:
  - Configurable via `config.ini` located next to the binary or in the working directory.
  - Automatically determines the default Elite Dangerous path on Windows and Linux if unconfigured.
  - Full CLI flag overrides for quick adjustments.

---

## Why ed-assist? (Comparison with Existing Tools)

The Elite Dangerous community has built incredible software over the years. However, most established tools were conceived before modern AI protocols existed and often come with heavy runtime dependencies, complex graphical interfaces, or proprietary plugin systems.

`ed-assist` was designed from the ground up around a distinct philosophy: **minimalist, modular, AI-native, and self-contained**.

### At a Glance: Tool Comparison

| Feature | **ed-assist** | **EDMC** | **EDDI** | **VoiceAttack + HCS** | **EDDiscovery** |
|---|---|---|---|---|---|
| **Architecture** | **Single standalone Go binary** | Python + Tkinter GUI | Large .NET / C# application | Closed-source Windows app ($) | Heavy .NET desktop app |
| **Runtime Dependencies** | **None** (pure Go, CGO-free) | Python runtime + pip packages | .NET Framework / Desktop Runtime | Windows SAPI / macro engine | .NET Runtime + database engines |
| **Memory Footprint** | **~10–20 MB RAM** | ~60–120 MB RAM | ~150–350 MB RAM | ~80–150 MB RAM | ~300 MB–1+ GB RAM |
| **Idle CPU Usage** | **0% CPU** (OS filesystem events) | Polling / timer loops | Event hooks + speech synthesis loops | Audio listening loop | Polling + database sync |
| **AI Integration** | **Native MCP Server** (`stdio` & `http`/SSE) | None (REST / bespoke plugins only) | Speech responders only | Macro script triggers | None |
| **Two-Way Control** | **Yes** (DirectInput scancodes from `.binds`) | No (telemetry / upload only) | No (voice responses only) | Yes (pre-recorded macros) | No (read-only telemetry) |
| **System Tracking** | **Lightweight local SQLite** (latest 100) | Cloud relay (EDDN/Inara) | Internal SQLite / DB | None | Massive multi-GB local history DB |
| **Cross-Platform** | **Windows & Linux** (native & Proton) | Windows, Linux, macOS | Windows-centric | Windows only | Windows (Linux experimental) |

### Key Differentiators

1. **AI-Native via Open MCP Standard**
   - Implements the official [Model Context Protocol (MCP)](https://modelcontextprotocol.io/).
   - Connects directly to modern LLMs (Claude Desktop, Cursor, local Ollama/vLLM agents) without proprietary scripting languages or bespoke middleware.
   - Dual transport support: runs headless over `stdio` or as a background HTTP/SSE service (`mcp_transport = http`) so your terminal continues to display live formatted game event logs in real time.

2. **Modular & Composable**
   - Follows the Unix philosophy: do one thing exceptionally well.
   - Built as an interoperable building block that can be combined with other MCP servers (web search, notes, market calculators, custom voice agents) or custom frontends.

3. **True Two-Way Ship Interaction**
   - Most community tools only *read* game data. `ed-assist` bridges telemetry with execution:
   - Reads and auto-detects the player's active `.binds` XML file.
   - Translates high-level actions (`landing_gear`, `hardpoints`, `pips_sys`, `fsd`, `target`, etc.) into hardware DirectInput scancodes with microsecond-level hold timing (`key_hold_ms`).

4. **Zero-Dependency Single Binary**
   - Built in pure Go with an embedded pure-Go SQLite driver (`modernc.org/sqlite`).
   - No CGO, no Python environments, no .NET runtimes, no DLL hell. Download a single executable (`ed-assist` or `ed-assist.exe`), drop in a `config.ini`, and start flying.

5. **Future-Ready Roadmap**
   - Designed to grow incrementally with planned modular additions:
     - Automated route planning & neutron highway navigation tools.
     - Real-time commodity trading & market opportunity alerts.
     - Local Text-To-Speech (TTS) audio copilot output.
     - Web-based telemetry dashboard / HUD overlay.

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
| `-track` | `bool` | `false` | Enable optional SQLite tracking of visited and targeted systems (alias: `-tracking`) |
| `-db` | `string` | `""` | Path to SQLite database file (default: `ed_assist.db` next to binary) |
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

; Optional system tracking (latest 100 visited systems & 100 targeted destinations in SQLite)
enable_tracking = false

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

; MCP transport: "stdio" or "http" (default: "stdio")
; In "http" mode, live terminal events and logs remain active while serving MCP over HTTP/SSE
mcp_transport = stdio

; HTTP listen address when mcp_transport = http (default: 127.0.0.1:8080)
mcp_addr = 127.0.0.1:8080

; Optional in-game control via DirectInput hardware scancodes and active .binds file
; Enables MCP tools send_game_command and list_game_commands
game_control = false

; Key press hold duration in ms (default: 80ms)
key_hold_ms = 80

; Custom path to Elite Dangerous Bindings folder or .binds file (optional)
bindings_path = 

; Logging
loglevel = info
; logfile = ed-assist.log
```

---

## Model Context Protocol (MCP) Server

When configured with `enable_mcp = true` in `config.ini`, `ed-assist` runs an MCP server exposing real-time Elite Dangerous status, system history, and in-game controls to AI assistants.

### Transport Modes

`ed-assist` supports two MCP transport protocols selectable via `mcp_transport` in `config.ini`:

1. **`http` (Recommended for interactive play)**:
   - Starts an HTTP server supporting **Server-Sent Events (SSE)** at `http://127.0.0.1:8080/sse`.
   - **Full Terminal Event Logging**: The running terminal continues to print live formatted game status updates and event logs as you fly.
   - MCP clients (Cursor, web dashboards, remote agents) connect over HTTP.

2. **`stdio` (Default)**:
   - Operates over standard input and output streams (`stdin`/`stdout`).
   - Suitable when spawned directly as a subprocess by local desktop applications (like Claude Desktop).

### Running with MCP

Set `enable_mcp = true` in `config.ini` and launch `ed-assist`:
```bash
./ed-assist
```
Or specify a dedicated config file:
```bash
./ed-assist -config /path/to/mcp-config.ini
```

### Client Configuration Examples

#### HTTP / SSE Transport (Cursor / remote MCP clients)
```json
{
  "mcpServers": {
    "ed-assist": {
      "url": "http://127.0.0.1:8080/sse"
    }
  }
}
```

#### Stdio Transport (Claude Desktop local command)
```json
{
  "mcpServers": {
    "ed-assist": {
      "command": "/path/to/ed-assist"
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
| `send_game_command` | Sends a discrete flight or cockpit command via DirectInput hardware scancodes (`landing_gear`, `hardpoints`, `cargo_scoop`, `lights`, `night_vision`, `boost`, `fsd`, `target`, `pips_sys`, `pips_eng`, `pips_wep`, `pips_reset`, etc.). |
| `list_game_commands` | Lists all available in-game actions and key bindings mapped from the player's active `.binds` file. |

### Exposed MCP Resources

| Resource URI | MIME Type | Description |
|---|---|---|
| `ed://status/current` | `application/json` | Raw JSON of latest status snapshot. |
| `ed://status/summary` | `text/plain` | Clean text summary of current status. |
| `ed://systems/visited` | `application/json` | JSON list of up to 100 latest visited star systems. |
| `ed://systems/targeted` | `application/json` | JSON list of up to 100 latest targeted destinations. |
| `ed://controls/commands` | `application/json` | JSON list of all configured keyboard commands and aliases loaded from `.binds`. |

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
│   ├── input/
│   │   ├── binds.go             # Elite Dangerous .binds XML parser and preset detector
│   │   ├── binds_test.go
│   │   ├── controller.go        # Controller mapping actions to scancodes
│   │   ├── controller_test.go
│   │   ├── scancodes.go         # DirectInput hardware scancode table
│   │   ├── sender.go            # KeySender interface
│   │   ├── sender_windows.go    # Windows SendInput/keybd_event implementation
│   │   └── sender_other.go      # Non-Windows stub
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
