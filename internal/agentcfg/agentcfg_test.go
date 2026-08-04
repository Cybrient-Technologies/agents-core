package agentcfg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Cybrient-Technologies/agents-core/internal/crypto"
)

const keyHex = "abababababababab" + "abababababababab" + "abababababababab" + "abababababababab"

func TestLoad(t *testing.T) {
	c, err := crypto.New(keyHex)
	if err != nil {
		t.Fatal(err)
	}
	encKey, err := c.Encrypt("sk-ant-secret")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	ws := filepath.Join(dir, "echo-node")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	agentJSON := `{
		"company_id": 4,
		"api_key_encrypted": "` + encKey + `",
		"model": "claude-sonnet-4-6",
		"agent_type": "worker",
		"tool_policy": {"denied": ["run_code"]},
		"mcp_servers": [
			{"slug":"deepwiki","name":"DeepWiki","url":"https://mcp.deepwiki.com/mcp","auth_type":"none",
			 "tools":[{"name":"ask","description":"ask","input_schema":{"type":"object"}}]}
		]
	}`
	if err := os.WriteFile(filepath.Join(ws, "agent.json"), []byte(agentJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir, "echo-node", 4, c)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "sk-ant-secret" {
		t.Fatalf("api key not decrypted: %q", cfg.APIKey)
	}
	if cfg.Model != "claude-sonnet-4-6" || cfg.AgentType != "worker" {
		t.Fatalf("fields: %+v", cfg)
	}
	if cfg.ToolPolicy == nil || len(cfg.ToolPolicy.Denied) != 1 || cfg.ToolPolicy.Denied[0] != "run_code" {
		t.Fatalf("tool_policy: %+v", cfg.ToolPolicy)
	}
	if len(cfg.MCPServers) != 1 || cfg.MCPServers[0].Slug != "deepwiki" || len(cfg.MCPServers[0].Tools) != 1 {
		t.Fatalf("mcp_servers: %+v", cfg.MCPServers)
	}

	// wrong company → error
	if _, err := Load(dir, "echo-node", 999, c); err == nil {
		t.Fatal("expected company mismatch error")
	}
}
