package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetUpdatesSendAndProcess(t *testing.T) {
	var sentChatID int64
	var sentText string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var p struct {
				ChatID int64  `json:"chat_id"`
				Text   string `json:"text"`
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &p)
			sentChatID, sentText = p.ChatID, p.Text
			_, _ = w.Write([]byte(`{"ok":true}`))
		case strings.Contains(r.URL.Path, "/getUpdates"):
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":10,"message":{"chat":{"id":555},"text":"hello"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	b := &Bot{client: ts.Client(), apiBase: ts.URL}

	ups, err := b.getUpdates(context.Background(), 0, 0)
	if err != nil || len(ups) != 1 || ups[0].Message == nil || ups[0].Message.Text != "hello" {
		t.Fatalf("getUpdates = %+v err=%v", ups, err)
	}

	var ranText string
	next := b.processUpdate(context.Background(), ups[0], func(ctx context.Context, chatID int64, text string) string {
		ranText = text
		return "reply: " + text
	})
	if next != 11 {
		t.Fatalf("next offset = %d, want 11", next)
	}
	if ranText != "hello" {
		t.Fatalf("run got %q", ranText)
	}
	if sentChatID != 555 || sentText != "reply: hello" {
		t.Fatalf("sent chat=%d text=%q", sentChatID, sentText)
	}
}

func TestProcessSkipsNonText(t *testing.T) {
	b := &Bot{apiBase: "http://unused"}
	called := false
	// update with no message → skipped, no run, offset advances
	next := b.processUpdate(context.Background(), Update{UpdateID: 7}, func(context.Context, int64, string) string {
		called = true
		return "x"
	})
	if next != 8 || called {
		t.Fatalf("non-text update should skip run; next=%d called=%v", next, called)
	}
}
