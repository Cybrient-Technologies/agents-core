// Command agentcore is the open-source Go agent runtime.
//
// Two modes:
//   - server (default): serves the HMAC-signed runtime API.
//   - -cli: run one agent task from the command line and exit (the OSS "try it" path).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Cybrient-Technologies/agents-core/internal/agent"
	"github.com/Cybrient-Technologies/agents-core/internal/agentcfg"
	"github.com/Cybrient-Technologies/agents-core/internal/channel/telegram"
	"github.com/Cybrient-Technologies/agents-core/internal/config"
	"github.com/Cybrient-Technologies/agents-core/internal/crypto"
	"github.com/Cybrient-Technologies/agents-core/internal/llm"
	"github.com/Cybrient-Technologies/agents-core/internal/tools"
	"github.com/Cybrient-Technologies/agents-core/internal/wire"
)

const runtimeVersion = "0.2.0-mvp"

const systemPrompt = "You are a capable AI agent. Use the available tools when they help, then give a clear final answer."

// runAgent loads an agent, assembles its tools (built-in + MCP), runs the tool-calling loop.
func runAgent(ctx context.Context, cfg *config.Config, cipher *crypto.Cipher, reg *tools.Registry, slug string, companyID int, task string) (map[string]any, error) {
	ac, err := agentcfg.Load(cfg.WorkspacesDir, slug, companyID, cipher)
	if err != nil {
		return nil, err
	}
	if ac.APIKey == "" {
		return nil, errors.New("no API key configured for this agent")
	}
	var allowed, denied []string
	if ac.ToolPolicy != nil {
		allowed, denied = ac.ToolPolicy.Allowed, ac.ToolPolicy.Denied
	}
	schemas := reg.SchemasFor(allowed, denied, ac.MCPServers)
	env := tools.Env{WorkspaceDir: filepath.Join(cfg.WorkspacesDir, slug)}

	system := systemPrompt
	if mem := tools.LoadMemory(env.WorkspaceDir); mem != "" {
		system += "\n\nWhat you remember:\n" + mem
	}
	round := func(ctx context.Context, steps []llm.Step, ts []llm.ToolSchema) llm.RoundResult {
		return llm.ToolRound(ctx, ac.APIKey, ac.Model, system, steps, ts, 2048)
	}
	exec := func(ctx context.Context, name string, input map[string]any) string {
		return reg.Execute(ctx, name, input, ac.MCPServers, env)
	}
	res := agent.Run(ctx, round, exec, task, schemas, 6)
	if !res.OK {
		return nil, errors.New(res.Error)
	}
	return map[string]any{
		"text": res.Text, "model": ac.Model,
		"input_tokens": res.InputTokens, "output_tokens": res.OutputTokens,
	}, nil
}

func main() {
	cliMode := flag.Bool("cli", false, "run one agent task from the CLI and exit")
	tgMode := flag.Bool("telegram", false, "run the Telegram channel (long-poll) for -slug")
	cliSlug := flag.String("slug", "", "agent slug (cli/telegram mode)")
	cliTask := flag.String("task", "", "task text (cli mode)")
	cliCompany := flag.Int("company", 0, "company id (0 skips the ownership check)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	var cipher *crypto.Cipher
	if cfg.WorkspaceKeyHex != "" {
		if cipher, err = crypto.New(cfg.WorkspaceKeyHex); err != nil {
			log.Fatalf("crypto: %v", err)
		}
	}
	registry := tools.NewRegistry()

	// ── CLI mode ──
	if *cliMode {
		if *cliSlug == "" || *cliTask == "" {
			log.Fatal("cli mode requires -slug and -task")
		}
		out, err := runAgent(context.Background(), cfg, cipher, registry, *cliSlug, *cliCompany, *cliTask)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	// ── Telegram channel mode (OSS) ──
	if *tgMode {
		if cfg.TelegramToken == "" || *cliSlug == "" {
			log.Fatal("telegram mode requires TELEGRAM_BOT_TOKEN and -slug")
		}
		bot := telegram.New(cfg.TelegramToken)
		run := func(ctx context.Context, chatID int64, text string) string {
			out, err := runAgent(ctx, cfg, cipher, registry, *cliSlug, *cliCompany, text)
			if err != nil {
				return "⚠️ " + err.Error()
			}
			if s, ok := out["text"].(string); ok && s != "" {
				return s
			}
			return "(no answer)"
		}
		log.Printf("agentcore-go %s — Telegram channel for %q", runtimeVersion, *cliSlug)
		if err := bot.Poll(context.Background(), run); err != nil {
			log.Fatalf("telegram: %v", err)
		}
		return
	}

	// ── Server mode ──
	srv := wire.NewServer(cfg.Token, cfg.SigningKey, cfg.RequireHTTPS)

	srv.Handle("/api/status", func(c *wire.Ctx) (map[string]any, *wire.HTTPError) {
		if c.Int("company_id") == 0 {
			return nil, wire.Errf(http.StatusUnprocessableEntity, "company_id is required.")
		}
		return map[string]any{"runtime": "agentcore-go", "version": runtimeVersion, "status": "healthy"}, nil
	})

	srv.Handle("/api/run", func(c *wire.Ctx) (map[string]any, *wire.HTTPError) {
		slug, companyID, task := c.Str("slug"), c.Int("company_id"), c.Str("task")
		if slug == "" || companyID == 0 || task == "" {
			return nil, wire.Errf(http.StatusUnprocessableEntity, "slug, company_id and task are required.")
		}
		out, err := runAgent(c.R.Context(), cfg, cipher, registry, slug, companyID, task)
		if err != nil {
			return nil, wire.Errf(http.StatusBadGateway, err.Error())
		}
		return out, nil
	})

	httpSrv := &http.Server{Addr: cfg.Addr, Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("agentcore-go %s listening on %s (requireHTTPS=%v)", runtimeVersion, cfg.Addr, cfg.RequireHTTPS)
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
