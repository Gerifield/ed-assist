package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ed-assist/internal/config"
	"ed-assist/internal/input"
	"ed-assist/internal/mcpserver"
	"ed-assist/internal/parser"
	"ed-assist/internal/reader"
	"ed-assist/internal/store"
	"ed-assist/internal/tracker"
)

var globalLogLevel *slog.LevelVar

func setupLogging(level, logfile string, mcpMode bool) {
	globalLogLevel = &slog.LevelVar{}
	opts := &slog.HandlerOptions{
		Level: globalLogLevel,
	}

	switch level {
	case "debug":
		globalLogLevel.Set(slog.LevelDebug)
	case "info":
		globalLogLevel.Set(slog.LevelInfo)
	case "warn":
		globalLogLevel.Set(slog.LevelWarn)
	case "error":
		globalLogLevel.Set(slog.LevelError)
	default:
		globalLogLevel.Set(slog.LevelInfo)
	}

	// In MCP mode, stdout is reserved for JSON-RPC messages, so logs must go to stderr
	var w io.Writer = os.Stdout
	if mcpMode {
		w = os.Stderr
	}

	if logfile != "" {
		f, err := os.OpenFile(logfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open logfile: %v\n", err)
		} else {
			w = io.MultiWriter(w, f)
		}
	}

	handler := slog.NewTextHandler(w, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

var Version = "dev"

func main() {
	// Parse CLI flags
	configFileFlag := flag.String("config", "", "Path to config.ini file")
	statusFileFlag := flag.String("status", "", "Override path to Status.json")
	dbPathFlag := flag.String("db", "", "Path to SQLite database file (default: ed_assist.db next to binary)")
	logLevelFlag := flag.String("loglevel", "", "Set log level (debug, info, warn, error)")
	logFileFlag := flag.String("logfile", "", "Log to specified file")
	pollIntervalFlag := flag.Duration("interval", 0, "Polling interval for poll mode (default: 250ms)")
	modeFlag := flag.String("mode", "", "Update detection mode: watch (event-driven, default) or poll (ticker)")
	retriesFlag := flag.Int("retries", -1, "Maximum quick retries on read failure (default: 3)")
	retryDelayFlag := flag.Duration("retry-delay", 0, "Delay time between quick retries (default: 25ms)")
	trackFlag := flag.Bool("track", false, "Enable SQLite tracking of visited and targeted systems (default: false)")
	flag.BoolVar(trackFlag, "tracking", false, "Enable SQLite tracking of visited and targeted systems (alias)")
	maxVisitedFlag := flag.Int("max-visited", 0, "Maximum number of visited star systems to retain in SQLite (default: 100)")
	maxTargetedFlag := flag.Int("max-targeted", 0, "Maximum number of targeted systems to retain in SQLite (default: 100)")
	cacheHoursFlag := flag.Int("cache-hours", 0, "System info and external API cache TTL in hours (default: 8)")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("ed-assist %s\n", Version)
		return
	}

	// Load configuration from config.ini (or auto-determine defaults)
	cfg, err := config.Load(*configFileFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply CLI overrides if provided
	trackFlagPassed := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "track" || f.Name == "tracking" {
			trackFlagPassed = true
		}
	})
	if trackFlagPassed {
		cfg.EnableTracking = *trackFlag
	}
	if *statusFileFlag != "" {
		cfg.StatusFilePath = *statusFileFlag
		cfg.ConfigSource = fmt.Sprintf("flag (%s)", *statusFileFlag)
	}
	if *dbPathFlag != "" {
		cfg.DBPath = *dbPathFlag
		if !trackFlagPassed {
			cfg.EnableTracking = true
		}
	}
	if *modeFlag != "" {
		cfg.Mode = *modeFlag
	}
	if *retriesFlag >= 0 {
		cfg.MaxRetries = *retriesFlag
	}
	if *retryDelayFlag > 0 {
		cfg.RetryDelay = *retryDelayFlag
	}
	if *logLevelFlag != "" {
		cfg.LogLevel = *logLevelFlag
	}
	if *logFileFlag != "" {
		cfg.LogFile = *logFileFlag
	}
	if *pollIntervalFlag > 0 {
		cfg.PollInterval = *pollIntervalFlag
	}
	if *cacheHoursFlag > 0 {
		cfg.SystemCacheHours = *cacheHoursFlag
		cfg.SystemCacheTTL = time.Duration(*cacheHoursFlag) * time.Hour
	}
	if *maxVisitedFlag > 0 {
		cfg.MaxVisitedSystems = *maxVisitedFlag
	}
	if *maxTargetedFlag > 0 {
		cfg.MaxTargetedSystems = *maxTargetedFlag
	}

	setupLogging(cfg.LogLevel, cfg.LogFile, cfg.EnableMCP && cfg.MCPTransport == "stdio")

	slog.Info("starting ed-assist",
		"mcp_enabled", cfg.EnableMCP,
		"mcp_transport", cfg.MCPTransport,
		"mcp_addr", cfg.MCPAddr,
		"tracking_enabled", cfg.EnableTracking,
		"max_visited", cfg.MaxVisitedSystems,
		"max_targeted", cfg.MaxTargetedSystems,
		"game_control", cfg.GameControl,
		"key_hold_ms", cfg.KeyHoldMs,
		"mode", cfg.Mode,
		"poll_interval", cfg.PollInterval,
		"max_retries", cfg.MaxRetries,
		"retry_delay", cfg.RetryDelay,
	)
	slog.Info("expected status file path", "path", cfg.StatusFilePath, "source", cfg.ConfigSource)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup graceful shutdown signal listener
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		slog.Info("shutdown signal received", "signal", sig.String())
		cancel()
	}()

	var sqliteStore *store.Store
	var sysTracker *tracker.Tracker

	if cfg.EnableTracking {
		// Initialize local SQLite database next to binary
		var err error
		sqliteStore, err = store.New(cfg.DBPath,
			store.WithMaxVisited(cfg.MaxVisitedSystems),
			store.WithMaxTargeted(cfg.MaxTargetedSystems),
		)
		if err != nil {
			slog.Error("failed to open sqlite database", "path", cfg.DBPath, "error", err)
			os.Exit(1)
		}
		defer sqliteStore.Close()

		// Initialize journal & status tracker for visited and targeted systems
		sysTracker = tracker.New(sqliteStore, cfg.StatusFilePath)
		go sysTracker.Start(ctx)
		slog.Info("system tracking enabled", "database", cfg.DBPath)
	} else {
		slog.Info("system tracking disabled (use -track to enable)")
	}

	// Initialize game control if enabled in config.ini
	var gameController *input.Controller
	if cfg.GameControl {
		gameController = input.NewController(cfg.BindingsPath, cfg.KeyHoldMs, nil)
		if err := gameController.Load(); err != nil {
			slog.Warn("game control enabled but failed loading binds", "error", err)
		} else {
			slog.Info("game control active", "actions", len(gameController.ListActions()), "key_hold_ms", cfg.KeyHoldMs)
		}
	} else {
		slog.Info("game control disabled (enable with game_control = true in config.ini)")
	}

	// Initialize status reader (event-driven watch mode or periodic polling)
	readerMode := reader.ModeWatch
	if cfg.Mode == string(reader.ModePoll) {
		readerMode = reader.ModePoll
	}

	statusReader := reader.New(
		cfg.StatusFilePath,
		reader.WithMode(readerMode),
		reader.WithPollInterval(cfg.PollInterval),
		reader.WithRetries(cfg.MaxRetries, cfg.RetryDelay),
	)

	go statusReader.Start(ctx)

	// If MCP mode is enabled, run the MCP server
	if cfg.EnableMCP {
		mcpSrv := mcpserver.New(statusReader, sqliteStore, gameController, mcpserver.WithCacheTTL(cfg.SystemCacheTTL))

		if cfg.MCPTransport == "http" {
			// In HTTP mode, run MCP server in background and fall through to terminal event display
			go func() {
				if err := mcpSrv.ServeHTTP(ctx, cfg.MCPAddr); err != nil && ctx.Err() == nil {
					slog.Error("MCP HTTP server stopped", "error", err)
					cancel()
				}
			}()
			slog.Info("MCP server ready over HTTP/SSE",
				"sse_endpoint", fmt.Sprintf("http://%s/sse", cfg.MCPAddr),
				"tools", 18,
				"resources", 5,
			)
		} else {
			// In stdio mode, stdio is reserved for JSON-RPC communication
			slog.Info("MCP server ready on stdio", "tools", 18, "resources", 5)

			// Forward status updates to tracker in background if tracking enabled
			if sysTracker != nil {
				go func() {
					for st := range statusReader.StatusChan() {
						sysTracker.ProcessStatus(st)
					}
				}()
			}

			errChan := make(chan error, 1)
			go func() {
				errChan <- mcpSrv.ServeStdio(ctx, os.Stdin, os.Stdout)
			}()

			select {
			case <-ctx.Done():
				slog.Info("MCP server shutting down")
			case err := <-errChan:
				if err != nil && err != io.EOF && ctx.Err() == nil {
					slog.Error("MCP server exited", "error", err)
				}
				cancel()
			}

			statusReader.Stop()
			statusReader.Wait()
			slog.Info("ed-assist stopped cleanly")
			return
		}
	}

	// Normal terminal display mode: display parsed status updates and track destinations
	for {
		select {
		case <-ctx.Done():
			statusReader.Stop()
			statusReader.Wait()
			slog.Info("ed-assist stopped cleanly")
			return

		case st, ok := <-statusReader.StatusChan():
			if !ok {
				return
			}
			if sysTracker != nil {
				sysTracker.ProcessStatus(st)
			}
			displayStatus(st)
		}
	}
}

func displayStatus(st *parser.Status) {
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println(st.Summary())
	fmt.Println("--------------------------------------------------------------------------------")
}
