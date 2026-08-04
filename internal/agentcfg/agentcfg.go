// Package agentcfg loads and validates an agent.json. The stored api_key_encrypted
// is WORKSPACE_KEY-encrypted (see internal/crypto); Load decrypts it into APIKey.
package agentcfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Cybrient-Technologies/agents-core/internal/crypto"
	"github.com/Cybrient-Technologies/agents-core/internal/mcp"
)

type ToolPolicy struct {
	Allowed         []string `json:"allowed"`
	Denied          []string `json:"denied"`
	RequireApproval []string `json:"require_approval"`
}

type Config struct {
	CompanyID             int          `json:"company_id"`
	APIKeyEncrypted       string       `json:"api_key_encrypted"`
	Model                 string       `json:"model"`
	AgentType             string       `json:"agent_type"` // manager|worker|standalone
	ToolPolicy            *ToolPolicy  `json:"tool_policy"`
	ConnectedIntegrations []string     `json:"connected_integrations"`
	MCPServers            []mcp.Server `json:"mcp_servers"`
	PromptCachingEnabled  bool         `json:"prompt_caching_enabled"`

	APIKey string `json:"-"` // decrypted at Load time
}

// Load reads {workspacesDir}/{slug}/agent.json, validates company ownership, and decrypts the key.
func Load(workspacesDir, slug string, companyID int, cipher *crypto.Cipher) (*Config, error) {
	path := filepath.Join(workspacesDir, slug, "agent.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("agent workspace not found")
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, errors.New("could not parse agent.json")
	}
	if companyID != 0 && c.CompanyID != companyID {
		return nil, errors.New("access denied: company mismatch")
	}
	if c.APIKeyEncrypted != "" && cipher != nil {
		key, err := cipher.Decrypt(c.APIKeyEncrypted)
		if err != nil {
			return nil, errors.New("could not decrypt agent api key")
		}
		c.APIKey = key
	}
	if c.AgentType == "" {
		c.AgentType = "standalone"
	}
	return &c, nil
}
