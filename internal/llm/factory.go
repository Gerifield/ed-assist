package llm

import (
	"fmt"
	"strings"
)

// ProviderConfig specifies the configuration needed to instantiate an LLM Client.
type ProviderConfig struct {
	Provider      string // "gemini" or "openai" (OpenAI-compatible)
	GeminiKey     string
	GeminiModel   string
	GeminiBaseURL string
	OpenAIKey     string
	OpenAIModel   string
	OpenAIBaseURL string
	SystemPrompt    string
	MaxToolRounds   int
	AutoTimeContext bool
	MCPCaller       MCPCaller
}

// NewClientFromConfig creates the appropriate LLM client based on ProviderConfig.
func NewClientFromConfig(cfg ProviderConfig) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "", "gemini":
		var opts []GeminiOption
		if cfg.GeminiBaseURL != "" {
			opts = append(opts, WithGeminiBaseURL(cfg.GeminiBaseURL))
		}
		if cfg.MCPCaller != nil {
			opts = append(opts, WithGeminiMCPCaller(cfg.MCPCaller))
		}
		if cfg.SystemPrompt != "" {
			opts = append(opts, WithGeminiSystemPrompt(cfg.SystemPrompt))
		}
		if cfg.MaxToolRounds > 0 {
			opts = append(opts, WithGeminiMaxRounds(cfg.MaxToolRounds))
		}
		opts = append(opts, WithGeminiAutoTime(cfg.AutoTimeContext))
		return NewGeminiClient(cfg.GeminiKey, cfg.GeminiModel, opts...), nil

	case "openai", "openai-compatible", "deepseek", "ollama", "groq", "lmstudio":
		var opts []OpenAIOption
		if cfg.OpenAIBaseURL != "" {
			opts = append(opts, WithOpenAIBaseURL(cfg.OpenAIBaseURL))
		}
		if cfg.MCPCaller != nil {
			opts = append(opts, WithOpenAIMCPCaller(cfg.MCPCaller))
		}
		if cfg.SystemPrompt != "" {
			opts = append(opts, WithOpenAISystemPrompt(cfg.SystemPrompt))
		}
		if cfg.MaxToolRounds > 0 {
			opts = append(opts, WithOpenAIMaxRounds(cfg.MaxToolRounds))
		}
		opts = append(opts, WithOpenAIAutoTime(cfg.AutoTimeContext))
		return NewOpenAIClient(cfg.OpenAIKey, cfg.OpenAIModel, opts...), nil

	default:
		return nil, fmt.Errorf("unsupported AI provider %q (supported: gemini, openai)", cfg.Provider)
	}
}
