package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

type mockMCPCaller struct {
	tools       []mcp.Tool
	calledTools []string
}

func (m *mockMCPCaller) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	return m.tools, nil
}

func (m *mockMCPCaller) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	m.calledTools = append(m.calledTools, name)
	if name == "get_ship_status" {
		return `{"mode":"Ship", "docked":true}`, nil
	}
	return `{"status":"ok"}`, nil
}

func TestGeminiExecuteTurnDirectText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiGenerateContentResponse{
			Candidates: []struct {
				Content struct {
					Role  string       `json:"role"`
					Parts []geminiPart `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Role  string       `json:"role"`
						Parts []geminiPart `json:"parts"`
					}{
						Role:  "model",
						Parts: []geminiPart{{Text: "Commander, all systems nominal."}},
					},
					FinishReason: "STOP",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewGeminiClient("test-key", "gemini-3.8-flash-lite", WithGeminiBaseURL(ts.URL), WithGeminiHTTPClient(ts.Client()))

	reply, err := client.ExecuteTurn(context.Background(), nil, "Status report", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "Commander, all systems nominal." {
		t.Errorf("expected reply 'Commander, all systems nominal.', got '%s'", reply)
	}
	if client.Provider() != "gemini" {
		t.Errorf("expected provider 'gemini', got '%s'", client.Provider())
	}
	if client.ModelName() != "gemini-3.8-flash-lite" {
		t.Errorf("expected model 'gemini-3.8-flash-lite', got '%s'", client.ModelName())
	}
}

func TestGeminiExecuteTurnWithToolCalling(t *testing.T) {
	rounds := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		if rounds == 1 {
			resp := geminiGenerateContentResponse{
				Candidates: []struct {
					Content struct {
						Role  string       `json:"role"`
						Parts []geminiPart `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				}{
					{
						Content: struct {
							Role  string       `json:"role"`
							Parts []geminiPart `json:"parts"`
						}{
							Role: "model",
							Parts: []geminiPart{
								{
									ThoughtSignature: "test_thought_sig_abc123",
									FunctionCall: &geminiFunctionCall{
										ID:   "call_abc",
										Name: "get_ship_status",
										Args: map[string]any{},
									},
								},
							},
						},
						FinishReason: "STOP",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			resp := geminiGenerateContentResponse{
				Candidates: []struct {
					Content struct {
						Role  string       `json:"role"`
						Parts []geminiPart `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				}{
					{
						Content: struct {
							Role  string       `json:"role"`
							Parts []geminiPart `json:"parts"`
						}{
							Role:  "model",
							Parts: []geminiPart{{Text: "We are currently safely docked in ship mode."}},
						},
						FinishReason: "STOP",
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

	client := NewGeminiClient("test-key", "gemini-3.8-flash-lite",
		WithGeminiBaseURL(ts.URL),
		WithGeminiHTTPClient(ts.Client()),
		WithGeminiMCPCaller(mockCaller),
	)

	reply, err := client.ExecuteTurn(context.Background(), nil, "Are we docked?", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "We are currently safely docked in ship mode." {
		t.Errorf("expected final reply, got '%s'", reply)
	}

	if len(mockCaller.calledTools) != 1 || mockCaller.calledTools[0] != "get_ship_status" {
		t.Errorf("expected get_ship_status to have been invoked, got %v", mockCaller.calledTools)
	}
}

func TestGeminiExecuteTurnRequiresAPIKey(t *testing.T) {
	client := NewGeminiClient("", "gemini-3.8-flash-lite")
	_, err := client.ExecuteTurn(context.Background(), nil, "Hello", nil, "")
	if err == nil {
		t.Fatalf("expected error when API key is missing")
	}
}

func TestGeminiSystemPrompt(t *testing.T) {
	cDef := NewGeminiClient("test-key", "gemini-flash-lite-latest")
	if cDef.SystemPrompt() != DefaultSystemPrompt {
		t.Errorf("expected DefaultSystemPrompt, got %s", cDef.SystemPrompt())
	}

	customPrompt := "You are a specialized pirate COVAS assistant."
	var capturedPrompt string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiGenerateContentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.SystemInstruction != nil && len(req.SystemInstruction.Parts) > 0 {
			capturedPrompt = req.SystemInstruction.Parts[0].Text
		}
		resp := geminiGenerateContentResponse{
			Candidates: []struct {
				Content struct {
					Role  string       `json:"role"`
					Parts []geminiPart `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Role  string       `json:"role"`
						Parts []geminiPart `json:"parts"`
					}{
						Role:  "model",
						Parts: []geminiPart{{Text: "Ahoy commander."}},
					},
					FinishReason: "STOP",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	cCustom := NewGeminiClient("test-key", "gemini-flash-lite-latest",
		WithGeminiSystemPrompt(customPrompt),
		WithGeminiBaseURL(ts.URL),
		WithGeminiHTTPClient(ts.Client()),
	)

	if cCustom.SystemPrompt() != customPrompt {
		t.Errorf("expected custom prompt '%s', got '%s'", customPrompt, cCustom.SystemPrompt())
	}

	reply, err := cCustom.ExecuteTurn(context.Background(), nil, "Greeting", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "Ahoy commander." {
		t.Errorf("expected 'Ahoy commander.', got '%s'", reply)
	}
	if capturedPrompt != customPrompt {
		t.Errorf("expected captured system prompt in HTTP request '%s', got '%s'", customPrompt, capturedPrompt)
	}
}
