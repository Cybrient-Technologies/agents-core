package llm

import "testing"

func TestResolve(t *testing.T) {
	cases := []struct {
		apiKey, model string
		wantKind, wantEndpoint, wantModel string
	}{
		{"sk-or-v1-x", "claude-opus-4-8", "openai", epOpenRouter, "anthropic/claude-opus-4-8"},
		{"sk-bold-x", "claude-opus-4-8", "openai", epBoldRouter, "anthropic/claude-opus-4.8"},
		{"sk-ant-x", "claude-sonnet-4-6", "anthropic", epAnthropic, "claude-sonnet-4-6"},
		{"sk-x", "gpt-4o", "openai", epOpenAI, "gpt-4o"},
		{"k", "MiniMax-M2.1", "openai", epMiniMax, "MiniMax-M2.1"},
		{"k", "gemini-3-flash", "gemini", "", "gemini-3-flash"},
		{"k", "totally-unknown", "unknown", "", "totally-unknown"},
	}
	for _, c := range cases {
		got := resolve(c.apiKey, c.model)
		if got.kind != c.wantKind || got.endpoint != c.wantEndpoint || got.model != c.wantModel {
			t.Errorf("resolve(%q,%q) = %+v; want {%s %s %s}",
				c.apiKey, c.model, got, c.wantKind, c.wantEndpoint, c.wantModel)
		}
	}
}

func TestModelMaps(t *testing.T) {
	if got := orModelID("claude-opus-4-8"); got != "anthropic/claude-opus-4-8" {
		t.Errorf("orModelID hyphen: %s", got)
	}
	if got := orModelID("openrouter/anthropic/claude-x"); got != "anthropic/claude-x" {
		t.Errorf("orModelID strip: %s", got)
	}
	if got := boldModelID("claude-sonnet-4-6"); got != "anthropic/claude-sonnet-4.6" {
		t.Errorf("boldModelID dotted: %s", got)
	}
	if got := boldModelID("bold/auto"); got != "bold/auto" {
		t.Errorf("boldModelID passthrough: %s", got)
	}
	if got := boldModelID("gpt-4.1-mini"); got != "openai/gpt-4o-mini" {
		t.Errorf("boldModelID fallback map: %s", got)
	}
}
