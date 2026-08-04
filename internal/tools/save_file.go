package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
)

// saveFile writes content to a file inside the agent's workspace.
type saveFile struct{}

func (saveFile) Name() string { return "save_file" }

func (saveFile) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "save_file",
		Description: "Save text content to a file in your workspace. Use a relative path like notes.md or out/report.txt.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
	}
}

func (saveFile) Execute(_ context.Context, input map[string]any, env Env) map[string]any {
	if env.WorkspaceDir == "" {
		return map[string]any{"ok": false, "error": "no workspace configured"}
	}
	rel := strings.TrimSpace(str(input["path"]))
	if rel == "" {
		return map[string]any{"ok": false, "error": "path is required"}
	}
	content := str(input["content"])

	// Force relative + clean, then confirm the result stays inside the workspace.
	clean := filepath.Clean("/" + strings.ReplaceAll(rel, "\\", "/"))
	full := filepath.Join(env.WorkspaceDir, clean)
	base, err1 := filepath.Abs(env.WorkspaceDir)
	abs, err2 := filepath.Abs(full)
	if err1 != nil || err2 != nil || (abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator))) {
		return map[string]any{"ok": false, "error": "invalid path (must stay within the workspace)"}
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "path": rel, "bytes": len(content)}
}
