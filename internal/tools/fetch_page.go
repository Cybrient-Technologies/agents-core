package tools

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
)

var fetchClient = &http.Client{Timeout: 15 * time.Second}

// fetchPage fetches a URL and returns its readable text.
type fetchPage struct{}

func (fetchPage) Name() string { return "fetch_page" }

func (fetchPage) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "fetch_page",
		Description: "Fetch and read the full text of any public URL. Use after web_search or when given a direct link.",
		InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string","description":"The https URL to fetch"}},"required":["url"]}`),
	}
}

func (fetchPage) Execute(ctx context.Context, input map[string]any, _ Env) map[string]any {
	url, _ := input["url"].(string)
	url = strings.TrimSpace(url)
	if url == "" || !strings.HasPrefix(url, "https://") {
		return map[string]any{"ok": false, "error": "a valid https:// url is required"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; agents.diy/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := fetchClient.Do(req)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	text := extractText(string(body))
	if len(text) > 4000 {
		text = text[:4000]
	}
	return map[string]any{"ok": true, "url": url, "content": text}
}

func extractText(html string) string {
	for _, re := range stripRes {
		html = re.ReplaceAllString(html, " ")
	}
	text := anyTagRe.ReplaceAllString(html, " ")
	text = wsRe.ReplaceAllString(text, " ")
	text = nlRe.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
