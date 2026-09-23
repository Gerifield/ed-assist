package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"ed-assist/internal/llm"
	"ed-assist/internal/stt"
)

//go:embed static/*
var staticFS embed.FS

// Server hosts the web client UI and JSON API endpoints.
type Server struct {
	addr           string
	ai             llm.Client
	bridge         *MCPBridge
	transcriber    stt.Transcriber
	audioInputMode string
	modelName      string
	gateThreshold  int
	silenceMs      int
	echoProtection bool
	historyMu      sync.Mutex
	history        []llm.ChatMessage
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

// WithTranscriber configures the STT transcriber for the web server.
func WithTranscriber(t stt.Transcriber) ServerOption {
	return func(s *Server) {
		s.transcriber = t
	}
}

// WithAudioInputMode configures the audio processing strategy ("native" or "transcribe").
func WithAudioInputMode(mode string) ServerOption {
	return func(s *Server) {
		s.audioInputMode = mode
	}
}

// NewServer creates a new web assistant server.
func NewServer(addr string, aiClient llm.Client, bridge *MCPBridge, modelName string, gateThreshold, silenceMs int, opts ...ServerOption) *Server {
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
		ai:             aiClient,
		bridge:         bridge,
		modelName:      modelName,
		gateThreshold:  gateThreshold,
		silenceMs:      silenceMs,
		echoProtection: true, // Default to true
		audioInputMode: "transcribe",
	}
	for _, opt := range opts {
		opt(srv)
	}
	if srv.audioInputMode == "" {
		srv.audioInputMode = "transcribe"
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

	// API route: audio transcription endpoint
	mux.HandleFunc("/api/transcribe", s.handleTranscribe)

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
	Reply         string `json:"reply,omitempty"`
	Transcription string `json:"transcription,omitempty"`
	Error         string `json:"error,omitempty"`
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

	var systemPrompt string
	provider := "unknown"
	if s.ai != nil {
		systemPrompt = s.ai.SystemPrompt()
		provider = s.ai.Provider()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider":              provider,
		"model":                 s.modelName,
		"mcp_mode":              mode,
		"tool_count":            toolCount,
		"status":                "online",
		"voice_gate_threshold":  s.gateThreshold,
		"voice_silence_ms":      s.silenceMs,
		"voice_echo_protection": s.echoProtection,
		"system_prompt":         systemPrompt,
		"audio_input_mode":      s.audioInputMode,
		"stt_enabled":           s.transcriber != nil,
	})
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.transcriber == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "STT transcriber is not configured or disabled"})
		return
	}

	var audioReader io.Reader
	filename := "audio.webm"

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed parsing multipart form: " + err.Error()})
			return
		}

		file, header, err := r.FormFile("audio")
		if err != nil {
			file, header, err = r.FormFile("file")
		}
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Missing audio file in form data ('audio' or 'file' field required)"})
			return
		}
		defer file.Close()
		audioReader = file
		if header != nil && header.Filename != "" {
			filename = header.Filename
		}
	} else {
		// Handle JSON body with base64 encoded audio
		var body struct {
			AudioB64  string `json:"audio_b64"`
			AudioMime string `json:"audio_mime"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body: " + err.Error()})
			return
		}

		if body.AudioB64 == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Missing audio_b64 in request body"})
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(body.AudioB64)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid base64 audio data: " + err.Error()})
			return
		}
		audioReader = bytes.NewReader(decoded)
		if strings.Contains(body.AudioMime, "mp3") {
			filename = "audio.mp3"
		} else if strings.Contains(body.AudioMime, "wav") {
			filename = "audio.wav"
		}
	}

	transcribedText, err := s.transcriber.Transcribe(r.Context(), audioReader, filename)
	if err != nil {
		slog.Error("STT transcription failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "STT transcription failed: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"text": transcribedText})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("panic in chat handler recovered", "panic", rec)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(ChatResponse{Error: fmt.Sprintf("Assistant internal error: %v", rec)})
		}
	}()

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

	prompt := req.Prompt
	var rawAudioBytes []byte
	audioMime := req.AudioMime
	var transcription string

	if hasAudio {
		decodedAudio, err := base64.StdEncoding.DecodeString(req.AudioB64)
		if err != nil {
			slog.Error("failed decoding base64 audio", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ChatResponse{Error: "Invalid base64 audio data: " + err.Error()})
			return
		}

		if s.audioInputMode == "transcribe" || s.audioInputMode == "" {
			if s.transcriber == nil {
				slog.Error("audio_input_mode is transcribe, but STT transcriber is nil")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(ChatResponse{Error: "STT transcription failed: Transcriber is not configured or disabled"})
				return
			}

			filename := "audio.webm"
			if strings.Contains(audioMime, "mp3") {
				filename = "audio.mp3"
			} else if strings.Contains(audioMime, "wav") {
				filename = "audio.wav"
			}

			var err error
			transcription, err = s.transcriber.Transcribe(r.Context(), bytes.NewReader(decodedAudio), filename)
			if err != nil {
				slog.Error("STT transcription failed in chat endpoint", "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(ChatResponse{Error: "STT transcription failed: " + err.Error()})
				return
			}

			slog.Info("STT transcription completed", "transcription", transcription)

			if prompt != "" {
				prompt = prompt + "\n" + transcription
			} else {
				prompt = transcription
			}

			// Clear audio bytes since audio has been pre-transcribed into pure text
			rawAudioBytes = nil
			audioMime = ""
		} else {
			// "native" mode: pass raw base64 audio bytes directly to multimodal LLM
			rawAudioBytes = []byte(req.AudioB64)
		}
	}

	s.historyMu.Lock()
	historyCopy := make([]llm.ChatMessage, len(s.history))
	copy(historyCopy, s.history)
	s.historyMu.Unlock()

	if s.ai == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ChatResponse{Error: "No AI client configured"})
		return
	}

	reply, err := s.ai.ExecuteTurn(r.Context(), historyCopy, prompt, rawAudioBytes, audioMime)
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
	if prompt != "" {
		s.history = append(s.history, llm.ChatMessage{Role: "user", Text: prompt})
	} else if req.AudioB64 != "" {
		s.history = append(s.history, llm.ChatMessage{Role: "user", Text: "[Voice Message]"})
	}
	s.history = append(s.history, llm.ChatMessage{Role: "assistant", Text: reply})

	// Retain only latest 20 turns
	if len(s.history) > 20 {
		s.history = s.history[len(s.history)-20:]
	}
	s.historyMu.Unlock()

	_ = json.NewEncoder(w).Encode(ChatResponse{
		Reply:         reply,
		Transcription: transcription,
	})
}
