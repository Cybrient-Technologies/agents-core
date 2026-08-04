package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

// Call routes to the right provider and returns a normalized Result (never panics;
// transport/API errors come back as {OK:false, Error}).
func Call(ctx context.Context, apiKey, model, system string, messages []Message, maxTokens int) Result {
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	r := resolve(apiKey, model)
	switch r.kind {
	case "anthropic":
		return callAnthropic(ctx, apiKey, r.model, system, messages, maxTokens)
	case "openai":
		return callOpenAICompat(ctx, apiKey, r.endpoint, r.model, system, messages, maxTokens)
	case "gemini":
		// TODO: Gemini uses a different request shape.
		return Result{Error: "gemini not yet implemented in the Go runtime"}
	default:
		return Result{Error: "Unknown model: " + model}
	}
}

func doJSON(ctx context.Context, url string, headers map[string]string, payload any) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return out, resp.StatusCode, err
}

// ── Anthropic (Claude) — /v1/messages ────────────────────────────────────────
func callAnthropic(ctx context.Context, apiKey, model, system string, messages []Message, maxTokens int) Result {
	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   messages,
	}
	raw, code, err := doJSON(ctx, epAnthropic, map[string]string{
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
	}, payload)
	if err != nil {
		return Result{Error: "Anthropic transport error: " + err.Error()}
	}
	var d struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &d)
	if code != http.StatusOK || len(d.Content) == 0 || d.Content[0].Text == "" {
		return Result{Error: "Anthropic API error: " + firstNonEmpty(d.Error.Message, fmt.Sprintf("HTTP %d", code))}
	}
	return Result{
		OK: true, Text: d.Content[0].Text, Model: model,
		InputTokens: d.Usage.InputTokens, OutputTokens: d.Usage.OutputTokens,
	}
}

// ── OpenAI-compatible (OpenAI, MiniMax, OpenRouter, BoldRouter) ───────────────
func callOpenAICompat(ctx context.Context, apiKey, endpoint, model, system string, messages []Message, maxTokens int) Result {
	msgs := make([]Message, 0, len(messages)+1)
	msgs = append(msgs, Message{Role: "system", Content: system})
	msgs = append(msgs, messages...)
	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   msgs,
	}
	raw, code, err := doJSON(ctx, endpoint, map[string]string{
		"Authorization": "Bearer " + apiKey,
	}, payload)
	if err != nil {
		return Result{Error: "LLM transport error: " + err.Error()}
	}
	var d struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &d)
	if code != http.StatusOK || len(d.Choices) == 0 || d.Choices[0].Message.Content == "" {
		return Result{Error: "LLM API error: " + firstNonEmpty(d.Error.Message, fmt.Sprintf("HTTP %d", code))}
	}
	return Result{
		OK: true, Text: d.Choices[0].Message.Content, Model: model,
		InputTokens: d.Usage.PromptTokens, OutputTokens: d.Usage.CompletionTokens,
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
