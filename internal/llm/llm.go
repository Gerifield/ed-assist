package llm

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// MCPCaller defines an abstraction for listing and executing MCP tools.
type MCPCaller interface {
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (string, error)
}

// DefaultSystemPrompt is the default system instruction given to the COVAS assistant.
const DefaultSystemPrompt = "You are an Elite Dangerous AI Cockpit Assistant (COVAS). " +
	"You have direct access to the ship's telemetry, navigation status, SQLite visited/targeted system history, and in-game controls via MCP tools. " +
	"When the commander asks for status, navigation info, fuel, visited systems, or commands a ship action (such as landing gear, lights, hardpoints, cargo scoop, night vision, boost, pips), " +
	"call the appropriate MCP tool to inspect or command the ship. Keep responses immersive, concise, and helpful like a ship computer."

// ChatMessage represents a user or assistant message in conversation history.
type ChatMessage struct {
	Role      string `json:"role"` // "user", "assistant", or "model"
	Text      string `json:"text,omitempty"`
	AudioB64  string `json:"audio_b64,omitempty"`
	AudioMime string `json:"audio_mime,omitempty"`
}

// Client is the common interface implemented by AI providers (Gemini, OpenAI-compatible, etc.).
type Client interface {
	// ExecuteTurn processes a user prompt (text and/or audio) within the given conversation history,
	// executes any requested MCP tools in an iterative loop, and returns the final assistant reply.
	ExecuteTurn(ctx context.Context, history []ChatMessage, userPrompt string, audioData []byte, audioMime string) (string, error)

	// SystemPrompt returns the active system instruction prompt.
	SystemPrompt() string

	// ModelName returns the configured model identifier.
	ModelName() string

	// Provider returns the provider name (e.g. "gemini", "openai").
	Provider() string
}
