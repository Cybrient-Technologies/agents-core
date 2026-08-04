# agents-core

The open-source **Go agent runtime** behind [agents.diy](https://agents.diy) — a single
static binary that runs an AI agent: multi-provider LLM calls, a built-in tool set, MCP tool
servers, and a Telegram channel. No PHP, no per-agent services, no runtime dependencies —
one binary you can run anywhere.

## Features

- **Multi-provider LLM** — Anthropic and OpenAI-compatible endpoints (Claude, GPT, MiniMax,
  OpenRouter, BoldRouter), selected automatically by API-key prefix.
- **Tool-calling loop** — the agent calls tools, reads results, and iterates to an answer.
- **Built-in tools** — `fetch_page`, `web_search` (Serper), `http_request`, `save_file`
  (workspace-sandboxed), and `remember` (durable memory).
- **Memory** — `remember` persists facts to the agent's workspace and injects them into the
  system prompt on every run.
- **MCP** — connect external [Model Context Protocol](https://modelcontextprotocol.io) tool
  servers; their tools are namespaced `mcp__<server>__<tool>` and offered to the model.
- **Telegram channel** — talk to your agent over Telegram with no public URL (long-poll).
- **CLI mode** — run a one-off task and print the result.
- **Encrypted keys** — per-agent API keys are stored AES-256-GCM encrypted at rest.

## Install

```bash
go install github.com/Cybrient-Technologies/agents-core/cmd/agentcore@latest
```

Or build from source:

```bash
git clone https://github.com/Cybrient-Technologies/agents-core
cd agents-core
go build ./cmd/agentcore
```

## Run

### CLI — one task

```bash
WORKSPACE_KEY=<64-hex> \
agentcore -cli -slug my-agent -task "Summarise the latest Go release notes"
```

### Telegram channel

```bash
TELEGRAM_BOT_TOKEN=<token> \
WORKSPACE_KEY=<64-hex> \
agentcore -telegram -slug my-agent
```

### Server

```bash
AGENTCORE_TOKEN=<shared-token> \
AGENTCORE_SIGNING_KEY=<hmac-key> \
WORKSPACE_KEY=<64-hex> \
agentcore
```

Exposes `POST /api/status` and `POST /api/run` behind an HMAC-signed request contract.

## Configuration

| Variable | Purpose |
| --- | --- |
| `WORKSPACE_KEY` | 64-hex AES-256 key used to decrypt stored agent API keys |
| `WORKSPACES_DIR` | where per-agent workspaces live (default `/opt/agentcore/workspaces`) |
| `AGENTCORE_TOKEN` | shared bearer token for server mode |
| `AGENTCORE_SIGNING_KEY` | HMAC-SHA256 signing key for server-mode requests |
| `SERPER_API_KEY` | enables the `web_search` tool |
| `TELEGRAM_BOT_TOKEN` | enables the Telegram channel |

An agent is a directory under `WORKSPACES_DIR/<slug>` containing an `agent.json`
(model, encrypted API key, tool policy, MCP servers) plus the agent's working files.

## Build & test

```bash
go build ./...
go test ./...
```

## License

[FSL-1.1-ALv2](LICENSE.md) — the Functional Source License. Free to use, modify, and
redistribute for any purpose except building a competing product; converts to Apache 2.0
two years after each release. © 2026 Cybrient Technologies SA.
