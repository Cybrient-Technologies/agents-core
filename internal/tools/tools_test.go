package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cybrient-Technologies/agents-core/internal/mcp"
)

func TestSchemasForFilters(t *testing.T) {
	r := NewRegistry()
	servers := []mcp.Server{{Slug: "dw", Name: "DeepWiki", Tools: []mcp.Tool{{Name: "ask", Description: "ask"}}}}

	all := r.SchemasFor(nil, nil, servers)
	names := map[string]bool{}
	for _, s := range all {
		names[s.Name] = true
	}
	if !names["fetch_page"] || !names["mcp__dw__ask"] {
		t.Fatalf("expected fetch_page + mcp__dw__ask, got %v", names)
	}

	// deny filters out
	denied := r.SchemasFor(nil, []string{"fetch_page"}, servers)
	for _, s := range denied {
		if s.Name == "fetch_page" {
			t.Fatal("fetch_page should be denied")
		}
	}
	// allow-list restricts
	only := r.SchemasFor([]string{"mcp__dw__ask"}, nil, servers)
	if len(only) != 1 || only[0].Name != "mcp__dw__ask" {
		t.Fatalf("allow-list failed: %+v", only)
	}
}

func TestExecuteFetchPage(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body><h1>Hello</h1><script>x=1</script> World</body></html>"))
	}))
	defer ts.Close()
	// point the tool's client at the test server's TLS
	fetchClient = ts.Client()

	r := NewRegistry()
	out := r.Execute(context.Background(), "fetch_page", map[string]any{"url": ts.URL}, nil, Env{})
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	if res["ok"] != true {
		t.Fatalf("fetch_page not ok: %s", out)
	}
	content, _ := res["content"].(string)
	if !strings.Contains(content, "Hello") || !strings.Contains(content, "World") || strings.Contains(content, "x=1") {
		t.Fatalf("bad extraction: %q", content)
	}
}

func TestExecuteMCPNotConnected(t *testing.T) {
	r := NewRegistry()
	out := r.Execute(context.Background(), "mcp__ghost__ask", map[string]any{}, nil, Env{})
	if !strings.Contains(out, "not connected") {
		t.Fatalf("expected not-connected error, got %s", out)
	}
	out = r.Execute(context.Background(), "does_not_exist", nil, nil, Env{})
	if !strings.Contains(out, "Unknown tool") {
		t.Fatalf("expected unknown-tool error, got %s", out)
	}
}
