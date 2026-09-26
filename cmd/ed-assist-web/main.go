package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ed-assist/internal/config"
	"ed-assist/internal/input"
	"ed-assist/internal/llm"
	"ed-assist/internal/mcpserver"
	"ed-assist/internal/reader"
	"ed-assist/internal/store"
	"ed-assist/internal/stt"
	"ed-assist/internal/tracker"
	"ed-assist/internal/web"
)

var Version = "dev"

func main() {
	// Parse CLI flags
	configFileFlag := flag.String("config", "", "Path to config.ini file")
	webAddrFlag := flag.String("addr", "", "Listen address for web UI (default: 127.0.0.1:3000)")
	providerFlag := flag.String("provider", "", "AI provider: gemini or openai (default: gemini)")
	mcpModeFlag := flag.String("mcp-mode", "", "MCP transport mode: http, stdio, or inprocess (default: inprocess/http)")
	mcpEndpointFlag := flag.String("mcp-endpoint", "", "MCP HTTP endpoint (e.g. http://127.0.0.1:8080/sse) or path to binary for stdio")
	apiKeyFlag := flag.String("api-key", "", "AI API key (Gemini or OpenAI depending on provider)")
	modelFlag := flag.String("model", "", "Model name (Gemini or OpenAI)")
	openaiKeyFlag := flag.String("openai-key", "", "OpenAI / OpenAI-compatible API key")
	openaiModelFlag := flag.String("openai-model", "", "OpenAI / OpenAI-compatible model (e.g. gpt-4o-mini, deepseek-chat)")
	openaiEndpointFlag := flag.String("openai-endpoint", "", "OpenAI-compatible base URL (e.g. https://api.openai.com/v1, https://api.deepseek.com/v1)")
	statusFileFlag := flag.String("status", "", "Override path to Status.json")
	dbPathFlag := flag.String("db", "", "Path to SQLite database file")
	maxVisitedFlag := flag.Int("max-visited", 0, "Maximum number of visited star systems to retain in SQLite (default: 100)")
	maxTargetedFlag := flag.Int("max-targeted", 0, "Maximum number of targeted systems to retain in SQLite (default: 100)")
	cacheHoursFlag := flag.Int("cache-hours", 0, "System info and external API cache TTL in hours (default: 8)")
	maxToolRoundsFlag := flag.Int("max-tool-rounds", 0, "Maximum rounds for AI tool calling loop (default: 10)")
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
	if *providerFlag != "" {
		cfg.AIProvider = *providerFlag
	}
	if *mcpModeFlag != "" {
		cfg.GeminiMCPMode = *mcpModeFlag
	}
	if *mcpEndpointFlag != "" {
		cfg.GeminiMCPEndpoint = *mcpEndpointFlag
	}
	if *openaiKeyFlag != "" {
		cfg.OpenAIAPIKey = *openaiKeyFlag
	}
	if *openaiModelFlag != "" {
		cfg.OpenAIModel = *openaiModelFlag
	}
	if *openaiEndpointFlag != "" {
		cfg.OpenAIBaseURL = *openaiEndpointFlag
	}
	if *apiKeyFlag != "" {
		if strings.ToLower(cfg.AIProvider) == "openai" {
			cfg.OpenAIAPIKey = *apiKeyFlag
		} else {
			cfg.GeminiAPIKey = *apiKeyFlag
		}
	}
	if *modelFlag != "" {
		if strings.ToLower(cfg.AIProvider) == "openai" {
			cfg.OpenAIModel = *modelFlag
		} else {
			cfg.GeminiModel = *modelFlag
		}
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
	if *maxToolRoundsFlag > 0 {
		cfg.MaxToolRounds = *maxToolRoundsFlag
	}
	if *maxVisitedFlag > 0 {
		cfg.MaxVisitedSystems = *maxVisitedFlag
	}
	if *maxTargetedFlag > 0 {
		cfg.MaxTargetedSystems = *maxTargetedFlag
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
		"max_visited", cfg.MaxVisitedSystems,
		"max_targeted", cfg.MaxTargetedSystems,
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
		st, err := store.New(cfg.DBPath,
			store.WithMaxVisited(cfg.MaxVisitedSystems),
			store.WithMaxTargeted(cfg.MaxTargetedSystems),
		)
		if err != nil {
			slog.Warn("could not initialize SQLite store, continuing in-memory", "error", err)
		} else {
			sqliteStore = st
			defer sqliteStore.Close()
		}

		// Vehicle state & journal tracker: ALWAYS initialize so COVAS knows whether player is in Ship, Nomad, SRV, or On Foot
		var activeStore *store.Store
		if cfg.EnableTracking {
			activeStore = sqliteStore
		}
		sysTracker = tracker.New(activeStore, cfg.StatusFilePath)
		go sysTracker.Start(ctx)

		var gameController *input.Controller
		if cfg.GameControl {
			gameController = input.NewController(cfg.BindingsPath, cfg.KeyHoldMs, input.NewKeySender())
			if err := gameController.Load(); err != nil {
				slog.Warn("game control enabled but failed loading binds", "error", err)
			} else {
				slog.Info("game control active in web assistant",
					"preset", gameController.PresetName(),
					"actions", len(gameController.ListActions()),
					"key_hold_ms", cfg.KeyHoldMs,
				)
			}
		} else {
			slog.Info("game control disabled (enable with game_control = true in config.ini)")
		}

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

		mcpSrv := mcpserver.New(statusReader, sqliteStore, gameController,
			mcpserver.WithCacheTTL(cfg.SystemCacheTTL),
			mcpserver.WithVehicleProvider(sysTracker),
		)
		b, err := web.NewInProcessBridge(mcpSrv.Server())
		if err != nil {
			slog.Error("failed creating in-process MCP bridge", "error", err)
		} else {
			bridge = b
			defer bridge.Close()
		}
	}

	// Create AI client (Gemini or OpenAI-compatible)
	aiClient, err := llm.NewClientFromConfig(llm.ProviderConfig{
		Provider:      cfg.AIProvider,
		GeminiKey:     cfg.GeminiAPIKey,
		GeminiModel:   cfg.GeminiModel,
		GeminiBaseURL: cfg.GeminiBaseURL,
		OpenAIKey:     cfg.OpenAIAPIKey,
		OpenAIModel:   cfg.OpenAIModel,
		OpenAIBaseURL: cfg.OpenAIBaseURL,
		SystemPrompt:    cfg.SystemPrompt,
		MaxToolRounds:   cfg.MaxToolRounds,
		AutoTimeContext: cfg.AutoTimeContext,
		MCPCaller:       bridge,
	})
	if err != nil {
		slog.Error("failed initializing AI client", "error", err)
		os.Exit(1)
	}

	if cfg.SystemPrompt != "" && cfg.SystemPrompt != llm.DefaultSystemPrompt {
		slog.Info("using custom system prompt for COVAS", "chars", len(cfg.SystemPrompt))
	}

	modelDisplayName := aiClient.ModelName()
	slog.Info("AI copilot initialized", "provider", aiClient.Provider(), "model", modelDisplayName)

	// Initialize STT transcriber if enabled
	var transcriber stt.Transcriber
	if cfg.STTEnabled {
		sttKey := cfg.STTGroqAPIKey
		if sttKey == "" {
			sttKey = cfg.OpenAIAPIKey
		}
		if sttKey == "" && (cfg.STTBackend == "groq" || cfg.STTBackend == "openai") {
			slog.Warn("STT enabled but neither groq_api_key nor openai_api_key configured; transcription may fail unless using a local backend", "backend", cfg.STTBackend)
		}
		t, err := stt.NewTranscriber(stt.Config{
			Enabled:          cfg.STTEnabled,
			Backend:          cfg.STTBackend,
			APIKey:           sttKey,
			Model:            cfg.STTGroqModel,
			BaseURL:          cfg.STTOpenAIBaseURL,
			PromptVocabulary: cfg.STTPromptVocabulary,
			Language:         cfg.STTLanguage,
		})
		if err != nil {
			slog.Warn("failed initializing STT transcriber", "error", err)
		} else {
			transcriber = t
			slog.Info("STT transcriber initialized", "backend", cfg.STTBackend, "audio_input_mode", cfg.AudioInputMode, "language", cfg.STTLanguage)
		}
	}

	// Create and start web server
	webServer := web.NewServer(
		cfg.WebAddr,
		aiClient,
		bridge,
		modelDisplayName,
		cfg.VoiceGateThreshold,
		cfg.VoiceSilenceMs,
		web.WithEchoProtection(cfg.VoiceEchoProtection),
		web.WithTranscriber(transcriber),
		web.WithAudioInputMode(cfg.AudioInputMode),
	)
	if err := webServer.Start(ctx); err != nil && ctx.Err() == nil {
		slog.Error("web server error", "error", err)
		os.Exit(1)
	}

	slog.Info("ed-assist-web stopped cleanly")
}
