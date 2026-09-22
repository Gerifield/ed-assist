package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ed-assist/internal/llm"
	"github.com/mark3labs/mcp-go/mcp"
)

type mockGeminiCaller struct {
	tools []mcp.Tool
}

func (m *mockGeminiCaller) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	return m.tools, nil
}

func (m *mockGeminiCaller) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	return `{"result":"ok"}`, nil
}

func TestWebServerInfoAndStatic(t *testing.T) {
	geminiClient := llm.NewGeminiClient("fake-key", "gemini-3.8-flash-lite")
	server := NewServer("127.0.0.1:0", geminiClient, nil, "gemini-3.8-flash-lite", 42, 1800)

	// Direct handler test
	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", server.handleInfo)
	mux.HandleFunc("/api/chat", server.handleChat)

	// Test /api/info
	reqInfo := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	wInfo := httptest.NewRecorder()
	mux.ServeHTTP(wInfo, reqInfo)

	if wInfo.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", wInfo.Code)
	}

	var info map[string]any
	if err := json.NewDecoder(wInfo.Body).Decode(&info); err != nil {
		t.Fatalf("failed decoding info json: %v", err)
	}

	if info["model"] != "gemini-3.8-flash-lite" {
		t.Errorf("expected gemini-3.8-flash-lite, got %v", info["model"])
	}
	if info["provider"] != "gemini" {
		t.Errorf("expected provider 'gemini', got %v", info["provider"])
	}
	if info["voice_gate_threshold"] != float64(42) {
		t.Errorf("expected voice_gate_threshold 42, got %v", info["voice_gate_threshold"])
	}
	if info["voice_silence_ms"] != float64(1800) {
		t.Errorf("expected voice_silence_ms 1800, got %v", info["voice_silence_ms"])
	}
	if info["voice_echo_protection"] != true {
		t.Errorf("expected voice_echo_protection true by default, got %v", info["voice_echo_protection"])
	}
	if info["system_prompt"] != llm.DefaultSystemPrompt {
		t.Errorf("expected default system_prompt, got %v", info["system_prompt"])
	}

	// Test with WithEchoProtection(false)
	serverNoEcho := NewServer("127.0.0.1:0", geminiClient, nil, "gemini-3.8-flash-lite", 42, 1800, WithEchoProtection(false))
	wNoEcho := httptest.NewRecorder()
	serverNoEcho.handleInfo(wNoEcho, reqInfo)
	var infoNoEcho map[string]any
	_ = json.NewDecoder(wNoEcho.Body).Decode(&infoNoEcho)
	if infoNoEcho["voice_echo_protection"] != false {
		t.Errorf("expected voice_echo_protection false, got %v", infoNoEcho["voice_echo_protection"])
	}
}

func TestWebServerChatEndpointWithGemini(t *testing.T) {
	tsGemini := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "All landing gear retracted, Commander."},
						},
					},
					"finishReason": "STOP",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer tsGemini.Close()

	geminiClient := llm.NewGeminiClient("test-key", "gemini-3.8-flash-lite",
		llm.WithGeminiBaseURL(tsGemini.URL),
		llm.WithGeminiHTTPClient(tsGemini.Client()),
	)

	server := NewServer("127.0.0.1:0", geminiClient, nil, "gemini-3.8-flash-lite", 40, 2000)

	chatReq := ChatRequest{
		Prompt: "Gear status?",
	}
	body, _ := json.Marshal(chatReq)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleChat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}

	if resp.Reply != "All landing gear retracted, Commander." {
		t.Errorf("expected reply, got %s", resp.Reply)
	}
}

func TestWebServerChatEndpointWithOpenAI(t *testing.T) {
	tsOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "DeepSeek COVAS online. Shields nominal, Commander.",
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer tsOpenAI.Close()

	openAIClient := llm.NewOpenAIClient("sk-test-deepseek", "deepseek-chat",
		llm.WithOpenAIBaseURL(tsOpenAI.URL),
		llm.WithOpenAIHTTPClient(tsOpenAI.Client()),
	)

	server := NewServer("127.0.0.1:0", openAIClient, nil, "deepseek-chat", 40, 2000)

	chatReq := ChatRequest{
		Prompt: "Shield status?",
	}
	body, _ := json.Marshal(chatReq)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleChat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}

	if resp.Reply != "DeepSeek COVAS online. Shields nominal, Commander." {
		t.Errorf("expected reply, got %s", resp.Reply)
	}
}
