package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiWrapper(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Commander, all systems nominal."},
						},
					},
					"finishReason": "STOP",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient("test-key", "gemini-3.8-flash-lite",
		WithBaseURL(ts.URL),
		WithHTTPClient(ts.Client()),
		WithSystemPrompt("Custom prompt"),
	)

	if client.SystemPrompt() != "Custom prompt" {
		t.Errorf("expected 'Custom prompt', got '%s'", client.SystemPrompt())
	}
	if client.Provider() != "gemini" {
		t.Errorf("expected provider 'gemini', got '%s'", client.Provider())
	}

	reply, err := client.ExecuteTurn(context.Background(), nil, "Status report", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "Commander, all systems nominal." {
		t.Errorf("expected reply 'Commander, all systems nominal.', got '%s'", reply)
	}
}

func TestGeminiDefaultPrompt(t *testing.T) {
	if DefaultSystemPrompt == "" {
		t.Errorf("expected non-empty DefaultSystemPrompt")
	}
}
