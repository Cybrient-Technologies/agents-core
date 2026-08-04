package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
)

// serperURL is a var so tests can point it at a fake server.
var serperURL = "https://google.serper.dev/search"

var searchClient = &http.Client{Timeout: 12 * time.Second}

// webSearch queries Serper for web results (keyless DDG fallback is TODO).
type webSearch struct{}

func (webSearch) Name() string { return "web_search" }

func (webSearch) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "web_search",
		Description: "Search the web for current information (news, prices, companies, people, events). Returns titles, snippets and URLs.",
		InputSchema: schema(`{"type":"object","properties":{"query":{"type":"string"},"max_results":{"type":"integer"}},"required":["query"]}`),
	}
}

func (webSearch) Execute(ctx context.Context, input map[string]any, _ Env) map[string]any {
	query, _ := input["query"].(string)
	if query == "" {
		return map[string]any{"ok": false, "error": "query is required"}
	}
	num := 5
	if f, ok := input["max_results"].(float64); ok && f > 0 {
		num = int(f)
	}
	if num > 8 {
		num = 8
	}
	key := serperKey()
	if key == "" {
		return map[string]any{"ok": false, "error": "web search not configured (no SERPER_API_KEY)"}
	}
	payload, _ := json.Marshal(map[string]any{"q": query, "num": num})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, serperURL, bytes.NewReader(payload))
	req.Header.Set("X-API-KEY", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := searchClient.Do(req)
	if err != nil {
		return map[string]any{"ok": false, "error": "serper request failed: " + err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return map[string]any{"ok": false, "error": "serper HTTP " + http.StatusText(resp.StatusCode)}
	}
	var d struct {
		Organic []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
			Link    string `json:"link"`
		} `json:"organic"`
	}
	_ = json.Unmarshal(body, &d)
	results := make([]map[string]any, 0, len(d.Organic))
	for i, r := range d.Organic {
		if i >= num {
			break
		}
		results = append(results, map[string]any{"title": r.Title, "snippet": r.Snippet, "url": r.Link})
	}
	return map[string]any{"ok": true, "query": query, "results": results}
}

func serperKey() string {
	// TODO: support a platform-keys file; env for now.
	return os.Getenv("SERPER_API_KEY")
}
