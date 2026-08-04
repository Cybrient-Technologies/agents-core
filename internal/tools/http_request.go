package tools

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Cybrient-Technologies/agents-core/internal/llm"
)

var httpToolClient = &http.Client{Timeout: 20 * time.Second}

// httpRequest makes an authenticated API call.
// Marked needs-approval; the loop doesn't gate approvals yet.
type httpRequest struct{}

func (httpRequest) Name() string { return "http_request" }

func (httpRequest) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "http_request",
		Description: "Make an HTTP request to an external API (CRMs, accounting, project tools). Supports GET/POST/PUT/PATCH/DELETE with headers and a JSON body.",
		InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string"},"method":{"type":"string"},"headers":{"type":"object"},"body":{"type":"string"}},"required":["url"]}`),
	}
}

func (httpRequest) Execute(ctx context.Context, input map[string]any, _ Env) map[string]any {
	url, _ := input["url"].(string)
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "https://") {
		return map[string]any{"ok": false, "error": "a valid https:// url is required"}
	}
	method := strings.ToUpper(strings.TrimSpace(str(input["method"])))
	if method == "" {
		method = "GET"
	}
	var bodyReader io.Reader
	if b := str(input["body"]); b != "" {
		bodyReader = strings.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	if hdrs, ok := input["headers"].(map[string]any); ok {
		for k, v := range hdrs {
			req.Header.Set(k, str(v))
		}
	}
	resp, err := httpToolClient.Do(req)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return map[string]any{"ok": true, "status": resp.StatusCode, "body": string(out)}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
