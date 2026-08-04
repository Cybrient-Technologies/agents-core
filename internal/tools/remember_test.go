package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRememberAndLoad(t *testing.T) {
	ws := t.TempDir()
	r := NewRegistry()

	if LoadMemory(ws) != "" {
		t.Fatal("expected empty memory initially")
	}

	out := r.Execute(context.Background(), "remember",
		map[string]any{"content": "the runtime is a single static binary"}, nil, Env{WorkspaceDir: ws})
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	if res["ok"] != true {
		t.Fatalf("remember failed: %s", out)
	}
	_ = r.Execute(context.Background(), "remember",
		map[string]any{"content": "the runtime is written in Go"}, nil, Env{WorkspaceDir: ws})

	mem := LoadMemory(ws)
	if !strings.Contains(mem, "the runtime is a single static binary") || !strings.Contains(mem, "the runtime is written in Go") {
		t.Fatalf("memory not persisted/loaded: %q", mem)
	}
}
