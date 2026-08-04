package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeMCP is a minimal JSON-RPC MCP server for conformance testing.
func fakeMCP(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		method, _ := req["method"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-123")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]any{
					"protocolVersion": ProtocolVersion,
					"serverInfo":      map[string]any{"name": "fake", "version": "9.9.9"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]any{"tools": []any{
					map[string]any{"name": "ask", "description": "ask a question",
						"inputSchema": map[string]any{"type": "object"}},
				}},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]any{"content": []any{
					map[string]any{"type": "text", "text": "the answer is 42"},
				}},
			})
		default:
			http.Error(w, "bad method", 400)
		}
	}))
}

func TestListAndCall(t *testing.T) {
	ts := fakeMCP(t)
	defer ts.Close()
	s := &Server{Slug: "fake", Name: "Fake", URL: ts.URL, AuthType: "none"}

	list := s.ListTools(context.Background())
	if !list.OK || len(list.Tools) != 1 || list.Tools[0].Name != "ask" {
		t.Fatalf("ListTools = %+v", list)
	}
	if list.ServerVersion != "9.9.9" || list.Protocol != ProtocolVersion {
		t.Fatalf("version/protocol not captured: %+v", list)
	}

	call := s.CallTool(context.Background(), "ask", map[string]any{"q": "meaning?"})
	if !call.OK || call.Text != "the answer is 42" {
		t.Fatalf("CallTool = %+v", call)
	}
}
