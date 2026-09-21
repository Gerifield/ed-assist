package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"ed-assist/internal/gemini"
)

//go:embed static/*
var staticFS embed.FS

// Server hosts the web client UI and JSON API endpoints.
type Server struct {
	addr           string
	gemini         *gemini.Client
	bridge         *MCPBridge
	modelName      string
	gateThreshold  int
	silenceMs      int
	echoProtection bool
	historyMu      sync.Mutex
	history        []gemini.ChatMessage
	httpServer     *http.Server
}

// ServerOption configures optional web server settings.
type ServerOption func(*Server)

// WithEchoProtection configures whether VOX listening is paused while COVAS speaks aloud.
func WithEchoProtection(enabled bool) ServerOption {
	return func(s *Server) {
		s.echoProtection = enabled
	}
}

// NewServer creates a new web assistant server.
func NewServer(addr string, geminiClient *gemini.Client, bridge *MCPBridge, modelName string, gateThreshold, silenceMs int, opts ...ServerOption) *Server {
	if addr == "" {
		addr = "127.0.0.1:3000"
	}
	if modelName == "" {
		modelName = "gemini-flash-lite-latest"
	}
	if gateThreshold <= 0 {
		gateThreshold = 40
	}
	if silenceMs <= 0 {
		silenceMs = 2000
	}
	srv := &Server{
		addr:           addr,
		gemini:         geminiClient,
		bridge:         bridge,
		modelName:      modelName,
		gateThreshold:  gateThreshold,
		silenceMs:      silenceMs,
		echoProtection: true, // Default to true
	}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// Start launches the HTTP server for the web interface and API.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Sub-tree filesystem for embedded static assets
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed resolving embedded static assets: %w", err)
	}

	// Static file handler (serves index.html at /)
	fileServer := http.FileServer(http.FS(subFS))
	mux.Handle("/", fileServer)

	// API route: status and MCP tool discovery info
	mux.HandleFunc("/api/info", s.handleInfo)

	// API route: chat turn processing
	mux.HandleFunc("/api/chat", s.handleChat)

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	errChan := make(chan error, 1)
	go func() {
		slog.Info("starting COVAS web client", "addr", fmt.Sprintf("http://%s", s.addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

// ChatRequest represents an incoming user turn.
type ChatRequest struct {
	Prompt    string `json:"prompt"`
	AudioB64  string `json:"audio_b64,omitempty"`
	AudioMime string `json:"audio_mime,omitempty"`
}

// ChatResponse represents the assistant output.
type ChatResponse struct {
	Reply string `json:"reply,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	toolCount := 0
	mode := "none"
	if s.bridge != nil {
		mode = s.bridge.Mode()
		tools, err := s.bridge.ListTools(r.Context())
		if err == nil {
			toolCount = len(tools)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":                 s.modelName,
		"mcp_mode":              mode,
		"tool_count":            toolCount,
		"status":                "online",
		"voice_gate_threshold":  s.gateThreshold,
		"voice_silence_ms":      s.silenceMs,
		"voice_echo_protection": s.echoProtection,
	})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ChatResponse{Error: "Invalid JSON request: " + err.Error()})
		return
	}

	hasAudio := req.AudioB64 != ""
	if req.Prompt != "" {
		slog.Info("received user command", "prompt", req.Prompt, "has_audio", hasAudio)
	} else if hasAudio {
		slog.Info("received user command", "type", "voice_audio", "mime", req.AudioMime)
	} else {
		slog.Info("received user command", "type", "empty")
	}

	s.historyMu.Lock()
	historyCopy := make([]gemini.ChatMessage, len(s.history))
	copy(historyCopy, s.history)
	s.historyMu.Unlock()

	var audioBytes []byte
	if req.AudioB64 != "" {
		audioBytes = []byte(req.AudioB64)
	}

	reply, err := s.gemini.ExecuteTurn(r.Context(), historyCopy, req.Prompt, audioBytes, req.AudioMime)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		slog.Error("failed executing user command", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ChatResponse{Error: err.Error()})
		return
	}

	slog.Info("command completed successfully", "reply_len", len(reply))

	// Update conversation history
	s.historyMu.Lock()
	if req.Prompt != "" {
		s.history = append(s.history, gemini.ChatMessage{Role: "user", Text: req.Prompt})
	} else if req.AudioB64 != "" {
		s.history = append(s.history, gemini.ChatMessage{Role: "user", Text: "[Voice Message]"})
	}
	s.history = append(s.history, gemini.ChatMessage{Role: "model", Text: reply})

	// Retain only latest 20 turns
	if len(s.history) > 20 {
		s.history = s.history[len(s.history)-20:]
	}
	s.historyMu.Unlock()

	_ = json.NewEncoder(w).Encode(ChatResponse{Reply: reply})
}
