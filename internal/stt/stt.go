package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

// Transcriber is the interface for Speech-to-Text audio transcription.
type Transcriber interface {
	Transcribe(ctx context.Context, audio io.Reader, filename string) (string, error)
}

// Config contains parameters for constructing a Transcriber.
type Config struct {
	Enabled          bool   `json:"enabled"`
	Backend          string `json:"backend"`             // "groq", "openai", "local_whisper"
	APIKey           string `json:"api_key"`             // Groq API key or OpenAI API key
	Model            string `json:"model"`               // e.g. "whisper-large-v3-turbo" or "whisper-1"
	BaseURL          string `json:"base_url"`            // e.g. "https://api.groq.com/openai/v1" or "http://localhost:8080/v1"
	PromptVocabulary string `json:"prompt_vocabulary"`  // Elite Dangerous vocabulary prompt to guide Whisper
	Language         string `json:"language"`           // Optional ISO language code (e.g. "en"), empty for auto-detect
	HTTPClient       *http.Client `json:"-"`
}

// OpenAITranscriber implements Transcriber for OpenAI-compatible transcription APIs (Groq, OpenAI, local Whisper).
type OpenAITranscriber struct {
	cfg        Config
	httpClient *http.Client
}

// NewTranscriber creates a Transcriber instance based on the provided configuration.
func NewTranscriber(cfg Config) (Transcriber, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("stt is disabled in configuration")
	}

	backend := strings.ToLower(strings.TrimSpace(cfg.Backend))
	if backend == "" {
		backend = "groq"
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	switch backend {
	case "groq":
		if cfg.Model == "" {
			cfg.Model = "whisper-large-v3-turbo"
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.groq.com/openai/v1"
		}
	case "openai", "local_whisper", "whisper":
		if cfg.Model == "" {
			cfg.Model = "whisper-1"
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
	default:
		return nil, fmt.Errorf("unsupported stt backend: %s", cfg.Backend)
	}

	return &OpenAITranscriber{
		cfg:        cfg,
		httpClient: client,
	}, nil
}

type transcribeResponse struct {
	Text  string `json:"text"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Transcribe converts audio content into text using OpenAI-compatible multipart HTTP endpoint.
func (t *OpenAITranscriber) Transcribe(ctx context.Context, audio io.Reader, filename string) (string, error) {
	if audio == nil {
		return "", fmt.Errorf("audio reader is nil")
	}

	if filename == "" {
		filename = "audio.webm"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file field with proper content-type header
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filepath.Base(filename)))

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp3":
		h.Set("Content-Type", "audio/mpeg")
	case ".wav":
		h.Set("Content-Type", "audio/wav")
	case ".m4a":
		h.Set("Content-Type", "audio/m4a")
	case ".ogg":
		h.Set("Content-Type", "audio/ogg")
	default:
		h.Set("Content-Type", "audio/webm")
	}

	part, err := writer.CreatePart(h)
	if err != nil {
		return "", fmt.Errorf("failed creating multipart file header: %w", err)
	}

	if _, err := io.Copy(part, audio); err != nil {
		return "", fmt.Errorf("failed copying audio data: %w", err)
	}

	// Add model field
	if err := writer.WriteField("model", t.cfg.Model); err != nil {
		return "", fmt.Errorf("failed writing model form field: %w", err)
	}

	// Add prompt vocabulary field if configured
	if strings.TrimSpace(t.cfg.PromptVocabulary) != "" {
		if err := writer.WriteField("prompt", t.cfg.PromptVocabulary); err != nil {
			return "", fmt.Errorf("failed writing prompt form field: %w", err)
		}
	}

	// Add language field if configured, otherwise omitted for Whisper auto-detection
	if strings.TrimSpace(t.cfg.Language) != "" {
		if err := writer.WriteField("language", strings.TrimSpace(t.cfg.Language)); err != nil {
			return "", fmt.Errorf("failed writing language form field: %w", err)
		}
	}

	// Temperature=0.0 for deterministic output
	if err := writer.WriteField("temperature", "0.0"); err != nil {
		return "", fmt.Errorf("failed writing temperature form field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed closing multipart writer: %w", err)
	}

	endpoint := strings.TrimRight(t.cfg.BaseURL, "/") + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("failed creating transcription http request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if t.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.cfg.APIKey)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("stt http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed reading stt response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp transcribeResponse
		if err := json.Unmarshal(respBytes, &errResp); err == nil && errResp.Error != nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("stt transcription api error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("stt transcription api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var tr transcribeResponse
	if err := json.Unmarshal(respBytes, &tr); err != nil {
		return "", fmt.Errorf("failed unmarshaling stt response JSON: %w", err)
	}

	return tr.Text, nil
}
