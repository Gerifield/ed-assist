package llm

import (
	"strings"
	"testing"
)

func TestBuildTurnSystemPrompt(t *testing.T) {
	prompt := BuildTurnSystemPrompt("Base prompt")
	if !strings.Contains(prompt, "Base prompt") {
		t.Errorf("expected prompt to contain base prompt, got %s", prompt)
	}
	if !strings.Contains(prompt, "Ship chronometer") {
		t.Errorf("expected prompt to contain chronometer context, got %s", prompt)
	}

	emptyPrompt := BuildTurnSystemPrompt("")
	if !strings.Contains(emptyPrompt, DefaultSystemPrompt) {
		t.Errorf("expected prompt to contain DefaultSystemPrompt, got %s", emptyPrompt)
	}
}

func TestSanitizeReply(t *testing.T) {
	raw := "Commander, local ship chronometer is synchronized to Universal Time (UTC).\nCurrent Time: {utc_time} (System clock active)."
	sanitized := SanitizeReply(raw)
	if strings.Contains(sanitized, "{utc_time}") {
		t.Errorf("expected {utc_time} to be replaced, got %s", sanitized)
	}
	if !strings.Contains(sanitized, "UTC") {
		t.Errorf("expected sanitized string to contain UTC time, got %s", sanitized)
	}

	// Normal reply without placeholders untouched
	normal := "Landing gear deployed."
	if SanitizeReply(normal) != normal {
		t.Errorf("expected normal text unchanged, got %s", SanitizeReply(normal))
	}
}

func TestAutoTimeOption(t *testing.T) {
	gemini := NewGeminiClient("test-key", "gemini-flash", WithGeminiAutoTime(false))
	if gemini.autoTime {
		t.Errorf("expected gemini autoTime to be false")
	}

	openai := NewOpenAIClient("test-key", "gpt-4o", WithOpenAIAutoTime(false))
	if openai.autoTime {
		t.Errorf("expected openai autoTime to be false")
	}
}

