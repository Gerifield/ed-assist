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

// GeminiClient interacts with the Google Gemini API (generateContent) with function calling / MCP support.
type GeminiClient struct {
	apiKey       string
	model        string
	systemPrompt string
	maxRounds    int
	mcpCaller    MCPCaller
	httpClient   *http.Client
	baseURL      string
}

// GeminiOption configures a GeminiClient.
type GeminiOption func(*GeminiClient)

// WithGeminiHTTPClient overrides the default http.Client.
func WithGeminiHTTPClient(client *http.Client) GeminiOption {
	return func(c *GeminiClient) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithGeminiBaseURL overrides the default Gemini API base URL (useful for testing).
func WithGeminiBaseURL(url string) GeminiOption {
	return func(c *GeminiClient) {
		if url != "" {
			c.baseURL = strings.TrimRight(url, "/")
		}
	}
}

// WithGeminiMCPCaller attaches an MCP caller for function execution.
func WithGeminiMCPCaller(caller MCPCaller) GeminiOption {
	return func(c *GeminiClient) {
		c.mcpCaller = caller
	}
}

// WithGeminiSystemPrompt sets a custom system instruction prompt for Gemini.
func WithGeminiSystemPrompt(prompt string) GeminiOption {
	return func(c *GeminiClient) {
		if strings.TrimSpace(prompt) != "" {
			c.systemPrompt = strings.TrimSpace(prompt)
		}
	}
}

// WithGeminiMaxRounds sets the maximum allowed tool execution rounds per turn.
func WithGeminiMaxRounds(rounds int) GeminiOption {
	return func(c *GeminiClient) {
		if rounds > 0 {
			c.maxRounds = rounds
		}
	}
}

// NewGeminiClient creates a new Gemini client.
func NewGeminiClient(apiKey, model string, opts ...GeminiOption) *GeminiClient {
	if model == "" {
		model = "gemini-flash-lite-latest"
	}
	c := &GeminiClient{
		apiKey:       apiKey,
		model:        model,
		systemPrompt: DefaultSystemPrompt,
		maxRounds:    10,
		httpClient:   &http.Client{Timeout: 90 * time.Second},
		baseURL:      "https://generativelanguage.googleapis.com/v1beta",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ModelName returns the model identifier.
func (c *GeminiClient) ModelName() string {
	return c.model
}

// Provider returns the provider identifier.
func (c *GeminiClient) Provider() string {
	return "gemini"
}

// SystemPrompt returns the currently configured system instruction prompt.
func (c *GeminiClient) SystemPrompt() string {
	if strings.TrimSpace(c.systemPrompt) != "" {
		return c.systemPrompt
	}
	return DefaultSystemPrompt
}

// geminiPart represents a part of a message (text, functionCall, functionResponse, inlineData).
type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thought_signature,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
}

// UnmarshalJSON captures both snake_case and camelCase thought signatures on geminiPart.
func (p *geminiPart) UnmarshalJSON(data []byte) error {
	type Alias geminiPart
	aux := struct {
		*Alias
		CamelSig string `json:"thoughtSignature"`
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if p.ThoughtSignature == "" && aux.CamelSig != "" {
		p.ThoughtSignature = aux.CamelSig
	}
	return nil
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type geminiFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role"` // "user" or "model"
	Parts []geminiPart `json:"parts"`
}

type geminiToolDeclaration struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type geminiGenerateContentRequest struct {
	Contents          []geminiContent         `json:"contents"`
	Tools             []geminiToolDeclaration `json:"tools,omitempty"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
}

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content struct {
			Role  string       `json:"role"`
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// ExecuteTurn processes a user prompt (text and/or audio), interacts with Gemini,
// executes any requested MCP tools in a loop, and returns the final assistant text.
func (c *GeminiClient) ExecuteTurn(ctx context.Context, history []ChatMessage, userPrompt string, audioData []byte, audioMime string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("gemini API key is not configured (set gemini_api_key in config.ini or GEMINI_API_KEY environment variable)")
	}

	var contents []geminiContent

	// Convert history into Gemini contents
	for _, m := range history {
		if m.Text == "" {
			continue
		}
		role := m.Role
		if role != "user" && role != "model" {
			role = "user"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Text}},
		})
	}

	// Prepare current turn parts
	var currentParts []geminiPart
	if len(audioData) > 0 {
		if audioMime == "" {
			audioMime = "audio/webm"
		}
		currentParts = append(currentParts, geminiPart{
			InlineData: &geminiInlineData{
				MimeType: audioMime,
				Data:     string(audioData),
			},
		})
	}
	if userPrompt != "" {
		currentParts = append(currentParts, geminiPart{Text: userPrompt})
	}

	if len(currentParts) == 0 {
		return "", fmt.Errorf("no user prompt or audio data provided")
	}

	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: currentParts,
	})

	// Fetch available MCP tools if caller configured
	var tools []geminiToolDeclaration
	if c.mcpCaller != nil {
		mcpTools, err := c.mcpCaller.ListTools(ctx)
		if err == nil && len(mcpTools) > 0 {
			var decls []geminiFunctionDeclaration
			for _, t := range mcpTools {
				decls = append(decls, geminiFunctionDeclaration{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  cleanParameters(t.InputSchema),
				})
			}
			if len(decls) > 0 {
				tools = append(tools, geminiToolDeclaration{FunctionDeclarations: decls})
			}
		}
	}

	systemInstruction := &geminiContent{
		Role: "user",
		Parts: []geminiPart{
			{
				Text: c.SystemPrompt(),
			},
		},
	}

	// Multi-turn tool execution loop (max rounds to avoid infinite loops)
	maxRounds := c.maxRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}
	for round := 0; round < maxRounds; round++ {
		reqPayload := geminiGenerateContentRequest{
			Contents:          contents,
			Tools:             tools,
			SystemInstruction: systemInstruction,
		}

		resp, err := c.postGenerateContent(ctx, reqPayload)
		if err != nil {
			return "", err
		}

		if len(resp.Candidates) == 0 {
			return "No response received from Gemini.", nil
		}

		candidate := resp.Candidates[0]
		if candidate.Content.Role == "" {
			candidate.Content.Role = "model"
		}

		// Append model output turn to conversation
		contents = append(contents, candidate.Content)

		// Check if candidate contains function calls
		var calls []*geminiFunctionCall
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				calls = append(calls, part.FunctionCall)
			}
		}

		// If no function calls, collect text response and return
		if len(calls) == 0 {
			var textParts []string
			for _, part := range candidate.Content.Parts {
				if part.Thought {
					continue
				}
				if part.Text != "" {
					textParts = append(textParts, part.Text)
				}
			}
			if len(textParts) == 0 {
				return "Action completed nominal, Commander.", nil
			}
			return strings.Join(textParts, "\n\n"), nil
		}

		// Execute function calls via MCP caller
		if c.mcpCaller == nil {
			return "", fmt.Errorf("model requested tool execution but no MCPCaller is attached")
		}

		var responseParts []geminiPart
		for _, call := range calls {
			resStr, err := c.mcpCaller.CallTool(ctx, call.Name, call.Args)
			respMap := make(map[string]any)
			if err != nil {
				respMap["error"] = err.Error()
			} else {
				var jsonParsed any
				if err := json.Unmarshal([]byte(resStr), &jsonParsed); err == nil {
					respMap["result"] = jsonParsed
				} else {
					respMap["result"] = resStr
				}
			}

			responseParts = append(responseParts, geminiPart{
				FunctionResponse: &geminiFunctionResponse{
					ID:       call.ID,
					Name:     call.Name,
					Response: respMap,
				},
			})
		}

		// Feed tool execution responses back as a user turn
		contents = append(contents, geminiContent{
			Role:  "user",
			Parts: responseParts,
		})
	}

	return "Maximum tool call iterations reached without a final answer.", nil
}

func (c *GeminiClient) postGenerateContent(ctx context.Context, payload geminiGenerateContentRequest) (*geminiGenerateContentResponse, error) {
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini API HTTP request failed: %w", err)
	}
	defer res.Body.Close()

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading response body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		var errResp geminiGenerateContentResponse
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Error != nil {
			return nil, fmt.Errorf("gemini API error (code %d: %s): %s", errResp.Error.Code, errResp.Error.Status, errResp.Error.Message)
		}
		return nil, fmt.Errorf("gemini API returned HTTP status %d: %s", res.StatusCode, string(respBody))
	}

	var genResp geminiGenerateContentResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return nil, fmt.Errorf("failed decoding Gemini response: %w", err)
	}

	return &genResp, nil
}

// cleanParameters sanitizes the JSON schema representation from mcp.Tool.InputSchema
// to match an OpenAPI-compatible schema subset.
func cleanParameters(schema interface{}) interface{} {
	if schema == nil {
		return nil
	}
	bytes, err := json.Marshal(schema)
	if err != nil {
		return schema
	}
	var m map[string]interface{}
	if err := json.Unmarshal(bytes, &m); err != nil {
		return schema
	}
	delete(m, "$schema")
	delete(m, "additionalProperties")
	return m
}
