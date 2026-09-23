package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient interacts with OpenAI-compatible Chat Completions APIs
// (OpenAI, DeepSeek, Groq, Ollama, LM Studio, OpenRouter, etc.) with tool calling support.
type OpenAIClient struct {
	apiKey       string
	model        string
	baseURL      string
	systemPrompt string
	maxRounds    int
	autoTime     bool
	mcpCaller    MCPCaller
	httpClient   *http.Client
}

// OpenAIOption configures an OpenAIClient.
type OpenAIOption func(*OpenAIClient)

// WithOpenAIHTTPClient overrides the default http.Client.
func WithOpenAIHTTPClient(client *http.Client) OpenAIOption {
	return func(c *OpenAIClient) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithOpenAIBaseURL overrides the default OpenAI base URL (default: "https://api.openai.com/v1").
func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(c *OpenAIClient) {
		if url != "" {
			c.baseURL = strings.TrimRight(url, "/")
		}
	}
}

// WithOpenAIMCPCaller attaches an MCP caller for tool execution.
func WithOpenAIMCPCaller(caller MCPCaller) OpenAIOption {
	return func(c *OpenAIClient) {
		c.mcpCaller = caller
	}
}

// WithOpenAISystemPrompt sets a custom system instruction prompt.
func WithOpenAISystemPrompt(prompt string) OpenAIOption {
	return func(c *OpenAIClient) {
		if strings.TrimSpace(prompt) != "" {
			c.systemPrompt = strings.TrimSpace(prompt)
		}
	}
}

// WithOpenAIMaxRounds sets the maximum allowed tool execution rounds per turn.
func WithOpenAIMaxRounds(rounds int) OpenAIOption {
	return func(c *OpenAIClient) {
		if rounds > 0 {
			c.maxRounds = rounds
		}
	}
}

// WithOpenAIAutoTime enables or disables automatic time/chronometer injection into the prompt.
func WithOpenAIAutoTime(enable bool) OpenAIOption {
	return func(c *OpenAIClient) {
		c.autoTime = enable
	}
}

