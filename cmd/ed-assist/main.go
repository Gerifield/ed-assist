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

	"ed-assist/internal/config"
	"ed-assist/internal/parser"
	"ed-assist/internal/reader"
)

var globalLogLevel *slog.LevelVar

func setupLogging(level, logfile string) {
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

	var w io.Writer = os.Stdout
	if logfile != "" {
		f, err := os.OpenFile(logfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open logfile: %v\n", err)
		} else {
			w = io.MultiWriter(os.Stdout, f)
		}
	}

	handler := slog.NewTextHandler(w, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

func main() {
	// Parse CLI flags
	configFileFlag := flag.String("config", "", "Path to config.ini file")
	statusFileFlag := flag.String("status", "", "Override path to Status.json")
	logLevelFlag := flag.String("loglevel", "", "Set log level (debug, info, warn, error)")
	logFileFlag := flag.String("logfile", "", "Log to specified file")
	pollIntervalFlag := flag.Duration("interval", 0, "Polling interval for poll mode (default: 250ms)")
	modeFlag := flag.String("mode", "", "Update detection mode: watch (event-driven, default) or poll (ticker)")
	retriesFlag := flag.Int("retries", -1, "Maximum quick retries on read failure (default: 3)")
	retryDelayFlag := flag.Duration("retry-delay", 0, "Delay time between quick retries (default: 25ms)")
	flag.Parse()

	// Load configuration from config.ini (or auto-determine defaults)
	cfg, err := config.Load(*configFileFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply CLI overrides if provided
	if *statusFileFlag != "" {
		cfg.StatusFilePath = *statusFileFlag
		cfg.ConfigSource = fmt.Sprintf("flag (%s)", *statusFileFlag)
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

	setupLogging(cfg.LogLevel, cfg.LogFile)

	slog.Info("starting ed-assist",
		"mode", cfg.Mode,
		"poll_interval", cfg.PollInterval,
		"max_retries", cfg.MaxRetries,
		"retry_delay", cfg.RetryDelay,
	)
	// Log expected path as required
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

	// Display parsed status updates as they occur
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
			displayStatus(st)
		}
	}
}

func displayStatus(st *parser.Status) {
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println(st.Summary())
	fmt.Println("--------------------------------------------------------------------------------")
}
