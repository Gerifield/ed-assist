package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// MCPCaller defines an abstraction for listing and executing MCP tools.
type MCPCaller interface {
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (string, error)
}

// Client interacts with the Google Gemini API (generateContent) with function calling / MCP support.
type Client struct {
	apiKey     string
	model      string
	mcpCaller  MCPCaller
	httpClient *http.Client
	baseURL    string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the default http.Client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithBaseURL overrides the default Gemini API base URL (useful for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.baseURL = strings.TrimRight(url, "/")
		}
	}
}

// WithMCPCaller attaches an MCP caller for function execution.
func WithMCPCaller(caller MCPCaller) Option {
	return func(c *Client) {
		c.mcpCaller = caller
	}
}

// NewClient creates a new Gemini client.
func NewClient(apiKey, model string, opts ...Option) *Client {
	if model == "" {
		model = "gemini-3.8-flash-lite"
	}
	c := &Client{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 90 * time.Second},
		baseURL:    "https://generativelanguage.googleapis.com/v1beta",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Part represents a part of a message (text, functionCall, functionResponse, inlineData).
type Part struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
	InlineData       *InlineData       `json:"inlineData,omitempty"`
}

// InlineData represents inline binary media (e.g. audio/webm or audio/wav).
type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

// FunctionCall represents Gemini requesting tool invocation.
type FunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// FunctionResponse represents the result of tool execution returned to Gemini.
type FunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// Content represents a conversation turn.
type Content struct {
	Role  string `json:"role"` // "user" or "model"
	Parts []Part `json:"parts"`
}

// ToolDeclaration represents tools passed to Gemini.
type ToolDeclaration struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// FunctionDeclaration describes a tool function for Gemini.
type FunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// GenerateContentRequest is the request payload to Gemini API.
type GenerateContentRequest struct {
	Contents          []Content        `json:"contents"`
	Tools             []ToolDeclaration `json:"tools,omitempty"`
	SystemInstruction *Content         `json:"systemInstruction,omitempty"`
}

