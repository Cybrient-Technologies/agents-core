// Package llm is a multi-provider LLM router selected by API-key
// prefix first, then model prefix.
package llm

import "strings"

// Message is one conversation turn (MVP: text content only; multimodal later).
type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// Result is the normalized LLM response.
type Result struct {
	OK           bool   `json:"ok"`
	Text         string `json:"text,omitempty"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Error        string `json:"error,omitempty"`
}

const (
	epAnthropic  = "https://api.anthropic.com/v1/messages"
	epOpenAI     = "https://api.openai.com/v1/chat/completions"
	epMiniMax    = "https://api.minimax.io/v1/chat/completions"
	epOpenRouter = "https://openrouter.ai/api/v1/chat/completions"
	epBoldRouter = "https://api.boldrouter.com/v1/chat/completions"
)

type route struct {
	kind     string // "anthropic" | "openai" | "gemini" | "unknown"
	endpoint string
	model    string
}

// resolve picks the provider: gateway key prefix wins,
// otherwise dispatch by model prefix. Pure + network-free for testability.
func resolve(apiKey, model string) route {
	switch {
	case strings.HasPrefix(apiKey, "sk-or-v1-"):
		return route{"openai", epOpenRouter, orModelID(model)}
	case strings.HasPrefix(apiKey, "sk-bold-"):
		return route{"openai", epBoldRouter, boldModelID(model)}
	case strings.HasPrefix(model, "claude-"):
		return route{"anthropic", epAnthropic, model}
	case strings.HasPrefix(model, "gpt-"):
		return route{"openai", epOpenAI, model}
	case strings.HasPrefix(model, "gemini-"):
		return route{"gemini", "", model}
	case strings.HasPrefix(model, "MiniMax-"):
		return route{"openai", epMiniMax, model}
	default:
		return route{"unknown", "", model}
	}
}

// orModelID maps internal model ids to OpenRouter's provider/model form (hyphenated).
func orModelID(model string) string {
	if strings.HasPrefix(model, "openrouter/") {
		return model[len("openrouter/"):]
	}
	if v, ok := orMap[model]; ok {
		return v
	}
	return model
}

var orMap = map[string]string{
	"claude-opus-4-8":   "anthropic/claude-opus-4-8",
	"claude-sonnet-4-6": "anthropic/claude-sonnet-4-6",
	"claude-haiku-4-5":  "anthropic/claude-haiku-4-5",
	"gpt-4o":            "openai/gpt-4o",
	"gpt-4o-mini":       "openai/gpt-4o-mini",
	"gpt-4.1-mini":      "openai/gpt-4.1-mini",
	"gemini-2.5-flash":  "google/gemini-2.5-flash-preview-05-20",
	"gemini-3-flash":    "google/gemini-flash-1.5",
	"MiniMax-M2.1":      "minimax/minimax-01",
	"MiniMax-M2.5":      "minimax/minimax-m2",
}

// boldModelID maps internal model ids to BoldRouter's dotted provider/model form.
func boldModelID(model string) string {
	if strings.Contains(model, "/") {
		if strings.HasPrefix(model, "openrouter/") {
			return model[len("openrouter/"):]
		}
		return model
	}
	if v, ok := boldMap[model]; ok {
		return v
	}
	switch {
	case strings.HasPrefix(model, "claude-"):
		return "anthropic/" + model
	case strings.HasPrefix(model, "gpt-"):
		return "openai/" + model
	case strings.HasPrefix(model, "gemini-"):
		return "google/" + model
	case strings.HasPrefix(strings.ToLower(model), "minimax"):
		return "minimax/" + strings.ToLower(model)
	}
	return model
}

var boldMap = map[string]string{
	"claude-opus-4-8":   "anthropic/claude-opus-4.8",
	"claude-sonnet-4-6": "anthropic/claude-sonnet-4.6",
	"claude-haiku-4-5":  "anthropic/claude-haiku-4.5",
	"gpt-4o":            "openai/gpt-4o",
	"gpt-4o-mini":       "openai/gpt-4o-mini",
	"gpt-4.1-mini":      "openai/gpt-4o-mini",
	"gemini-2.5-flash":  "google/gemini-2.5-flash",
	"gemini-3-flash":    "google/gemini-3-flash",
	"MiniMax-M2.1":      "minimax/minimax-01",
	"MiniMax-M2.5":      "minimax/minimax-m2",
}
