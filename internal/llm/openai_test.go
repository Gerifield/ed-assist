package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func ptr[T any](v T) *T {
	return &v
}

func TestOpenAIDirectTextTurn(t *testing.T) {
	var capturedAuth string
	var capturedReq openAIChatRequest

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&capturedReq)

		resp := openAIChatResponse{
			ID: "chatcmpl-test",
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: openAIMessage{
						Role:    "assistant",
						Content: ptr("All systems nominal, Commander."),
					},
					FinishReason: "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewOpenAIClient("sk-test-12345", "deepseek-chat",
		WithOpenAIBaseURL(ts.URL),
		WithOpenAIHTTPClient(ts.Client()),
	)

	reply, err := client.ExecuteTurn(context.Background(), nil, "Status check", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "All systems nominal, Commander." {
		t.Errorf("expected 'All systems nominal, Commander.', got '%s'", reply)
	}
	if client.Provider() != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", client.Provider())
	}
	if client.ModelName() != "deepseek-chat" {
		t.Errorf("expected model 'deepseek-chat', got '%s'", client.ModelName())
	}
	if capturedAuth != "Bearer sk-test-12345" {
		t.Errorf("expected auth header 'Bearer sk-test-12345', got '%s'", capturedAuth)
	}
	if len(capturedReq.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Errorf("expected first message to be system, got %s", capturedReq.Messages[0].Role)
	}
	if *capturedReq.Messages[1].Content != "Status check" {
		t.Errorf("expected user message content 'Status check', got '%s'", *capturedReq.Messages[1].Content)
	}
}

func TestOpenAIToolCalling(t *testing.T) {
	rounds := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		if rounds == 1 {
			// Round 1: Model requests tool call to get_ship_status
			resp := openAIChatResponse{
				ID: "chatcmpl-tool-1",
				Choices: []struct {
					Index        int           `json:"index"`
					Message      openAIMessage `json:"message"`
					FinishReason string        `json:"finish_reason"`
				}{
					{
						Index: 0,
						Message: openAIMessage{
							Role: "assistant",
							ToolCalls: []openAIToolCall{
								{
									ID:   "call_status_001",
									Type: "function",
									Function: openAIFunctionCallDesc{
										Name:      "get_ship_status",
										Arguments: `{}`,
									},
								},
							},
						},
						FinishReason: "tool_calls",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			// Round 2: Model gives final text reply after inspecting tool response
			var req openAIChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			// Verify that tool response message was sent
			hasToolMsg := false
			for _, m := range req.Messages {
				if m.Role == "tool" && m.ToolCallID == "call_status_001" {
					hasToolMsg = true
					break
				}
			}
			if !hasToolMsg {
				t.Errorf("expected tool response message with id call_status_001 in round 2")
			}

			resp := openAIChatResponse{
				ID: "chatcmpl-tool-2",
				Choices: []struct {
					Index        int           `json:"index"`
					Message      openAIMessage `json:"message"`
					FinishReason string        `json:"finish_reason"`
				}{
					{
						Index: 0,
						Message: openAIMessage{
							Role:    "assistant",
							Content: ptr("We are currently docked at starport."),
						},
						FinishReason: "stop",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer ts.Close()

	mockCaller := &mockMCPCaller{
		tools: []mcp.Tool{
			mcp.NewTool("get_ship_status", mcp.WithDescription("Get ship status")),
		},
	}

	client := NewOpenAIClient("sk-test-key", "gpt-4o-mini",
		WithOpenAIBaseURL(ts.URL),
		WithOpenAIHTTPClient(ts.Client()),
		WithOpenAIMCPCaller(mockCaller),
	)

	reply, err := client.ExecuteTurn(context.Background(), nil, "Are we docked?", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "We are currently docked at starport." {
		t.Errorf("expected 'We are currently docked at starport.', got '%s'", reply)
	}
	if len(mockCaller.calledTools) != 1 || mockCaller.calledTools[0] != "get_ship_status" {
		t.Errorf("expected tool get_ship_status to be called, got %v", mockCaller.calledTools)
	}
}

func TestOpenAIURLNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1/chat/completions"},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.deepseek.com/v1", "https://api.deepseek.com/v1/chat/completions"},
		{"http://localhost:11434/v1", "http://localhost:11434/v1/chat/completions"},
		{"http://localhost:11434", "http://localhost:11434/v1/chat/completions"},
		{"https://example.com/custom/chat/completions", "https://example.com/custom/chat/completions"},
	}

	for _, tt := range tests {
		got := normalizeOpenAIChatURL(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeOpenAIChatURL(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestOpenAISystemPrompt(t *testing.T) {
	cDef := NewOpenAIClient("sk-key", "gpt-4o")
	if cDef.SystemPrompt() != DefaultSystemPrompt {
		t.Errorf("expected DefaultSystemPrompt, got %s", cDef.SystemPrompt())
	}

	customPrompt := "You are an explorer COVAS assistant."
	cCustom := NewOpenAIClient("sk-key", "gpt-4o", WithOpenAISystemPrompt(customPrompt))
	if cCustom.SystemPrompt() != customPrompt {
		t.Errorf("expected '%s', got '%s'", customPrompt, cCustom.SystemPrompt())
	}
}

func TestOpenAIErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := openAIChatResponse{
			Error: &struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    any    `json:"code"`
			}{
				Message: "Incorrect API key provided",
				Type:    "invalid_request_error",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewOpenAIClient("sk-invalid", "gpt-4o",
		WithOpenAIBaseURL(ts.URL),
		WithOpenAIHTTPClient(ts.Client()),
	)

	_, err := client.ExecuteTurn(context.Background(), nil, "Hello", nil, "")
	if err == nil {
		t.Fatalf("expected error from API response")
	}
}
