package stt

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewTranscriberDisabled(t *testing.T) {
	_, err := NewTranscriber(Config{Enabled: false})
	if err == nil {
		t.Fatal("expected error when STT is disabled, got nil")
	}
}

func TestNewTranscriberUnsupportedBackend(t *testing.T) {
	_, err := NewTranscriber(Config{
		Enabled: true,
		Backend: "unsupported_backend",
	})
	if err == nil {
		t.Fatal("expected error for unsupported backend, got nil")
	}
}

func TestTranscribeSuccess(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
			t.Errorf("expected path ending in /audio/transcriptions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("failed parsing multipart form: %v", err)
		}

		if r.FormValue("model") != "whisper-large-v3-turbo" {
			t.Errorf("expected model whisper-large-v3-turbo, got %s", r.FormValue("model"))
		}
		if r.FormValue("prompt") != "FSD, SRV, COVAS" {
			t.Errorf("expected prompt FSD, SRV, COVAS, got %s", r.FormValue("prompt"))
		}
		if r.FormValue("language") != "en" {
			t.Errorf("expected language en, got %s", r.FormValue("language"))
		}
		if r.FormValue("temperature") != "0.0" {
			t.Errorf("expected temperature 0.0, got %s", r.FormValue("temperature"))
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file in multipart form: %v", err)
		}
		defer file.Close()

		if header.Filename != "test.webm" {
			t.Errorf("expected filename test.webm, got %s", header.Filename)
		}

		audioContent, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("failed reading file from form: %v", err)
		}

		if string(audioContent) != "dummy audio content" {
			t.Errorf("expected 'dummy audio content', got %s", string(audioContent))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"text":"Frame Shift Drive engaged"}`))
	}))
	defer mockServer.Close()

	transcriber, err := NewTranscriber(Config{
		Enabled:          true,
		Backend:          "groq",
		APIKey:           "test-key",
		Model:            "whisper-large-v3-turbo",
		BaseURL:          mockServer.URL,
		PromptVocabulary: "FSD, SRV, COVAS",
		Language:         "en",
		HTTPClient:       mockServer.Client(),
	})
	if err != nil {
		t.Fatalf("failed creating transcriber: %v", err)
	}

	audioReader := bytes.NewReader([]byte("dummy audio content"))
	text, err := transcriber.Transcribe(context.Background(), audioReader, "test.webm")
	if err != nil {
		t.Fatalf("unexpected transcription error: %v", err)
	}

	if text != "Frame Shift Drive engaged" {
		t.Errorf("expected transcribed text 'Frame Shift Drive engaged', got '%s'", text)
	}
}

func TestTranscribeAPIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`))
	}))
	defer mockServer.Close()

	transcriber, err := NewTranscriber(Config{
		Enabled:    true,
		Backend:    "openai",
		APIKey:     "bad-key",
		BaseURL:    mockServer.URL,
		HTTPClient: mockServer.Client(),
	})
	if err != nil {
		t.Fatalf("failed creating transcriber: %v", err)
	}

	audioReader := bytes.NewReader([]byte("dummy audio"))
	_, err = transcriber.Transcribe(context.Background(), audioReader, "test.mp3")
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("expected error to contain 'Invalid API key', got '%v'", err)
	}
}