// NewOpenAIClient creates a new OpenAI-compatible client.
func NewOpenAIClient(apiKey, model string, opts ...OpenAIOption) *OpenAIClient {
	if model == "" {
		model = "gpt-4o-mini"
	}
	c := &OpenAIClient{
		apiKey:       apiKey,
		model:        model,
		baseURL:      "https://api.openai.com/v1",
		systemPrompt: DefaultSystemPrompt,
		maxRounds:    10,
		autoTime:     true,
		httpClient:   &http.Client{Timeout: 90 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ModelName returns the configured model identifier.
func (c *OpenAIClient) ModelName() string {
	return c.model
}

// Provider returns the provider identifier.
func (c *OpenAIClient) Provider() string {
	return "openai"
}

// SystemPrompt returns the currently configured system instruction prompt.
func (c *OpenAIClient) SystemPrompt() string {
	if strings.TrimSpace(c.systemPrompt) != "" {
		return c.systemPrompt
	}
	return DefaultSystemPrompt
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
}

type openAIMessage struct {
	Role       string            `json:"role"`
	Content    *string           `json:"content"`
	ToolCalls  []openAIToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"` // "function"
	Function openAIFunctionDesc `json:"function"`
}

type openAIFunctionDesc struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // "function"
	Function openAIFunctionCallDesc `json:"function"`
}

type openAIFunctionCallDesc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

// ExecuteTurn processes a user prompt within the conversation history,
// executes any requested MCP tools in a loop, and returns the final assistant text.
func (c *OpenAIClient) ExecuteTurn(ctx context.Context, history []ChatMessage, userPrompt string, audioData []byte, audioMime string) (string, error) {
	// If OpenAI official endpoint is used without an API key, reject early
	isLocal := strings.Contains(c.baseURL, "localhost") || strings.Contains(c.baseURL, "127.0.0.1")
	if c.apiKey == "" && !isLocal {
		return "", fmt.Errorf("OpenAI API key is not configured (set openai_api_key in config.ini or OPENAI_API_KEY environment variable)")
	}

	var messages []openAIMessage

	// 1. Prepend system instruction
	sysPrompt := c.SystemPrompt()
	if c.autoTime {
		sysPrompt = BuildTurnSystemPrompt(sysPrompt)
	}
	messages = append(messages, openAIMessage{
		Role:    "system",
		Content: &sysPrompt,
	})

	// 2. Convert conversation history
	for _, m := range history {
		if m.Text == "" {
			continue
		}
		role := m.Role
		if role == "model" {
			role = "assistant"
		} else if role != "user" && role != "assistant" && role != "system" {
			role = "user"
		}
		text := m.Text
		messages = append(messages, openAIMessage{
			Role:    role,
			Content: &text,
		})
	}

	// 3. Prepare current user prompt
	if userPrompt != "" {
		messages = append(messages, openAIMessage{
			Role:    "user",
			Content: &userPrompt,
		})
	} else if len(audioData) > 0 {
		return "", fmt.Errorf("audio streaming is not supported by text-based OpenAI-compatible endpoints; please provide text input or switch to Gemini")
	} else {
		return "", fmt.Errorf("no user prompt or audio data provided")
	}

	// 4. Prepare MCP tool declarations
	var tools []openAITool
	if c.mcpCaller != nil {
		mcpTools, err := c.mcpCaller.ListTools(ctx)
		if err == nil && len(mcpTools) > 0 {
			for _, t := range mcpTools {
				tools = append(tools, openAITool{
					Type: "function",
					Function: openAIFunctionDesc{
						Name:        t.Name,
						Description: t.Description,
						Parameters:  cleanParameters(t.InputSchema),
					},
				})
			}
		}
	}

	// 5. Multi-turn execution loop (max rounds to avoid infinite loops)
	maxRounds := c.maxRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}
	for round := 0; round < maxRounds; round++ {
		reqPayload := openAIChatRequest{
			Model:    c.model,
			Messages: messages,
			Tools:    tools,
		}

		resp, err := c.postChatCompletions(ctx, reqPayload)
		if err != nil {
			return "", err
		}

		if len(resp.Choices) == 0 {
			return "No response received from model.", nil
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message
		if assistantMsg.Role == "" {
			assistantMsg.Role = "assistant"
		}

		// Append assistant response to messages history
		messages = append(messages, assistantMsg)

		// Check if the assistant requested tool calls
		if len(assistantMsg.ToolCalls) == 0 {
			if assistantMsg.Content != nil && *assistantMsg.Content != "" {
				return SanitizeReply(*assistantMsg.Content), nil
			}
			return "Action completed nominal, Commander.", nil
		}

		if c.mcpCaller == nil {
			return "", fmt.Errorf("model requested tool execution but no MCPCaller is attached")
		}

		// Execute all requested tool calls
		for _, tc := range assistantMsg.ToolCalls {
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			if args == nil {
				args = make(map[string]any)
			}

			resStr, err := c.mcpCaller.CallTool(ctx, tc.Function.Name, args)
			if err != nil {
				resStr = fmt.Sprintf(`{"error": %q}`, err.Error())
			}

			messages = append(messages, openAIMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    &resStr,
			})
		}
	}

	return "Maximum tool call iterations reached without a final answer.", nil
}

func (c *OpenAIClient) postChatCompletions(ctx context.Context, payload openAIChatRequest) (*openAIChatResponse, error) {
	url := normalizeOpenAIChatURL(c.baseURL)

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI-compatible API request failed: %w", err)
	}
	defer res.Body.Close()

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading response body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		var errResp openAIChatResponse
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Error != nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("OpenAI API error (%s): %s", errResp.Error.Type, errResp.Error.Message)
		}
		return nil, fmt.Errorf("OpenAI API returned HTTP status %d: %s", res.StatusCode, string(respBody))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed decoding OpenAI response: %w", err)
	}

	return &chatResp, nil
}

// normalizeOpenAIChatURL constructs the full /chat/completions endpoint URL
// from any baseURL variant (e.g. "https://api.deepseek.com", "https://api.openai.com/v1", "http://localhost:11434/v1").
func normalizeOpenAIChatURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/chat/completions"
	}
	return trimmed + "/v1/chat/completions"
}
