package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ed-assist/internal/edapi"
	"ed-assist/internal/input"
	"ed-assist/internal/parser"
	"ed-assist/internal/reader"
	"ed-assist/internal/store"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StatusProvider defines an interface for fetching Elite Dangerous status data.
type StatusProvider interface {
	LastStatus() *parser.Status
	ReadOnce() (*parser.Status, error)
}

// Option configures MCPServer.
type Option func(*MCPServer)

// WithCacheTTL sets external API cache TTL.
func WithCacheTTL(ttl time.Duration) Option {
	return func(s *MCPServer) {
		if ttl > 0 {
			s.cacheTTL = ttl
		}
	}
}

// WithEDAPIClient overrides the EDAPI client.
func WithEDAPIClient(client *edapi.Client) Option {
	return func(s *MCPServer) {
		if client != nil {
			s.edapi = client
		}
	}
}

// VehicleStateProvider provides the latest vehicle state inferred from Journal events.
type VehicleStateProvider interface {
	LatestVehicleState() (mode string, event string, timestamp time.Time, ok bool)
}

// WithVehicleProvider supplies a VehicleStateProvider (e.g. Journal tracker) for event timestamp alignment.
func WithVehicleProvider(vp VehicleStateProvider) Option {
	return func(s *MCPServer) {
		s.vehicleProvider = vp
	}
}

// MCPServer wraps the mark3labs MCP server for Elite Dangerous.
type MCPServer struct {
	server          *server.MCPServer
	provider        StatusProvider
	vehicleProvider VehicleStateProvider
	store           *store.Store
	controller      *input.Controller
	edapi           *edapi.Client
	cacheTTL        time.Duration
}

