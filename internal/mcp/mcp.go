// Package mcp is a minimal MCP client (JSON-RPC 2.0 over Streamable HTTP).
// Supports remote servers with none/bearer/header auth and captures
// the server version + negotiated protocol (for version governance).
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ProtocolVersion = "2025-06-18"

var client = &http.Client{Timeout: 60 * time.Second}

// Server describes a connected MCP server (auth value is plaintext at call time).
type Server struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	AuthType   string `json:"auth_type"` // none|bearer|header
	AuthHeader string `json:"auth_header"`
	AuthValue  string `json:"auth_value"`
	Tools      []Tool `json:"tools"`
}

// Tool is a normalized MCP tool schema.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ListResult struct {
	OK            bool
	Tools         []Tool
	Protocol      string
	ServerVersion string
	Error         string
}

type CallResult struct {
	OK    bool
	Text  string
	Error string
}

func (s *Server) authHeaders() map[string]string {
	h := map[string]string{}
	if s.AuthType == "bearer" && s.AuthValue != "" {
		h["Authorization"] = "Bearer " + s.AuthValue
	} else if s.AuthType == "header" && s.AuthValue != "" && s.AuthHeader != "" {
		h[s.AuthHeader] = s.AuthValue
	}
	return h
}

func post(ctx context.Context, url string, extra map[string]string, payload any) (int, http.Header, []byte, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, out, nil
}

// parse extracts a JSON-RPC message from a plain-JSON body or an SSE stream.
func parse(body []byte) map[string]any {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '{' {
		var m map[string]any
		if json.Unmarshal(trimmed, &m) == nil {
			return m
		}
		return nil
	}
	var result map[string]any
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(data), &m) == nil {
			if _, ok := m["result"]; ok {
				result = m
			} else if _, ok := m["error"]; ok {
				result = m
			} else if _, ok := m["id"]; ok {
				result = m
			}
		}
	}
	return result
}

type session struct {
	id       string
	protocol string
	svName   string
	svVer    string
}

// open runs initialize + notifications/initialized.
func (s *Server) open(ctx context.Context) (*session, error) {
	auth := s.authHeaders()
	code, hdr, body, err := post(ctx, s.URL, auth, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "agents.diy", "version": "1.0.0"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("HTTP %d", code)
	}
	msg := parse(body)
	if msg == nil || msg["error"] != nil {
		return nil, fmt.Errorf("initialize failed")
	}
	res, _ := msg["result"].(map[string]any)
	sess := &session{id: hdr.Get("Mcp-Session-Id")}
	if res != nil {
		sess.protocol, _ = res["protocolVersion"].(string)
		if info, ok := res["serverInfo"].(map[string]any); ok {
			sess.svName, _ = info["name"].(string)
			sess.svVer, _ = info["version"].(string)
		}
	}
	// fire-and-forget initialized notification
	nh := s.authHeaders()
	if sess.id != "" {
		nh["Mcp-Session-Id"] = sess.id
	}
	proto := sess.protocol
	if proto == "" {
		proto = ProtocolVersion
	}
	nh["MCP-Protocol-Version"] = proto
	_, _, _, _ = post(ctx, s.URL, nh, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return sess, nil
}

func (s *Server) sessionHeaders(sess *session) map[string]string {
	h := s.authHeaders()
	if sess.id != "" {
		h["Mcp-Session-Id"] = sess.id
	}
	proto := sess.protocol
	if proto == "" {
		proto = ProtocolVersion
	}
	h["MCP-Protocol-Version"] = proto
	return h
}

// ListTools handshakes and returns the server's tool list + version/protocol.
func (s *Server) ListTools(ctx context.Context) ListResult {
	sess, err := s.open(ctx)
	if err != nil {
		return ListResult{Error: err.Error()}
	}
	_, _, body, err := post(ctx, s.URL, s.sessionHeaders(sess), map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{},
	})
	if err != nil {
		return ListResult{Error: err.Error()}
	}
	msg := parse(body)
	if msg == nil || msg["error"] != nil {
		return ListResult{Error: "tools/list failed"}
	}
	result, _ := msg["result"].(map[string]any)
	rawTools, _ := result["tools"].([]any)
	tools := make([]Tool, 0, len(rawTools))
	for _, rt := range rawTools {
		tm, _ := rt.(map[string]any)
		name, _ := tm["name"].(string)
		if name == "" {
			continue
		}
		desc, _ := tm["description"].(string)
		schema := json.RawMessage(`{"type":"object","properties":{}}`)
		if is, ok := tm["inputSchema"]; ok {
			if b, err := json.Marshal(is); err == nil {
				schema = b
			}
		}
		tools = append(tools, Tool{Name: name, Description: desc, InputSchema: schema})
	}
	return ListResult{OK: true, Tools: tools, Protocol: sess.protocol, ServerVersion: sess.svVer}
}

// CallTool handshakes and invokes a tool, concatenating text content blocks.
func (s *Server) CallTool(ctx context.Context, toolName string, args map[string]any) CallResult {
	sess, err := s.open(ctx)
	if err != nil {
		return CallResult{Error: err.Error()}
	}
	if args == nil {
		args = map[string]any{}
	}
	_, _, body, err := post(ctx, s.URL, s.sessionHeaders(sess), map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": toolName, "arguments": args},
	})
	if err != nil {
		return CallResult{Error: err.Error()}
	}
	msg := parse(body)
	if msg == nil || msg["error"] != nil {
		return CallResult{Error: "tools/call failed"}
	}
	result, _ := msg["result"].(map[string]any)
	var text strings.Builder
	if content, ok := result["content"].([]any); ok {
		for _, blk := range content {
			if bm, ok := blk.(map[string]any); ok {
				if t, ok := bm["text"].(string); ok {
					text.WriteString(t)
				}
			}
		}
	}
	if isErr, _ := result["isError"].(bool); isErr {
		if text.Len() == 0 {
			return CallResult{Error: "tool returned an error"}
		}
		return CallResult{Error: text.String()}
	}
	return CallResult{OK: true, Text: text.String()}
}
