// Package agent implements the multi-round tool-calling loop (callLLMWithTools parity).
// The LLM round and tool execution are injected, so the loop is provider-agnostic and testable.
package agent

import (
	"context"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
)

// Round runs one LLM turn with the current provider-neutral transcript + tool schemas.
type Round func(ctx context.Context, steps []llm.Step, tools []llm.ToolSchema) llm.RoundResult

// Exec runs a tool by name and returns the tool_result content (a JSON string).
type Exec func(ctx context.Context, name string, input map[string]any) string

// Run drives the loop: LLM -> tool_calls -> tool results -> LLM, until the model answers
// or maxRounds is hit. Tokens are summed across rounds.
func Run(ctx context.Context, round Round, exec Exec, task string, tools []llm.ToolSchema, maxRounds int) llm.Result {
	if maxRounds <= 0 {
		maxRounds = 6
	}
	steps := []llm.Step{{Role: "user", Text: task}}
	var inTok, outTok int

	for i := 0; i < maxRounds; i++ {
		rr := round(ctx, steps, tools)
		if !rr.OK {
			return llm.Result{Error: rr.Error}
		}
		inTok += rr.InputTokens
		outTok += rr.OutputTokens

		if len(rr.ToolCalls) == 0 {
			return llm.Result{OK: true, Text: rr.Text, InputTokens: inTok, OutputTokens: outTok}
		}

		steps = append(steps, llm.Step{Role: "assistant", Text: rr.Text, ToolCalls: rr.ToolCalls})
		results := make([]llm.ToolResult, 0, len(rr.ToolCalls))
		for _, tc := range rr.ToolCalls {
			out := exec(ctx, tc.Name, tc.Input)
			results = append(results, llm.ToolResult{ID: tc.ID, Name: tc.Name, Content: out})
		}
		steps = append(steps, llm.Step{Role: "tool", ToolResults: results})
	}
	return llm.Result{
		OK: true, Text: "Reached the maximum reasoning rounds; returning partial results.",
		InputTokens: inTok, OutputTokens: outTok,
	}
}
