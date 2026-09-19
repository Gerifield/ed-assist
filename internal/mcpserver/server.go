package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"ed-assist/internal/parser"
	"ed-assist/internal/reader"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StatusProvider is an interface for obtaining the current Elite Dangerous status.
type StatusProvider interface {
	LastStatus() *parser.Status
	ReadOnce() (*parser.Status, error)
}

// MCPServer wraps the mark3labs MCP server for Elite Dangerous.
type MCPServer struct {
	server   *server.MCPServer
	provider StatusProvider
}

// New creates and configures an MCP server exposing Elite Dangerous game status.
func New(provider StatusProvider) *MCPServer {
	s := &MCPServer{
		server: server.NewMCPServer(
			"ed-assist",
			"1.0.0",
			server.WithToolCapabilities(false),
			server.WithResourceCapabilities(false, false),
			server.WithDescription("Elite Dangerous Status Assistant - exposes live ship, navigation, cockpit, and on-foot game data"),
		),
		provider: provider,
	}

	s.registerTools()
	s.registerResources()
	return s
}

// getStatus helper retrieves current status from cache or performs a read.
func (s *MCPServer) getStatus() (*parser.Status, error) {
	st := s.provider.LastStatus()
	if st != nil {
		return st, nil
	}
	// Attempt fresh read if not cached yet
	st, err := s.provider.ReadOnce()
	if err != nil {
		return nil, fmt.Errorf("status data is not yet available (is Elite Dangerous running?): %w", err)
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
}

// ServeStdio starts the MCP server over standard input and output streams.
func (s *MCPServer) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	slog.Info("starting MCP server on stdio")
	stdioServer := server.NewStdioServer(s.server)
	return stdioServer.Listen(ctx, in, out)
}

// Server returns the underlying MCPServer.
func (s *MCPServer) Server() *server.MCPServer {
	return s.server
}

// Ensure reader.Reader implements StatusProvider
var _ StatusProvider = (*reader.Reader)(nil)