// New creates and configures an MCP server exposing Elite Dangerous game status and controls.
func New(provider StatusProvider, st *store.Store, ctrl *input.Controller, opts ...Option) *MCPServer {
	s := &MCPServer{
		server: server.NewMCPServer(
			"ed-assist",
			"1.0.0",
			server.WithToolCapabilities(false),
			server.WithResourceCapabilities(false, false),
			server.WithDescription("Elite Dangerous Status Assistant - exposes live ship data, in-game control, and galaxy intelligence"),
		),
		provider:   provider,
		store:      st,
		controller: ctrl,
		cacheTTL:   8 * time.Hour,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.edapi == nil {
		s.edapi = edapi.NewClient(
			edapi.WithCacher(st),
			edapi.WithCacheTTL(s.cacheTTL),
		)
	}

	s.registerTools()
	s.registerResources()
	return s
}

// getStatus helper retrieves current status from cache (kept fresh in memory by the background reader goroutine),
// falling back to a direct read only if the cache has not been populated yet.
// If a Journal event has a newer timestamp than Status.json, the Journal-confirmed vehicle mode takes precedence.
func (s *MCPServer) getStatus() (*parser.Status, error) {
	var st *parser.Status
	// 1. Fast in-memory cache read (maintained by background watcher / polling thread)
	if last := s.provider.LastStatus(); last != nil {
		st = last
	} else {
		// Fallback to direct read on startup if background tick hasn't run yet
		fresh, err := s.provider.ReadOnce()
		if err != nil {
			return nil, fmt.Errorf("status data is not yet available (is Elite Dangerous running?): %w", err)
		}
		st = fresh
	}

	// 2. Check if a Journal vehicle event (e.g. DockSRV, DockFighter) has a newer timestamp than Status.json
	if s.vehicleProvider != nil {
		if jMode, jEvent, jTs, ok := s.vehicleProvider.LatestVehicleState(); ok && !jTs.IsZero() {
			if stTime, err := st.ParsedTime(); err == nil {
				if jTs.After(stTime) {
					slog.Debug("overriding vehicle mode from newer Journal event",
						"journal_mode", jMode,
						"journal_event", jEvent,
						"journal_timestamp", jTs,
						"status_timestamp", stTime,
					)
					st.SetOverrideMode(jMode)
				}
			}
		}
	}

	return st, nil
}

func (s *MCPServer) registerTools() {
	// Tool 1: get_status (Full status)
	s.server.AddTool(
		mcp.NewTool("get_status",
			mcp.WithDescription("Get the complete live status of the player, ship/SRV, cockpit, and navigation target from Elite Dangerous as JSON"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st, err := s.getStatus()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			bytes, err := json.MarshalIndent(st, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to serialize status: %v", err)), nil
			}
			return mcp.NewToolResultText(string(bytes)), nil
		},
	)

	// Tool 2: get_status_summary (Human-readable text summary)
	s.server.AddTool(
		mcp.NewTool("get_status_summary",
			mcp.WithDescription("Get a clean, human-readable text summary of the current Elite Dangerous game state"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st, err := s.getStatus()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(st.Summary()), nil
		},
	)

	// Tool 3: get_ship_status (Ship/SRV mode, active flags)
	s.server.AddTool(
		mcp.NewTool("get_ship_status",
			mcp.WithDescription("Get ship and vehicle flight state: current mode (Ship/SRV/Fighter/On Foot), legal state, and active flags (docked, landed, gear, shields, supercruise, hardpoints, FAOff, silent running, etc.)"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st, err := s.getStatus()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			payload := map[string]any{
				"timestamp":    st.Timestamp,
				"mode":         st.Mode(),
				"legal_state":  st.LegalState,
				"active_flags": st.ActiveFlags(),
				"flags":        st.Flags,
			}
			bytes, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(bytes)), nil
		},
	)

	// Tool 4: get_cockpit (Power pips, fuel, cargo, GUI focus, firegroup)
	s.server.AddTool(
		mcp.NewTool("get_cockpit",
			mcp.WithDescription("Get cockpit systems status: power pips distribution (SYS, ENG, WEP), fuel levels (Main and Reservoir in tons), cargo mass (tons), balance (credits), fire group, and active GUI focus menu"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st, err := s.getStatus()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			payload := map[string]any{
				"timestamp":  st.Timestamp,
				"mode":       st.Mode(),
				"fire_group": st.FireGroup,
				"gui_focus":  st.GuiFocusName(),
				"cargo_tons": st.Cargo,
				"balance_cr": st.Balance,
				"pips": map[string]float64{
					"sys": st.PipsSys(),
					"eng": st.PipsEng(),
					"wep": st.PipsWep(),
				},
				"fuel": map[string]float64{
					"main_tons":      st.Fuel.FuelMain,
					"reservoir_tons": st.Fuel.FuelReservoir,
				},
			}
			bytes, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(bytes)), nil
		},
	)

	// Tool 5: get_navigation (Planetary coordinates, body name, targeted destination)
	s.server.AddTool(
		mcp.NewTool("get_navigation",
			mcp.WithDescription("Get navigation and planetary position data: body name, latitude, longitude, altitude (meters), heading (degrees), planet radius, and current targeted destination"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st, err := s.getStatus()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			payload := map[string]any{
				"timestamp":     st.Timestamp,
				"mode":          st.Mode(),
				"body_name":     st.BodyName,
				"latitude":      st.Latitude,
				"longitude":     st.Longitude,
				"altitude":      st.Altitude,
				"heading":       st.Heading,
				"planet_radius": st.PlanetRadius,
				"destination":   st.Destination,
			}
			bytes, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(bytes)), nil
		},
	)

	// Tool 6: get_on_foot (Odyssey on-foot stats)
	s.server.AddTool(
		mcp.NewTool("get_on_foot",
			mcp.WithDescription("Get Odyssey on-foot data: health, oxygen, temperature (Kelvin), gravity (relative to 1G), selected weapon, and on-foot environment flags"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st, err := s.getStatus()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			weapon := st.SelectedWeaponLocalised
			if weapon == "" {
				weapon = st.SelectedWeapon
			}

			payload := map[string]any{
				"timestamp":             st.Timestamp,
				"is_on_foot":            st.Flags2.OnFoot,
				"health":                st.Health,
				"oxygen":                st.Oxygen,
				"temperature_k":         st.Temperature,
				"gravity_g":             st.Gravity,
				"selected_weapon":       weapon,
				"breathable_atmosphere": st.Flags2.BreathableAtmosphere,
				"active_flags_odyssey":  st.ActiveFlags2(),
			}
			bytes, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(bytes)), nil
		},
	)

	// Tool 7: get_visited_systems (History of visited star systems from SQLite)
	s.server.AddTool(
		mcp.NewTool("get_visited_systems",
			mcp.WithDescription("Get the history of visited star systems (latest up to 100 entries, stored in SQLite database)"),
			mcp.WithNumber("limit", mcp.Description("Maximum number of visited systems to return (1-100, default 100)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if s.store == nil {
				return mcp.NewToolResultError("database tracking is not enabled (run with -track or set enable_tracking = true in config.ini)"), nil
			}
			defLimit := s.store.MaxVisited()
			limit := request.GetInt("limit", defLimit)
			visited, err := s.store.GetVisited(limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed retrieving visited systems: %v", err)), nil
			}
			jsonStr, err := store.FormatVisitedJSON(visited)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(jsonStr), nil
		},
	)

	// Tool 8: get_targeted_systems (History of targeted star systems from SQLite)
	s.server.AddTool(
		mcp.NewTool("get_targeted_systems",
			mcp.WithDescription("Get the history of targeted destinations and star systems (stored in SQLite database)"),
			mcp.WithNumber("limit", mcp.Description("Maximum number of targeted systems to return")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if s.store == nil {
				return mcp.NewToolResultError("database tracking is not enabled (run with -track or set enable_tracking = true in config.ini)"), nil
			}
			defLimit := s.store.MaxTargeted()
			limit := request.GetInt("limit", defLimit)
			targeted, err := s.store.GetTargeted(limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed retrieving targeted systems: %v", err)), nil
			}
			jsonStr, err := store.FormatTargetedJSON(targeted)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(jsonStr), nil
		},
	)

	// Tool 9: send_game_command (Execute in-game command via DirectInput scancode)
	s.server.AddTool(
		mcp.NewTool("send_game_command",
			mcp.WithDescription("Send a discrete in-game flight or cockpit command to Elite Dangerous via DirectInput hardware scancodes (e.g. landing_gear, hardpoints, cargo_scoop, lights, night_vision, boost, fsd, target, next_target, pips_sys, pips_eng, pips_wep, pips_reset, or any raw action from .binds)"),
			mcp.WithString("action", mcp.Required(), mcp.Description("Action name or friendly alias to execute (e.g. 'landing_gear', 'hardpoints', 'cargo_scoop', 'lights', 'night_vision', 'boost', 'fsd', 'target', 'pips_sys', 'pips_eng', 'pips_wep', 'pips_reset')")),
			mcp.WithNumber("hold_ms", mcp.Description("Key hold duration in milliseconds (optional, defaults to key_hold_ms from config.ini, default: 80ms)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if s.controller == nil {
				return mcp.NewToolResultError("game control is not enabled (set game_control = true under [general] in config.ini)"), nil
			}
			action := request.GetString("action", "")
			if action == "" {
				return mcp.NewToolResultError("action parameter is required"), nil
			}
			holdMs := request.GetInt("hold_ms", 0)
			res, err := s.controller.ExecuteAction(action, holdMs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("command failed: %v", err)), nil
			}
			bytes, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Action %q executed successfully:\n%s", res.ActionName, string(bytes))), nil
		},
	)

	// Tool 10: list_game_commands (List available bound actions)
	s.server.AddTool(
		mcp.NewTool("list_game_commands",
			mcp.WithDescription("List all available in-game actions and key bindings mapped from the player's active Elite Dangerous .binds file"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if s.controller == nil {
				return mcp.NewToolResultError("game control is not enabled (set game_control = true under [general] in config.ini)"), nil
			}
			jsonStr, err := s.controller.FormatActionsJSON()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(jsonStr), nil
		},
	)

	// Tool 11: search_system (Query star system information from EDSM)
	s.server.AddTool(
		mcp.NewTool("search_system",
			mcp.WithDescription("Look up star system information (3D coordinates, allegiance, government, economy, population, primary star) from EDSM. If system_name is omitted, defaults to the current target or current star system."),
			mcp.WithString("system_name", mcp.Description("Name of the star system to look up (e.g. 'Sol', 'Colonia', 'Shinrarta Dezhra'). Defaults to current target/ship location if omitted.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sysName := s.resolveSystemName(request.GetString("system_name", ""))
			if sysName == "" {
				return mcp.NewToolResultError("system_name was not provided and could not be determined from current telemetry"), nil
			}
			res, err := s.edapi.SearchSystem(ctx, sysName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("system lookup failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)

	// Tool 12: nearest_systems (Find systems within a sphere radius from EDSM)
	s.server.AddTool(
		mcp.NewTool("nearest_systems",
			mcp.WithDescription("Find star systems within a given radius (up to 100 light years) of a center star system or coordinates using EDSM."),
			mcp.WithString("system_name", mcp.Description("Center star system name. If omitted, uses current target or ship position.")),
			mcp.WithNumber("radius", mcp.Description("Search sphere radius in light years (ly), up to 100 ly (default: 25 ly)")),
			mcp.WithBoolean("only_populated", mcp.Description("If true, filters out unpopulated systems to return only inhabited systems")),
			mcp.WithNumber("x", mcp.Description("Galactic X coordinate (optional if system_name not given)")),
			mcp.WithNumber("y", mcp.Description("Galactic Y coordinate (optional if system_name not given)")),
			mcp.WithNumber("z", mcp.Description("Galactic Z coordinate (optional if system_name not given)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sysName := strings.TrimSpace(request.GetString("system_name", ""))
			radius := request.GetFloat("radius", 25.0)
			onlyPop := request.GetBool("only_populated", false)

			args := request.GetArguments()
			var xPtr, yPtr, zPtr *float64
			if xVal, ok := args["x"].(float64); ok {
				xPtr = &xVal
			}
			if yVal, ok := args["y"].(float64); ok {
				yPtr = &yVal
			}
			if zVal, ok := args["z"].(float64); ok {
				zPtr = &zVal
			}

			// If no systemName and no coordinates, try resolving from telemetry or store
			if sysName == "" && (xPtr == nil || yPtr == nil || zPtr == nil) {
				sysName = s.resolveSystemName("")
				if sysName == "" && s.store != nil {
					if visited, err := s.store.GetVisited(1); err == nil && len(visited) > 0 {
						if visited[0].StarPosX != nil && visited[0].StarPosY != nil && visited[0].StarPosZ != nil {
							xPtr = visited[0].StarPosX
							yPtr = visited[0].StarPosY
							zPtr = visited[0].StarPosZ
						}
					}
				}
			}

			if sysName == "" && (xPtr == nil || yPtr == nil || zPtr == nil) {
				return mcp.NewToolResultError("either system_name or (x, y, z) coordinates must be provided"), nil
			}

			res, err := s.edapi.NearestSystems(ctx, sysName, xPtr, yPtr, zPtr, radius, onlyPop)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("nearest systems lookup failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)

	// Tool 13: system_stations (List stations, outposts, settlements from EDSM)
	s.server.AddTool(
		mcp.NewTool("system_stations",
			mcp.WithDescription("List all starports, planetary outposts, settlements, and fleet carriers in a star system from EDSM, including commodity market, shipyard, and outfitting availability flags."),
			mcp.WithString("system_name", mcp.Description("Name of the star system. Defaults to current target/ship location if omitted.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sysName := s.resolveSystemName(request.GetString("system_name", ""))
			if sysName == "" {
				return mcp.NewToolResultError("system_name was not provided and could not be determined from current telemetry"), nil
			}
			res, err := s.edapi.SystemStations(ctx, sysName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("system stations lookup failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)

	// Tool 14: station_market (Retrieve commodity market prices from EDSM)
	s.server.AddTool(
		mcp.NewTool("station_market",
			mcp.WithDescription("Get live commodity market prices (buy/sell prices, stock, demand) at a specific station from EDSM. Useful for trade and mining sales."),
			mcp.WithString("system_name", mcp.Required(), mcp.Description("Name of the star system (e.g. 'Sol', 'Shinrarta Dezhra')")),
			mcp.WithString("station_name", mcp.Required(), mcp.Description("Name of the station or planetary port (e.g. 'Daedalus', 'Jameson Memorial')")),
			mcp.WithString("filter_commodity", mcp.Description("Optional commodity name filter (e.g. 'Tritium', 'Gold', 'Painite') to filter results")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sysName := request.GetString("system_name", "")
			stnName := request.GetString("station_name", "")
			filter := request.GetString("filter_commodity", "")
			if sysName == "" || stnName == "" {
				return mcp.NewToolResultError("both system_name and station_name are required"), nil
			}
			res, err := s.edapi.StationMarket(ctx, sysName, stnName, filter)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("station market lookup failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)

	// Tool 15: system_factions (Get faction influence & BGS states from EDSM)
	s.server.AddTool(
		mcp.NewTool("system_factions",
			mcp.WithDescription("Get Background Simulation (BGS) faction influence, allegiance, and government states for minor factions present in a star system from EDSM."),
			mcp.WithString("system_name", mcp.Description("Name of the star system. Defaults to current target/ship location if omitted.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sysName := s.resolveSystemName(request.GetString("system_name", ""))
			if sysName == "" {
				return mcp.NewToolResultError("system_name was not provided and could not be determined from current telemetry"), nil
			}
			res, err := s.edapi.SystemFactions(ctx, sysName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("system factions lookup failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)

	// Tool 16: system_bodies (Get celestial bodies from EDSM)
	s.server.AddTool(
		mcp.NewTool("system_bodies",
			mcp.WithDescription("Get celestial bodies (stars, planets, moons, gravity, landable status, rings, atmosphere) in a star system from EDSM."),
			mcp.WithString("system_name", mcp.Description("Name of the star system. Defaults to current target/ship location if omitted.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sysName := s.resolveSystemName(request.GetString("system_name", ""))
			if sysName == "" {
				return mcp.NewToolResultError("system_name was not provided and could not be determined from current telemetry"), nil
			}
			res, err := s.edapi.SystemBodies(ctx, sysName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("system bodies lookup failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)

	// Tool 17: plot_neutron_route (High-speed neutron highway routing via Spansh)
	s.server.AddTool(
		mcp.NewTool("plot_neutron_route",
			mcp.WithDescription("Plot a long-distance neutron highway jump route between two systems using the Spansh neutron router API."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Origin star system name (or 'current' to use current ship location)")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Destination star system name (e.g. 'Colonia', 'Sagittarius A*')")),
			mcp.WithNumber("range", mcp.Required(), mcp.Description("Ship uncharged jump range in light years (e.g. 50.0, 65.5)")),
			mcp.WithNumber("efficiency", mcp.Description("Routing efficiency percentage from 1 to 100 (default: 60)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			from := strings.TrimSpace(request.GetString("from", ""))
			if strings.EqualFold(from, "current") || from == "" {
				from = s.resolveSystemName("")
			}
			to := strings.TrimSpace(request.GetString("to", ""))
			jumpRange := request.GetFloat("range", 50.0)
			efficiency := request.GetInt("efficiency", 60)

			if from == "" || to == "" {
				return mcp.NewToolResultError("both 'from' and 'to' systems are required"), nil
			}

			res, err := s.edapi.PlotNeutronRoute(ctx, from, to, jumpRange, efficiency)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("neutron route planning failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)

	// Tool 18: get_server_status (Elite Dangerous server status from EDSM)
	s.server.AddTool(
		mcp.NewTool("get_server_status",
			mcp.WithDescription("Check current Elite Dangerous game server operational status from EDSM."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			res, err := s.edapi.ServerStatus(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("server status lookup failed: %v", err)), nil
			}
			return mcp.NewToolResultText(res), nil
		},
	)
}

func (s *MCPServer) resolveSystemName(specified string) string {
	if specified = strings.TrimSpace(specified); specified != "" {
		return specified
	}
	// 1. Active targeted destination from live status
	if st, err := s.getStatus(); err == nil && st != nil {
		if st.Destination != nil && st.Destination.Name != "" {
			return st.Destination.Name
		}
	}
	// 2. History: most recent visited star system (current location in galaxy)
	if s.store != nil {
		if visited, err := s.store.GetVisited(1); err == nil && len(visited) > 0 && visited[0].SystemName != "" {
			return visited[0].SystemName
		}
		// Fallback to most recent targeted destination in history
		if targeted, err := s.store.GetTargeted(1); err == nil && len(targeted) > 0 && targeted[0].SystemName != "" {
			return targeted[0].SystemName
		}
	}
	// 3. Fallback: inspect latest Journal log in the status/journal directory
	if fallbackSys := s.findLatestSystemFromJournals(); fallbackSys != "" {
		return fallbackSys
	}
	return ""
}

func (s *MCPServer) findLatestSystemFromJournals() string {
	var journalDir string
	if fp, ok := s.provider.(interface{ FilePath() string }); ok && fp.FilePath() != "" {
		journalDir = filepath.Dir(fp.FilePath())
	}
	if journalDir == "" {
		if userHome, err := os.UserHomeDir(); err == nil && userHome != "" {
			journalDir = filepath.Join(userHome, "Saved Games", "Frontier Developments", "Elite Dangerous")
		}
	}
	if journalDir == "" {
		return ""
	}

	files, err := filepath.Glob(filepath.Join(journalDir, "Journal.*.log"))
	if err != nil || len(files) == 0 {
		return ""
	}

	sort.Strings(files)
	for i := len(files) - 1; i >= 0 && i >= len(files)-3; i-- {
		if sys := extractLatestSystemFromJournal(files[i]); sys != "" {
			return sys
		}
	}
	return ""
}

func extractLatestSystemFromJournal(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var latestSystem string
	var latestTarget string

	type miniEvent struct {
		Event      string `json:"event"`
		StarSystem string `json:"StarSystem,omitempty"`
		Name       string `json:"Name,omitempty"`
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		var ev miniEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Event == "Location" || ev.Event == "FSDJump" || ev.Event == "CarrierJump" {
			if ev.StarSystem != "" {
				latestSystem = ev.StarSystem
			}
		} else if ev.Event == "FSDTarget" && ev.Name != "" {
			latestTarget = ev.Name
		}
	}

	if latestTarget != "" {
		return latestTarget
	}
	return latestSystem
}

func (s *MCPServer) registerResources() {
	// Resource 1: ed://status/current
	s.server.AddResource(
		mcp.NewResource(
			"ed://status/current",
			"Current Elite Dangerous Status",
			mcp.WithResourceDescription("Raw JSON format of the latest Status.json snapshot"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			st, err := s.getStatus()
			if err != nil {
				return nil, err
			}
			bytes, err := json.MarshalIndent(st, "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "ed://status/current",
					MIMEType: "application/json",
					Text:     string(bytes),
				},
			}, nil
		},
	)

	// Resource 2: ed://status/summary
	s.server.AddResource(
		mcp.NewResource(
			"ed://status/summary",
			"Elite Dangerous Status Summary",
			mcp.WithResourceDescription("Formatted text summary of the current Elite Dangerous game status"),
			mcp.WithMIMEType("text/plain"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			st, err := s.getStatus()
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "ed://status/summary",
					MIMEType: "text/plain",
					Text:     st.Summary(),
				},
			}, nil
		},
	)

	// Resource 3: ed://systems/visited
	s.server.AddResource(
		mcp.NewResource(
			"ed://systems/visited",
			"Visited Star Systems",
			mcp.WithResourceDescription("Latest 100 visited star systems stored in local SQLite database"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			if s.store == nil {
				return nil, fmt.Errorf("database tracking is not enabled (run with -track or set enable_tracking = true in config.ini)")
			}
			visited, err := s.store.GetVisited(s.store.MaxVisited())
			if err != nil {
				return nil, err
			}
			jsonStr, err := store.FormatVisitedJSON(visited)
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "ed://systems/visited",
					MIMEType: "application/json",
					Text:     jsonStr,
				},
			}, nil
		},
	)

	// Resource 4: ed://systems/targeted
	s.server.AddResource(
		mcp.NewResource(
			"ed://systems/targeted",
			"Targeted Systems",
			mcp.WithResourceDescription("Latest targeted systems/destinations stored in local SQLite database"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			if s.store == nil {
				return nil, fmt.Errorf("database tracking is not enabled (run with -track or set enable_tracking = true in config.ini)")
			}
			targeted, err := s.store.GetTargeted(s.store.MaxTargeted())
			if err != nil {
				return nil, err
			}
			jsonStr, err := store.FormatTargetedJSON(targeted)
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "ed://systems/targeted",
					MIMEType: "application/json",
					Text:     jsonStr,
				},
			}, nil
		},
	)

	// Resource 5: ed://controls/commands
	s.server.AddResource(
		mcp.NewResource(
			"ed://controls/commands",
			"Configured Game Commands",
			mcp.WithResourceDescription("List of all active keyboard commands and shortcuts mapped from player's .binds file"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			if s.controller == nil {
				return nil, fmt.Errorf("game control is not enabled (set game_control = true in config.ini)")
			}
			jsonStr, err := s.controller.FormatActionsJSON()
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "ed://controls/commands",
					MIMEType: "application/json",
					Text:     jsonStr,
				},
			}, nil
		},
	)
}

// ServeStdio starts the MCP server over standard input and output streams.
func (s *MCPServer) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	slog.Info("starting MCP server on stdio")
	stdioServer := server.NewStdioServer(s.server)
	return stdioServer.Listen(ctx, in, out)
}

// ServeHTTP starts the MCP server over HTTP (SSE transport) on the specified listen address.
func (s *MCPServer) ServeHTTP(ctx context.Context, addr string) error {
	slog.Info("starting MCP server over HTTP/SSE",
		"addr", addr,
		"sse_endpoint", fmt.Sprintf("http://%s/sse", addr),
		"message_endpoint", fmt.Sprintf("http://%s/message", addr),
	)

	sseServer := server.NewSSEServer(
		s.server,
		server.WithSSECORS(server.WithCORSAllowedOrigins("*")),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":             "ed-assist",
				"version":          "1.0.0",
				"status":           "running",
				"transport":        "sse",
				"sse_endpoint":     "/sse",
				"message_endpoint": "/message",
				"tools":            10,
				"resources":        5,
			})
			return
		}
		sseServer.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	errChan := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = sseServer.Shutdown(shutdownCtx)
		return srv.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

// Server returns the underlying MCPServer.
func (s *MCPServer) Server() *server.MCPServer {
	return s.server
}

// Ensure reader.Reader implements StatusProvider
var _ StatusProvider = (*reader.Reader)(nil)
