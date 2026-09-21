package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ed-assist/internal/config"
	"ed-assist/internal/gemini"
	"ed-assist/internal/input"
	"ed-assist/internal/mcpserver"
	"ed-assist/internal/reader"
	"ed-assist/internal/store"
	"ed-assist/internal/tracker"
	"ed-assist/internal/web"
)

var Version = "dev"

func main() {
	// Parse CLI flags
	configFileFlag := flag.String("config", "", "Path to config.ini file")
	webAddrFlag := flag.String("addr", "", "Listen address for web UI (default: 127.0.0.1:3000)")
	mcpModeFlag := flag.String("mcp-mode", "", "MCP transport mode: http, stdio, or inprocess (default: inprocess/http)")
	mcpEndpointFlag := flag.String("mcp-endpoint", "", "MCP HTTP endpoint (e.g. http://127.0.0.1:8080/sse) or path to binary for stdio")
	apiKeyFlag := flag.String("api-key", "", "Gemini API key (or GEMINI_API_KEY env)")
	modelFlag := flag.String("model", "", "Gemini model (default: gemini-flash-lite-latest)")
	statusFileFlag := flag.String("status", "", "Override path to Status.json")
	dbPathFlag := flag.String("db", "", "Path to SQLite database file")
	cacheHoursFlag := flag.Int("cache-hours", 0, "System info and external API cache TTL in hours (default: 8)")
	logLevelFlag := flag.String("loglevel", "info", "Log level (debug, info, warn, error)")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("ed-assist-web %s\n", Version)
		return
	}

	// Load configuration
	cfg, err := config.Load(*configFileFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply CLI overrides
	if *webAddrFlag != "" {
		cfg.WebAddr = *webAddrFlag
	}
	if *mcpModeFlag != "" {
		cfg.GeminiMCPMode = *mcpModeFlag
	}
	if *mcpEndpointFlag != "" {
		cfg.GeminiMCPEndpoint = *mcpEndpointFlag
	}
	if *apiKeyFlag != "" {
		cfg.GeminiAPIKey = *apiKeyFlag
	}
	if *modelFlag != "" {
		cfg.GeminiModel = *modelFlag
	}
	if *statusFileFlag != "" {
		cfg.StatusFilePath = *statusFileFlag
	}
	if *dbPathFlag != "" {
		cfg.DBPath = *dbPathFlag
		cfg.EnableTracking = true
	}
	if *logLevelFlag != "" {
		cfg.LogLevel = *logLevelFlag
	}
	if *cacheHoursFlag > 0 {
		cfg.SystemCacheHours = *cacheHoursFlag
		cfg.SystemCacheTTL = time.Duration(*cacheHoursFlag) * time.Hour
	}

	// Set up logger
	opts := &slog.HandlerOptions{}
	switch cfg.LogLevel {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	default:
		opts.Level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)

	slog.Info("starting ed-assist-web cockpit assistant",
		"web_addr", cfg.WebAddr,
		"model", cfg.GeminiModel,
		"mcp_mode", cfg.GeminiMCPMode,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize MCP Bridge according to mode
	var bridge *web.MCPBridge
	var statusReader *reader.Reader
	var sqliteStore *store.Store
	var sysTracker *tracker.Tracker

	switch cfg.GeminiMCPMode {
	case "stdio":
		binPath := cfg.GeminiMCPEndpoint
		if binPath == "" {
			binPath = "./ed-assist"
		}
		slog.Info("connecting to MCP server via stdio subprocess", "binary", binPath)
		b, err := web.NewStdioBridge(binPath)
		if err != nil {
			slog.Warn("could not connect to MCP stdio server", "error", err)
		} else {
			bridge = b
			defer bridge.Close()
		}

	case "http", "web", "sse":
		endpoint := cfg.GeminiMCPEndpoint
		if endpoint == "" {
			endpoint = "http://127.0.0.1:8080/sse"
		}
		slog.Info("connecting to external MCP server via HTTP/SSE", "endpoint", endpoint)
		b, err := web.NewHTTPBridge(endpoint)
		if err != nil {
			slog.Warn("could not connect to MCP HTTP/SSE server (is ed-assist running with mcp_transport=http?)", "endpoint", endpoint, "error", err)
		} else {
			bridge = b
			defer bridge.Close()
		}

	default: // "inprocess" - self-contained mode!
		slog.Info("running in-process MCP server and telemetry engine")
		if cfg.EnableTracking {
			st, err := store.New(cfg.DBPath)
			if err != nil {
				slog.Error("failed initializing SQLite store", "error", err)
			} else {
				sqliteStore = st
				defer sqliteStore.Close()
				sysTracker = tracker.New(sqliteStore, cfg.StatusFilePath)
				go sysTracker.Start(ctx)
			}
		}

		gameController := input.NewController(cfg.BindingsPath, cfg.KeyHoldMs, input.NewKeySender())

		statusReader = reader.New(
			cfg.StatusFilePath,
			reader.WithMode(reader.Mode(cfg.Mode)),
			reader.WithPollInterval(cfg.PollInterval),
			reader.WithRetries(cfg.MaxRetries, cfg.RetryDelay),
		)
		go statusReader.Start(ctx)
		defer func() {
			statusReader.Stop()
			statusReader.Wait()
		}()

		if sysTracker != nil {
			go func() {
				for st := range statusReader.StatusChan() {
					sysTracker.ProcessStatus(st)
				}
			}()
		}

		mcpSrv := mcpserver.New(statusReader, sqliteStore, gameController, mcpserver.WithCacheTTL(cfg.SystemCacheTTL))
		b, err := web.NewInProcessBridge(mcpSrv.Server())
		if err != nil {
			slog.Error("failed creating in-process MCP bridge", "error", err)
		} else {
			bridge = b
			defer bridge.Close()
		}
	}

	// Create Gemini client with MCP tools attached
	var optsList []gemini.Option
	if bridge != nil {
		optsList = append(optsList, gemini.WithMCPCaller(bridge))
	}
	geminiClient := gemini.NewClient(cfg.GeminiAPIKey, cfg.GeminiModel, optsList...)

	// Create and start web server
	webServer := web.NewServer(
		cfg.WebAddr,
		geminiClient,
		bridge,
		cfg.GeminiModel,
		cfg.VoiceGateThreshold,
		cfg.VoiceSilenceMs,
		web.WithEchoProtection(cfg.VoiceEchoProtection),
	)
	if err := webServer.Start(ctx); err != nil && ctx.Err() == nil {
		slog.Error("web server error", "error", err)
		os.Exit(1)
	}

	slog.Info("ed-assist-web stopped cleanly")
}