// GenerateContentResponse is the response payload from Gemini API.
type GenerateContentResponse struct {
	Candidates []struct {
		Content struct {
			Role  string `json:"role"`
			Parts []Part `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// ChatMessage represents a user or assistant message for UI exchange.
type ChatMessage struct {
	Role      string `json:"role"` // "user" or "model"
	Text      string `json:"text,omitempty"`
	AudioB64  string `json:"audio_b64,omitempty"`
	AudioMime string `json:"audio_mime,omitempty"`
}

// ExecuteTurn processes a user prompt (text and/or audio), interacts with Gemini,
// executes any requested MCP tools in a loop, and returns the final assistant text.
func (c *Client) ExecuteTurn(ctx context.Context, history []ChatMessage, userPrompt string, audioData []byte, audioMime string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("gemini API key is not configured (set gemini_api_key in config.ini or GEMINI_API_KEY environment variable)")
	}

	var contents []Content

	// Convert history into Gemini contents
	for _, m := range history {
		if m.Text == "" {
			continue
		}
		role := m.Role
		if role != "user" && role != "model" {
			role = "user"
		}
		contents = append(contents, Content{
			Role:  role,
			Parts: []Part{{Text: m.Text}},
		})
	}

	// Prepare current turn parts
	var currentParts []Part
	if len(audioData) > 0 {
		if audioMime == "" {
			audioMime = "audio/webm"
		}
		currentParts = append(currentParts, Part{
			InlineData: &InlineData{
				MimeType: audioMime,
				Data:     string(audioData), // Caller passes base64 string or bytes
			},
		})
	}
	if userPrompt != "" {
		currentParts = append(currentParts, Part{Text: userPrompt})
	}

	if len(currentParts) == 0 {
		return "", fmt.Errorf("no user prompt or audio data provided")
	}

	contents = append(contents, Content{
		Role:  "user",
		Parts: currentParts,
	})

	// Fetch available MCP tools if caller configured
	var tools []ToolDeclaration
	if c.mcpCaller != nil {
		mcpTools, err := c.mcpCaller.ListTools(ctx)
		if err == nil && len(mcpTools) > 0 {
			var decls []FunctionDeclaration
			for _, t := range mcpTools {
				decls = append(decls, FunctionDeclaration{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  cleanParameters(t.InputSchema),
				})
			}
			if len(decls) > 0 {
				tools = append(tools, ToolDeclaration{FunctionDeclarations: decls})
			}
		}
	}

	systemInstruction := &Content{
		Role: "user",
		Parts: []Part{
			{
				Text: "You are an Elite Dangerous AI Cockpit Assistant (COVAS). " +
					"You have direct access to the ship's telemetry, navigation status, SQLite visited/targeted system history, and in-game controls via MCP tools. " +
					"When the commander asks for status, navigation info, fuel, visited systems, or commands a ship action (such as landing gear, lights, hardpoints, cargo scoop, night vision, boost, pips), " +
					"call the appropriate MCP tool to inspect or command the ship. Keep responses immersive, concise, and helpful like a ship computer.",
			},
		},
	}

	// Multi-turn tool execution loop (max 10 rounds to avoid infinite loops)
	maxRounds := 10
	for round := 0; round < maxRounds; round++ {
		reqPayload := GenerateContentRequest{
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
		contents = append(contents, candidate.Content)

		// Check if the model called any tools
		var toolCalls []*FunctionCall
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				toolCalls = append(toolCalls, part.FunctionCall)
			}
		}

		// If no tool calls, return accumulated text response
		if len(toolCalls) == 0 {
			var sb strings.Builder
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					sb.WriteString(part.Text)
				}
			}
			return strings.TrimSpace(sb.String()), nil
		}

		// Execute tool calls via MCP caller
		if c.mcpCaller == nil {
			return "", fmt.Errorf("gemini requested tool call %s, but no MCP caller is configured", toolCalls[0].Name)
		}

		var responseParts []Part
		for _, call := range toolCalls {
			toolResult, err := c.mcpCaller.CallTool(ctx, call.Name, call.Args)
			var respMap map[string]any
			if err != nil {
				respMap = map[string]any{"error": err.Error()}
			} else {
				// Try to unmarshal as JSON or treat as text
				var parsed any
				if jsonErr := json.Unmarshal([]byte(toolResult), &parsed); jsonErr == nil {
					respMap = map[string]any{"result": parsed}
				} else {
					respMap = map[string]any{"result": toolResult}
				}
			}

			responseParts = append(responseParts, Part{
				FunctionResponse: &FunctionResponse{
					Name:     call.Name,
					Response: respMap,
				},
			})
		}

		// Feed tool execution responses back to Gemini
		contents = append(contents, Content{
			Role:  "user",
			Parts: responseParts,
		})
	}

	return "Maximum tool call iterations reached without a final answer.", nil
}

func (c *Client) postGenerateContent(ctx context.Context, payload GenerateContentRequest) (*GenerateContentResponse, error) {
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
		var errResp GenerateContentResponse
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Error != nil {
			return nil, fmt.Errorf("gemini API error (code %d: %s): %s", errResp.Error.Code, errResp.Error.Status, errResp.Error.Message)
		}
		return nil, fmt.Errorf("gemini API returned HTTP status %d: %s", res.StatusCode, string(respBody))
	}

	var genResp GenerateContentResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return nil, fmt.Errorf("failed decoding Gemini response: %w", err)
	}

	return &genResp, nil
}

// cleanParameters sanitizes the JSON schema representation from mcp.Tool.InputSchema
// to match Gemini's OpenAPI-compatible schema subset.
func cleanParameters(schema interface{}) interface{} {
	if schema == nil {
		return nil
	}
	// Convert through JSON map to ensure clean structure
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
