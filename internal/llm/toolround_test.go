package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAIToolRound proves the OpenAI-compatible tool-calling serialize/parse round-trip:
// round 1 (no tool results) -> the model returns a tool_call; round 2 (with a tool result)
// -> the model returns a final answer.
func TestOpenAIToolRound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		hasToolResult := false
		for _, m := range req.Messages {
			if m.Role == "tool" {
				hasToolResult = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if hasToolResult {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"the answer is 42"}}],"usage":{"prompt_tokens":7,"completion_tokens":4}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"meaning of life\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":6}}`))
	}))
	defer ts.Close()

	tools := []ToolSchema{{Name: "web_search", Description: "search", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	steps := []Step{{Role: "user", Text: "what is the meaning of life?"}}

	rr := openaiToolRound(context.Background(), "sk-x", ts.URL, "gpt-4o", "sys", steps, tools, 100)
	if !rr.OK || len(rr.ToolCalls) != 1 {
		t.Fatalf("round 1: %+v", rr)
	}
	tc := rr.ToolCalls[0]
	if tc.Name != "web_search" || tc.ID != "c1" || tc.Input["query"] != "meaning of life" {
		t.Fatalf("tool call not parsed: %+v", tc)
	}

	steps = append(steps,
		Step{Role: "assistant", ToolCalls: rr.ToolCalls},
		Step{Role: "tool", ToolResults: []ToolResult{{ID: "c1", Name: "web_search", Content: `{"ok":true}`}}},
	)
	rr2 := openaiToolRound(context.Background(), "sk-x", ts.URL, "gpt-4o", "sys", steps, tools, 100)
	if !rr2.OK || rr2.Text != "the answer is 42" || len(rr2.ToolCalls) != 0 {
		t.Fatalf("round 2: %+v", rr2)
	}
	if rr2.InputTokens != 7 || rr2.OutputTokens != 4 {
		t.Fatalf("tokens: %+v", rr2)
	}
}
