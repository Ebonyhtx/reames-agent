package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestProviderAPIModeNormalizesAndValidates(t *testing.T) {
	var cfg Config
	entry := ProviderEntry{Name: "openai", Kind: "openai", BaseURL: "https://api.openai.com/v1", APIMode: "responses", Model: "gpt-test"}
	if err := cfg.UpsertProvider(entry); err != nil {
		t.Fatal(err)
	}
	got, ok := cfg.Provider("openai")
	if !ok || got.APIMode != "responses" {
		t.Fatalf("provider = %+v, ok=%v", got, ok)
	}

	entry.Name = "chat"
	entry.APIMode = "chat"
	if err := cfg.UpsertProvider(entry); err != nil {
		t.Fatal(err)
	}
	got, _ = cfg.Provider("chat")
	if got.APIMode != "chat_completions" {
		t.Fatalf("chat alias normalized to %q", got.APIMode)
	}

	entry.Name = "bad"
	entry.APIMode = "future"
	if err := cfg.UpsertProvider(entry); err == nil || !strings.Contains(err.Error(), "api_mode") {
		t.Fatalf("invalid api_mode error = %v", err)
	}
	entry.Name = "anthropic"
	entry.Kind = "anthropic"
	entry.APIMode = "responses"
	if err := cfg.UpsertProvider(entry); err == nil || !strings.Contains(err.Error(), "kind=openai") {
		t.Fatalf("non-openai api_mode error = %v", err)
	}
}

func TestProviderAPIModeRendersAndRoundTrips(t *testing.T) {
	cfg := Default()
	cfg.Providers = []ProviderEntry{{
		Name:      "openai",
		Kind:      "openai",
		BaseURL:   "https://api.openai.com/v1",
		APIMode:   "responses",
		Model:     "gpt-test",
		APIKeyEnv: "OPENAI_API_KEY",
	}}
	rendered := RenderTOML(cfg)
	if !strings.Contains(rendered, `api_mode    = "responses"`) {
		t.Fatalf("rendered config missing api_mode:\n%s", rendered)
	}
	var decoded Config
	if _, err := toml.Decode(rendered, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Providers) != 1 || decoded.Providers[0].APIMode != "responses" {
		t.Fatalf("decoded providers = %+v", decoded.Providers)
	}
}
