// Package telegram is a minimal Telegram Bot client for the OSS runtime: long-polling
// (no public URL needed for self-host) + sendMessage, wired to run the agent per message.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Bot talks to one Telegram bot.
type Bot struct {
	client  *http.Client
	apiBase string // https://api.telegram.org/bot<token>
}

func New(token string) *Bot {
	return &Bot{
		client:  &http.Client{Timeout: 65 * time.Second},
		apiBase: "https://api.telegram.org/bot" + token,
	}
}

// Update / Message are the subset of the Telegram API we consume.
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

// RunFunc runs one inbound message and returns the reply text ("" = no reply).
type RunFunc func(ctx context.Context, chatID int64, text string) string

// SendMessage posts a text reply to a chat.
func (b *Bot) SendMessage(ctx context.Context, chatID int64, text string) error {
	payload, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiBase+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// getUpdates long-polls for new updates since offset.
func (b *Bot) getUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	q := url.Values{}
	q.Set("offset", strconv.FormatInt(offset, 10))
	q.Set("timeout", strconv.Itoa(timeoutSec))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.apiBase+"/getUpdates?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var d struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	return d.Result, nil
}

// processUpdate handles one update: run the agent on the text and send the reply.
// Returns the next offset (update_id + 1). Non-text updates are skipped.
func (b *Bot) processUpdate(ctx context.Context, u Update, run RunFunc) int64 {
	next := u.UpdateID + 1
	if u.Message == nil || u.Message.Text == "" {
		return next
	}
	reply := run(ctx, u.Message.Chat.ID, u.Message.Text)
	if reply != "" {
		_ = b.SendMessage(ctx, u.Message.Chat.ID, reply)
	}
	return next
}

// Poll runs the long-poll loop until ctx is cancelled.
func (b *Bot) Poll(ctx context.Context, run RunFunc) error {
	var offset int64
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		updates, err := b.getUpdates(ctx, offset, 30)
		if err != nil {
			// back off briefly, respecting cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			offset = b.processUpdate(ctx, u, run)
		}
	}
}
