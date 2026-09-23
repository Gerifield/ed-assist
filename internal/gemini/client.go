package gemini

import (
	"net/http"

	"ed-assist/internal/llm"
)

// DefaultSystemPrompt is the default system instruction given to the COVAS assistant.
const DefaultSystemPrompt = llm.DefaultSystemPrompt

// ChatMessage represents a user or assistant message for UI exchange.
type ChatMessage = llm.ChatMessage

// MCPCaller defines an abstraction for listing and executing MCP tools.
type MCPCaller = llm.MCPCaller

// Client alias to llm.GeminiClient for backwards compatibility.
type Client = llm.GeminiClient

// Option configures a Client.
type Option = llm.GeminiOption

// WithHTTPClient overrides the default http.Client.
func WithHTTPClient(client *http.Client) Option {
	return llm.WithGeminiHTTPClient(client)
}

// WithBaseURL overrides the default Gemini API base URL.
func WithBaseURL(url string) Option {
	return llm.WithGeminiBaseURL(url)
}

// WithMCPCaller attaches an MCP caller for function execution.
func WithMCPCaller(caller MCPCaller) Option {
	return llm.WithGeminiMCPCaller(caller)
}

// WithSystemPrompt sets a custom system instruction prompt for Gemini.
func WithSystemPrompt(prompt string) Option {
	return llm.WithGeminiSystemPrompt(prompt)
}

// NewClient creates a new Gemini client.
func NewClient(apiKey, model string, opts ...Option) *Client {
	return llm.NewGeminiClient(apiKey, model, opts...)
}
