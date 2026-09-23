package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// MCPCaller defines an abstraction for listing and executing MCP tools.
type MCPCaller interface {
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (string, error)
}

// DefaultSystemPrompt is the default system instruction given to the COVAS assistant.
const DefaultSystemPrompt = "You are an Elite Dangerous AI Cockpit Assistant (COVAS). " +
	"You have direct access to the ship's telemetry, navigation status, SQLite visited/targeted system history, in-game controls, and chronometer / time via MCP tools. " +
	"When the commander asks for status, navigation info, fuel, visited systems, current time, or commands a ship action (such as landing gear, lights, hardpoints, cargo scoop, night vision, boost, pips), " +
	"call the appropriate MCP tool to inspect or command the ship. Keep responses immersive, concise, and helpful like a ship computer."

// BuildTurnSystemPrompt combines the configured base system prompt with dynamic chronometer context.
func BuildTurnSystemPrompt(basePrompt string) string {
	if basePrompt == "" {
		basePrompt = DefaultSystemPrompt
	}
	now := time.Now().UTC()
	gameYear := now.Year() + 1286
	gameTime := fmt.Sprintf("%02d %s %04d, %02d:%02d:%02d UTC",
		now.Day(), now.Format("Jan"), gameYear, now.Hour(), now.Minute(), now.Second())
	return basePrompt + fmt.Sprintf("\nShip chronometer / Current UTC time: %s (%s).", now.Format(time.RFC3339), gameTime)
}

// SanitizeReply cleans up any unresolved placeholder tokens like {utc_time} in LLM responses.
func SanitizeReply(reply string) string {
	if strings.Contains(reply, "{utc_time}") || strings.Contains(reply, "{time}") {
		now := time.Now().UTC()
		gameYear := now.Year() + 1286
		gameTime := fmt.Sprintf("%02d:%02d:%02d UTC (%02d %s %04d)",
			now.Hour(), now.Minute(), now.Second(), now.Day(), now.Format("Jan"), gameYear)
		reply = strings.ReplaceAll(reply, "{utc_time}", gameTime)
		reply = strings.ReplaceAll(reply, "{time}", gameTime)
	}
	return reply
}

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
