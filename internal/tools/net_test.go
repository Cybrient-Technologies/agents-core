package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchSerper(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") == "" {
			t.Error("missing X-API-KEY")
		}
		_, _ = w.Write([]byte(`{"organic":[{"title":"Go","snippet":"a language","link":"https://go.dev"}]}`))
	}))
	defer ts.Close()

	serperURL = ts.URL
	t.Setenv("SERPER_API_KEY", "test-key")

	r := NewRegistry()
	out := r.Execute(context.Background(), "web_search", map[string]any{"query": "golang"}, nil, Env{})
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	if res["ok"] != true {
		t.Fatalf("web_search not ok: %s", out)
	}
	results, _ := res["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 result: %s", out)
	}
	first, _ := results[0].(map[string]any)
	if first["url"] != "https://go.dev" {
		t.Fatalf("bad result: %v", first)
	}
}

func TestHTTPRequestTool(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "yes" {
			t.Error("custom header not forwarded")
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`created`))
	}))
	defer ts.Close()
	httpToolClient = ts.Client()

	r := NewRegistry()
	out := r.Execute(context.Background(), "http_request", map[string]any{
		"url": ts.URL, "method": "post", "headers": map[string]any{"X-Custom": "yes"}, "body": "{}",
	}, nil, Env{})
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	if res["ok"] != true || res["status"].(float64) != 201 || !strings.Contains(res["body"].(string), "created") {
		t.Fatalf("http_request: %s", out)
	}
}
