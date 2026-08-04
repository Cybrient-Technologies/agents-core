// Package config loads runtime configuration from the environment,
// for a clean single-binary deploy.
package config

import (
	"errors"
	"os"
)

type Config struct {
	// Wire auth (must match the SaaS side exactly)
	Token      string // AGENTCORE_TOKEN
	SigningKey string // AGENTCORE_SIGNING_KEY (HMAC-SHA256 of "<ts>.<body>")

	// Local crypto — hex-encoded 32-byte AES-256-GCM key (WORKSPACE_KEY)
	WorkspaceKeyHex string

	// Storage
	WorkspacesDir string // per-agent workspaces (agent.json lives here)
	DSN           string // MySQL DSN for the runtime DB (optional in MVP)

	// Server
	Addr string // listen address, e.g. 127.0.0.1:3010

	// Channels (OSS)
	TelegramToken string // TELEGRAM_BOT_TOKEN — the OSS Telegram channel

	// When true, require X-Forwarded-Proto: https (behind Nginx TLS). Off for local dev.
	RequireHTTPS bool
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from the environment and validates the required fields.
func Load() (*Config, error) {
	c := &Config{
		Token:           os.Getenv("AGENTCORE_TOKEN"),
		SigningKey:      os.Getenv("AGENTCORE_SIGNING_KEY"),
		WorkspaceKeyHex: os.Getenv("WORKSPACE_KEY"),
		WorkspacesDir:   env("WORKSPACES_DIR", "/opt/agentcore/workspaces"),
		DSN:             os.Getenv("AGENTCORE_DSN"),
		Addr:            env("AGENTCORE_ADDR", "127.0.0.1:3010"),
		RequireHTTPS:    env("AGENTCORE_REQUIRE_HTTPS", "true") != "false",
		TelegramToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
	}
	if c.Token == "" {
		return nil, errors.New("AGENTCORE_TOKEN is required")
	}
	// SigningKey may be empty only if the caller also has it empty (signature step skipped).
	return c, nil
}
