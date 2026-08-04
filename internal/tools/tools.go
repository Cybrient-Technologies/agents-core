// Package tools provides the built-in tool registry + MCP tool integration:
// schema assembly and dispatch, including namespaced mcp__<server>__<tool> routing.
package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
	"github.com/Cybrient-Technologies/agents-core/internal/mcp"
)

// Env carries per-agent execution context (e.g. the workspace dir for file tools).
type Env struct {
	WorkspaceDir string
}

// Tool is a built-in runtime tool.
type Tool interface {
	Name() string
	Schema() llm.ToolSchema
	Execute(ctx context.Context, input map[string]any, env Env) map[string]any
}

// Registry holds the built-in tools.
type Registry struct {
	builtin map[string]Tool
}

// NewRegistry registers the built-in tools available in the OSS core (MVP: fetch_page).
func NewRegistry() *Registry {
	r := &Registry{builtin: map[string]Tool{}}
	r.register(&fetchPage{})
	r.register(&webSearch{})
	r.register(&httpRequest{})
	r.register(&saveFile{})
	r.register(&remember{})
	return r
}

func (r *Registry) register(t Tool) { r.builtin[t.Name()] = t }

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// SchemasFor builds the tool list advertised to the model: built-ins + MCP tools
// (namespaced mcp__<slug>__<tool>), filtered by the agent's tool_policy allow/deny.
func (r *Registry) SchemasFor(allowed, denied []string, mcpServers []mcp.Server) []llm.ToolSchema {
	permit := func(name string) bool {
		if allowed != nil && !contains(allowed, name) {
			return false
		}
		return !contains(denied, name)
	}
	var out []llm.ToolSchema
	for name, t := range r.builtin {
		if permit(name) {
			out = append(out, t.Schema())
		}
	}
	for _, s := range mcpServers {
		for _, tl := range s.Tools {
			q := "mcp__" + s.Slug + "__" + tl.Name
			if !permit(q) {
				continue
			}
			out = append(out, llm.ToolSchema{
				Name:        q,
				Description: "[" + s.Name + " via MCP] " + tl.Description,
				InputSchema: tl.InputSchema,
			})
		}
	}
	return out
}

// Execute dispatches a tool call and returns the tool_result content (a JSON string).
func (r *Registry) Execute(ctx context.Context, name string, input map[string]any, mcpServers []mcp.Server, env Env) string {
	if strings.HasPrefix(name, "mcp__") {
		rest := name[len("mcp__"):]
		i := strings.Index(rest, "__")
		if i < 0 {
			return jsonStr(map[string]any{"ok": false, "error": "malformed MCP tool name"})
		}
		slug, tool := rest[:i], rest[i+2:]
		for _, s := range mcpServers {
			if s.Slug == slug {
				res := s.CallTool(ctx, tool, input)
				if res.OK {
					return jsonStr(map[string]any{"ok": true, "text": res.Text})
				}
				return jsonStr(map[string]any{"ok": false, "error": res.Error})
			}
		}
		return jsonStr(map[string]any{"ok": false, "error": "MCP server '" + slug + "' is not connected to this agent."})
	}
	if t, ok := r.builtin[name]; ok {
		return jsonStr(t.Execute(ctx, input, env))
	}
	return jsonStr(map[string]any{"ok": false, "error": "Unknown tool: " + name})
}

func jsonStr(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func schema(props string) json.RawMessage {
	return json.RawMessage(props)
}

// RE2 has no backreferences, so strip each container tag explicitly.
var stripRes = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
	regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
	regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`),
}
var anyTagRe = regexp.MustCompile(`<[^>]*>`)
var wsRe = regexp.MustCompile(`[ \t]+`)
var nlRe = regexp.MustCompile(`(\n\s*){3,}`)
