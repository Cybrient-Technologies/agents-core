package agent

import (
	"context"
	"testing"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
)

// TestLoopExecutesToolThenAnswers proves round->tool_call->tool_result->answer without network.
func TestLoopExecutesToolThenAnswers(t *testing.T) {
	var round int
	fakeRound := func(ctx context.Context, steps []llm.Step, tools []llm.ToolSchema) llm.RoundResult {
		round++
		if round == 1 {
			if len(tools) == 0 {
				t.Fatalf("expected tools to be advertised")
			}
			return llm.RoundResult{
				OK: true, InputTokens: 10, OutputTokens: 5,
				ToolCalls: []llm.ToolCall{{ID: "t1", Name: "mcp__fake__ask", Input: map[string]any{"q": "hi"}}},
			}
		}
		// round 2: transcript should be user, assistant(tool_call), tool(result)
		if len(steps) != 3 || steps[1].Role != "assistant" || steps[2].Role != "tool" {
			t.Fatalf("bad transcript by round 2: %+v", steps)
		}
		if len(steps[2].ToolResults) != 1 || steps[2].ToolResults[0].ID != "t1" {
			t.Fatalf("tool result not threaded: %+v", steps[2])
		}
		return llm.RoundResult{OK: true, Text: "the answer is 42", InputTokens: 8, OutputTokens: 3}
	}

	var execCalls int
	fakeExec := func(ctx context.Context, name string, input map[string]any) string {
		execCalls++
		if name != "mcp__fake__ask" {
			t.Fatalf("unexpected tool %q", name)
		}
		return `{"ok":true,"text":"42"}`
	}

	tools := []llm.ToolSchema{{Name: "mcp__fake__ask", Description: "x"}}
	res := Run(context.Background(), fakeRound, fakeExec, "what is the answer?", tools, 6)

	if !res.OK || res.Text != "the answer is 42" {
		t.Fatalf("result = %+v", res)
	}
	if execCalls != 1 {
		t.Fatalf("expected 1 tool exec, got %d", execCalls)
	}
	if res.InputTokens != 18 || res.OutputTokens != 8 {
		t.Fatalf("tokens not summed: %+v", res)
	}
}
