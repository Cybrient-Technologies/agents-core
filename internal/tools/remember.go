package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
)

const memoryFile = "memory.md"

// remember appends a durable fact to the agent's workspace memory.
// The stored memory is injected into the system prompt on each run (see LoadMemory).
type remember struct{}

func (remember) Name() string { return "remember" }

func (remember) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "remember",
		Description: "Save a durable fact you should recall in future conversations (key facts, decisions, context).",
		InputSchema: schema(`{"type":"object","properties":{"content":{"type":"string"}},"required":["content"]}`),
	}
}

func (remember) Execute(_ context.Context, input map[string]any, env Env) map[string]any {
	if env.WorkspaceDir == "" {
		return map[string]any{"ok": false, "error": "no workspace configured"}
	}
	content := strings.TrimSpace(str(input["content"]))
	if content == "" {
		return map[string]any{"ok": false, "error": "content is required"}
	}
	path := filepath.Join(env.WorkspaceDir, memoryFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	defer f.Close()
	if _, err := f.WriteString("- " + content + "\n"); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "saved": content}
}

// LoadMemory returns the agent's stored memory (empty if none), for system-prompt injection.
func LoadMemory(workspaceDir string) string {
	if workspaceDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(workspaceDir, memoryFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
