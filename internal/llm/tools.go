package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ToolSchema is a tool advertised to the model (input_schema is a raw JSON Schema).
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ── Provider-neutral transcript ──────────────────────────────────────────────
// The agent loop works in these terms; each provider's *ToolRound translates to/from
// its own wire format. This keeps the loop provider-agnostic.

type ToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

type ToolResult struct {
	ID      string
	Name    string
	Content string // JSON string from executeTool
}

type Step struct {
	Role        string       // "user" | "assistant" | "tool"
	Text        string       // user prompt, or assistant text
	ToolCalls   []ToolCall   // assistant's tool requests
	ToolResults []ToolResult // when Role=="tool"
}

// RoundResult is one round of the tool-calling loop.
type RoundResult struct {
	OK           bool
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
	Error        string
}

// ToolRound runs one tool-calling round, dispatching by provider (key prefix then model),
// exactly like plain Call. Gemini tool-calling is not yet ported.
func ToolRound(ctx context.Context, apiKey, model, system string, steps []Step, tools []ToolSchema, maxTokens int) RoundResult {
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	r := resolve(apiKey, model)
	switch r.kind {
	case "anthropic":
		return anthropicToolRound(ctx, apiKey, r.model, system, steps, tools, maxTokens)
	case "openai":
		return openaiToolRound(ctx, apiKey, r.endpoint, r.model, system, steps, tools, maxTokens)
	case "gemini":
		return RoundResult{Error: "gemini tool-calling not yet implemented in the Go runtime"}
	default:
		return RoundResult{Error: "Unknown model: " + model}
	}
}

// ── Anthropic (Messages API, tool_use / tool_result blocks) ───────────────────
func anthropicToolRound(ctx context.Context, apiKey, model, system string, steps []Step, tools []ToolSchema, maxTokens int) RoundResult {
	messages := make([]map[string]any, 0, len(steps))
	for _, s := range steps {
		switch s.Role {
		case "user":
			messages = append(messages, map[string]any{"role": "user", "content": s.Text})
		case "assistant":
			var blocks []map[string]any
			if s.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": s.Text})
			}
			for _, tc := range s.ToolCalls {
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": tc.Input})
			}
			messages = append(messages, map[string]any{"role": "assistant", "content": blocks})
		case "tool":
			var blocks []map[string]any
			for _, tr := range s.ToolResults {
				blocks = append(blocks, map[string]any{"type": "tool_result", "tool_use_id": tr.ID, "content": tr.Content})
			}
			messages = append(messages, map[string]any{"role": "user", "content": blocks})
		}
	}
	toolDefs := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		toolDefs = append(toolDefs, map[string]any{"name": t.Name, "description": t.Description, "input_schema": schema})
	}
	payload := map[string]any{"model": model, "max_tokens": maxTokens, "system": system, "messages": messages}
	if len(toolDefs) > 0 {
		payload["tools"] = toolDefs
	}
	raw, code, err := doJSON(ctx, epAnthropic, map[string]string{
		"x-api-key": apiKey, "anthropic-version": "2023-06-01",
	}, payload)
	if err != nil {
		return RoundResult{Error: "Anthropic transport error: " + err.Error()}
	}
	var d struct {
		Content []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
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
	if code != http.StatusOK || len(d.Content) == 0 {
		return RoundResult{Error: "Anthropic API error: " + firstNonEmpty(d.Error.Message, fmt.Sprintf("HTTP %d", code))}
	}
	res := RoundResult{OK: true, InputTokens: d.Usage.InputTokens, OutputTokens: d.Usage.OutputTokens}
	for _, b := range d.Content {
		switch b.Type {
		case "text":
			res.Text += b.Text
		case "tool_use":
			res.ToolCalls = append(res.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	return res
}

// ── OpenAI-compatible (chat/completions, tool_calls / tool role) ──────────────
func openaiToolRound(ctx context.Context, apiKey, endpoint, model, system string, steps []Step, tools []ToolSchema, maxTokens int) RoundResult {
	messages := []map[string]any{{"role": "system", "content": system}}
	for _, s := range steps {
		switch s.Role {
		case "user":
			messages = append(messages, map[string]any{"role": "user", "content": s.Text})
		case "assistant":
			m := map[string]any{"role": "assistant"}
			if s.Text != "" {
				m["content"] = s.Text
			}
			if len(s.ToolCalls) > 0 {
				calls := make([]map[string]any, 0, len(s.ToolCalls))
				for _, tc := range s.ToolCalls {
					args, _ := json.Marshal(tc.Input)
					calls = append(calls, map[string]any{
						"id": tc.ID, "type": "function",
						"function": map[string]any{"name": tc.Name, "arguments": string(args)},
					})
				}
				m["tool_calls"] = calls
			}
			messages = append(messages, m)
		case "tool":
			for _, tr := range s.ToolResults {
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": tr.ID, "content": tr.Content})
			}
		}
	}
	toolDefs := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		toolDefs = append(toolDefs, map[string]any{
			"type":     "function",
			"function": map[string]any{"name": t.Name, "description": t.Description, "parameters": params},
		})
	}
	payload := map[string]any{"model": model, "max_tokens": maxTokens, "messages": messages}
	if len(toolDefs) > 0 {
		payload["tools"] = toolDefs
	}
	raw, code, err := doJSON(ctx, endpoint, map[string]string{"Authorization": "Bearer " + apiKey}, payload)
	if err != nil {
		return RoundResult{Error: "LLM transport error: " + err.Error()}
	}
	var d struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
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
	if code != http.StatusOK || len(d.Choices) == 0 {
		return RoundResult{Error: "LLM API error: " + firstNonEmpty(d.Error.Message, fmt.Sprintf("HTTP %d", code))}
	}
	msg := d.Choices[0].Message
	res := RoundResult{OK: true, Text: msg.Content, InputTokens: d.Usage.PromptTokens, OutputTokens: d.Usage.CompletionTokens}
	for _, tc := range msg.ToolCalls {
		input := map[string]any{}
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		res.ToolCalls = append(res.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Input: input})
	}
	return res
}
