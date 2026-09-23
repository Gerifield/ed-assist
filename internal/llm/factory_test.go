package llm

import (
	"testing"
)

func TestNewClientFromConfig(t *testing.T) {
	// Gemini client creation
	client, err := NewClientFromConfig(ProviderConfig{
		Provider:      "gemini",
		GeminiKey:     "test-gemini-key",
		GeminiModel:   "gemini-test",
		GeminiBaseURL: "http://localhost:8080",
		SystemPrompt:  "custom system prompt",
		MaxToolRounds: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error creating gemini client: %v", err)
	}
	if client.Provider() != "gemini" {
		t.Errorf("expected provider gemini, got %s", client.Provider())
	}
	if client.ModelName() != "gemini-test" {
		t.Errorf("expected model gemini-test, got %s", client.ModelName())
	}
	if client.SystemPrompt() != "custom system prompt" {
		t.Errorf("expected custom system prompt, got %s", client.SystemPrompt())
	}

	// OpenAI client creation
	client, err = NewClientFromConfig(ProviderConfig{
		Provider:      "openai",
		OpenAIKey:     "test-openai-key",
		OpenAIModel:   "gpt-4o",
		OpenAIBaseURL: "https://api.openai.com/v1",
		SystemPrompt:  "openai prompt",
		MaxToolRounds: 7,
	})
	if err != nil {
		t.Fatalf("unexpected error creating openai client: %v", err)
	}
	if client.Provider() != "openai" {
		t.Errorf("expected provider openai, got %s", client.Provider())
	}
	if client.ModelName() != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", client.ModelName())
	}
	if client.SystemPrompt() != "openai prompt" {
		t.Errorf("expected openai prompt, got %s", client.SystemPrompt())
	}

	// Unsupported provider error
	_, err = NewClientFromConfig(ProviderConfig{
		Provider: "anthropic",
	})
	if err == nil {
		t.Errorf("expected error for unsupported provider, got nil")
	}
}
