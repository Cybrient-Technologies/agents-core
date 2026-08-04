package wire

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testToken = "tok-abc"
	testKey   = "signing-key-xyz"
)

func newTestServer() *Server {
	s := NewServer(testToken, testKey, false) // RequireHTTPS off for the test
	s.Handle("/api/echo", func(c *Ctx) (map[string]any, *HTTPError) {
		if c.Str("fail") == "yes" {
			return nil, Errf(http.StatusUnprocessableEntity, "asked to fail")
		}
		return map[string]any{"echo": c.Str("msg")}, nil
	})
	return s
}

func signedReq(path, body string, token, key string, ts int64) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("X-AgentCore-Token", token)
	r.Header.Set("X-AgentCore-Ts", strconv.FormatInt(ts, 10))
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "." + body))
	r.Header.Set("X-AgentCore-Sig", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func run(s *Server, r *http.Request) (int, map[string]any) {
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var body map[string]any
	data, _ := io.ReadAll(w.Result().Body)
	_ = json.Unmarshal(data, &body)
	return w.Code, body
}

func TestValidRequest(t *testing.T) {
	s := newTestServer()
	code, body := run(s, signedReq("/api/echo", `{"msg":"hi"}`, testToken, testKey, time.Now().Unix()))
	if code != 200 || body["ok"] != true || body["echo"] != "hi" {
		t.Fatalf("valid request: code=%d body=%v", code, body)
	}
}

func TestBadToken(t *testing.T) {
	s := newTestServer()
	code, body := run(s, signedReq("/api/echo", `{}`, "wrong", testKey, time.Now().Unix()))
	if code != 403 || body["error"] != "Forbidden." {
		t.Fatalf("bad token: code=%d body=%v", code, body)
	}
}

func TestBadSignature(t *testing.T) {
	s := newTestServer()
	r := signedReq("/api/echo", `{}`, testToken, "wrong-key", time.Now().Unix())
	code, body := run(s, r)
	if code != 403 || body["error"] != "Invalid request signature." {
		t.Fatalf("bad sig: code=%d body=%v", code, body)
	}
}

func TestStaleTimestamp(t *testing.T) {
	s := newTestServer()
	code, body := run(s, signedReq("/api/echo", `{}`, testToken, testKey, time.Now().Unix()-1000))
	if code != 403 || body["error"] != "Request timestamp out of range." {
		t.Fatalf("stale ts: code=%d body=%v", code, body)
	}
}

func TestMethodAndRoute(t *testing.T) {
	s := newTestServer()
	// GET → 405
	get := httptest.NewRequest(http.MethodGet, "/api/echo", nil)
	if code, _ := run(s, get); code != 405 {
		t.Fatalf("GET should be 405, got %d", code)
	}
	// unknown route → 404
	code, body := run(s, signedReq("/api/nope", `{}`, testToken, testKey, time.Now().Unix()))
	if code != 404 || body["error"] != "Not found." {
		t.Fatalf("unknown route: code=%d body=%v", code, body)
	}
}

func TestHandlerError(t *testing.T) {
	s := newTestServer()
	code, body := run(s, signedReq("/api/echo", `{"fail":"yes"}`, testToken, testKey, time.Now().Unix()))
	if code != 422 || body["ok"] != false || body["error"] != "asked to fail" {
		t.Fatalf("handler error: code=%d body=%v", code, body)
	}
}
