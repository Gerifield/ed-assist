package gemini

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
		resp := GenerateContentResponse{
			Candidates: []struct {
				Content struct {
					Role  string `json:"role"`
					Parts []Part `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Role  string `json:"role"`
						Parts []Part `json:"parts"`
					}{
						Role:  "model",
						Parts: []Part{{Text: "Commander, all systems nominal."}},
					},
					FinishReason: "STOP",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient("test-key", "gemini-3.8-flash-lite", WithBaseURL(ts.URL), WithHTTPClient(ts.Client()))

	reply, err := client.ExecuteTurn(context.Background(), nil, "Status report", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "Commander, all systems nominal." {
		t.Errorf("expected reply 'Commander, all systems nominal.', got '%s'", reply)
	}
}

func TestGeminiExecuteTurnWithToolCalling(t *testing.T) {
	rounds := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		if rounds == 1 {
			// Round 1: Model asks to call get_ship_status with thought_signature on Part
			resp := GenerateContentResponse{
				Candidates: []struct {
					Content struct {
						Role  string `json:"role"`
						Parts []Part `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				}{
					{
						Content: struct {
							Role  string `json:"role"`
							Parts []Part `json:"parts"`
						}{
							Role: "model",
							Parts: []Part{
								{
									ThoughtSignature: "test_thought_sig_abc123",
									FunctionCall: &FunctionCall{
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
			return
		}

		// Round 2: verify thought_signature is on Part, NOT inside functionCall, and ID is preserved
		var req GenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed decoding round 2 request: %v", err)
		}

		foundSigOnPart := false
		for _, msg := range req.Contents {
			for _, p := range msg.Parts {
				if p.ThoughtSignature == "test_thought_sig_abc123" {
					foundSigOnPart = true
				}
				if p.FunctionResponse != nil && p.FunctionResponse.ID != "call_abc" {
					t.Errorf("expected FunctionResponse ID 'call_abc', got '%s'", p.FunctionResponse.ID)
				}
			}
		}
		if !foundSigOnPart {
			t.Errorf("round 2 request missing thought_signature on echoed model Part")
		}

		// Model returns final text with tool result
		resp := GenerateContentResponse{
			Candidates: []struct {
				Content struct {
					Role  string `json:"role"`
					Parts []Part `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Role  string `json:"role"`
						Parts []Part `json:"parts"`
					}{
						Role:  "model",
						Parts: []Part{{Text: "We are currently safely docked in ship mode."}},
					},
					FinishReason: "STOP",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	mockCaller := &mockMCPCaller{
		tools: []mcp.Tool{
			mcp.NewTool("get_ship_status", mcp.WithDescription("Get ship status")),
		},
	}

	client := NewClient("test-key", "gemini-3.8-flash-lite",
		WithBaseURL(ts.URL),
		WithHTTPClient(ts.Client()),
		WithMCPCaller(mockCaller),
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
	client := NewClient("", "gemini-3.8-flash-lite")
	_, err := client.ExecuteTurn(context.Background(), nil, "Hello", nil, "")
	if err == nil {
		t.Fatalf("expected error when API key is missing")
	}
}
