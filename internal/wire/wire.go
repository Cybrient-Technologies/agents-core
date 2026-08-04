// Package wire implements the runtime's HMAC request protocol and JSON envelope.
package wire

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	maxSkewSeconds = 300
	maxBodyBytes   = 10 << 20 // 10 MiB
)

// Ctx carries a verified request into a handler.
type Ctx struct {
	W    http.ResponseWriter
	R    *http.Request
	Raw  []byte
	Body map[string]any
}

// Str returns a string field from the decoded body (empty if missing).
func (c *Ctx) Str(key string) string {
	if v, ok := c.Body[key].(string); ok {
		return v
	}
	return ""
}

// Int returns an int field from the decoded body (JSON numbers decode as float64).
func (c *Ctx) Int(key string) int {
	switch v := c.Body[key].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// HTTPError is returned by a handler to produce a {ok:false,error} envelope.
type HTTPError struct {
	Status int
	Msg    string
}

func (e *HTTPError) Error() string { return e.Msg }

func Errf(status int, msg string) *HTTPError { return &HTTPError{Status: status, Msg: msg} }

// Handler returns the success payload (should include "ok":true; added if absent) or an *HTTPError.
type Handler func(*Ctx) (map[string]any, *HTTPError)

// Server routes verified POST requests.
type Server struct {
	Token        string
	SigningKey   string
	RequireHTTPS bool
	routes       map[string]Handler
}

func NewServer(token, signingKey string, requireHTTPS bool) *Server {
	return &Server{Token: token, SigningKey: signingKey, RequireHTTPS: requireHTTPS, routes: map[string]Handler{}}
}

func (s *Server) Handle(path string, h Handler) { s.routes[path] = h }

func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate")
}

func writeEnvelope(w http.ResponseWriter, status int, payload map[string]any) {
	setSecurityHeaders(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeEnvelope(w, status, map[string]any{"ok": false, "error": msg})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// HTTPS gate (TLS terminated at Nginx → check the forwarded proto).
	if s.RequireHTTPS && r.Header.Get("X-Forwarded-Proto") != "https" && r.TLS == nil {
		fail(w, http.StatusForbidden, "HTTPS required.")
		return
	}
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Method not allowed.")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		fail(w, http.StatusBadRequest, "Could not read body.")
		return
	}

	// Token (constant-time).
	if !hmac.Equal([]byte(r.Header.Get("X-AgentCore-Token")), []byte(s.Token)) {
		fail(w, http.StatusForbidden, "Forbidden.")
		return
	}

	// Signature (skipped only when SigningKey is empty).
	if s.SigningKey != "" {
		ts, _ := strconv.ParseInt(r.Header.Get("X-AgentCore-Ts"), 10, 64)
		if abs(time.Now().Unix()-ts) > maxSkewSeconds {
			fail(w, http.StatusForbidden, "Request timestamp out of range.")
			return
		}
		mac := hmac.New(sha256.New, []byte(s.SigningKey))
		mac.Write([]byte(strconv.FormatInt(ts, 10) + "." + string(raw)))
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-AgentCore-Sig"))) {
			fail(w, http.StatusForbidden, "Invalid request signature.")
			return
		}
	}

	h, ok := s.routes[r.URL.Path]
	if !ok {
		fail(w, http.StatusNotFound, "Not found.")
		return
	}

	body := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body) // lenient decode
	}

	payload, herr := h(&Ctx{W: w, R: r, Raw: raw, Body: body})
	if herr != nil {
		fail(w, herr.Status, herr.Msg)
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if _, has := payload["ok"]; !has {
		payload["ok"] = true
	}
	writeEnvelope(w, http.StatusOK, payload)
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
