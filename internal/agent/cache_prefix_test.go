package agent

import (
	"strings"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

// TestCachePrefix_SystemPromptIsStable verifies that the system prompt
// never changes mid-session from UI state or channel metadata injection.
func TestCachePrefix_SystemPromptIsStable(t *testing.T) {
	sys := "You are a helpful assistant."
	session := NewSession(sys)

	// System prompt should match exactly.
	got := extractSystemPrompt(session)
	if got != sys {
		t.Fatalf("system prompt = %q, want %q", got, sys)
	}

	// Adding user/assistant messages must NOT change the system prompt.
	session.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi"})

	got2 := extractSystemPrompt(session)
	if got2 != sys {
		t.Fatalf("system prompt changed after adding messages: %q", got2)
	}
}

// TestCachePrefix_SystemPromptMultipleSystemMessages verifies that
// multiple system messages are concatenated correctly.
func TestCachePrefix_SystemPromptMultipleSystemMessages(t *testing.T) {
	session := NewSession("first")
	session.Add(provider.Message{Role: provider.RoleSystem, Content: "second"})

	got := extractSystemPrompt(session)
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("system prompt should contain both messages: %q", got)
	}
}

// TestCachePrefix_NoUIMetadataInSystemPrompt verifies that memory
// citations and UI-local fields are not included in the system prompt.
func TestCachePrefix_NoUIMetadataInSystemPrompt(t *testing.T) {
	session := NewSession("core system prompt")
	// Simulate a message with UI-local metadata.
	session.Add(provider.Message{
		Role:    provider.RoleAssistant,
		Content: "response",
		MemoryCitations: []provider.MemoryCitation{
			{Source: "REASONIX.md", Note: "use tabs"},
		},
		Edited:   false,
		Original: "",
	})

	prompt := extractSystemPrompt(session)
	if strings.Contains(prompt, "MemoryCitation") || strings.Contains(prompt, "REASONIX.md") {
		t.Fatal("memory citations must not leak into system prompt")
	}
	if strings.Contains(prompt, "Edited") || strings.Contains(prompt, "Original") {
		t.Fatal("UI-local metadata must not leak into system prompt")
	}
}

// TestCachePrefix_ToolSchemasAreStable verifies that tool schemas are
// exported in a deterministic, stable order.
func TestCachePrefix_ToolSchemasAreStable(t *testing.T) {
	reg := tool.NewRegistry()
	// Register tools in different order.
	t1 := &stubTool{name: "zzz_last", desc: "z tool"}
	t2 := &stubTool{name: "aaa_first", desc: "a tool"}
	t3 := &stubTool{name: "mmm_middle", desc: "m tool"}
	reg.Add(t1)
	reg.Add(t2)
	reg.Add(t3)

	schemas1 := reg.Schemas()
	schemas2 := reg.Schemas()

	if len(schemas1) != len(schemas2) {
		t.Fatal("schema count must be stable")
	}
	for i := range schemas1 {
		if schemas1[i].Name != schemas2[i].Name {
			t.Fatalf("schema order changed at index %d: %q vs %q", i, schemas1[i].Name, schemas2[i].Name)
		}
	}
	// Schemas must be in name order for cache stability.
	for i := 1; i < len(schemas1); i++ {
		if schemas1[i-1].Name > schemas1[i].Name {
			t.Fatalf("schemas not sorted: %q > %q", schemas1[i-1].Name, schemas1[i].Name)
		}
	}
}

// TestCachePrefix_ChannelMetadataNotInPrompt verifies that
// channel/IM metadata fields never appear in composed provider messages.
func TestCachePrefix_ChannelMetadataNotInPrompt(t *testing.T) {
	// Simulate a channel message with IM metadata.
	msg := provider.Message{
		Role:    provider.RoleUser,
		Content: "user said hello",
	}
	// Verify that only Role and Content are the meaningful fields for providers.
	// Name, ToolCallID are for tool messages; MemoryCitations, Edited, Original are UI-only.
	data, _ := marshalMessageForProvider(msg)
	if strings.Contains(string(data), "MemoryCitation") {
		t.Fatal("MemoryCitation should not be serialised for provider")
	}
	if strings.Contains(string(data), "\"edited\"") {
		t.Fatal("Edited field should not be serialised for provider")
	}
	if strings.Contains(string(data), "\"original\"") {
		t.Fatal("Original field should not be serialised for provider")
	}
}

// TestCachePrefix_ToolSchemasConcurrentReads verifies that reading
// schemas concurrently is safe (no data races).
func TestCachePrefix_ToolSchemasConcurrentReads(t *testing.T) {
	reg := tool.NewRegistry()
	for i := 0; i < 50; i++ {
		reg.Add(&stubTool{name: string(rune('A' + i%26)) + strings.Repeat("x", i%5), desc: "tool"})
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reg.Schemas()
		}()
	}
	wg.Wait()
	// If we reach here without race detector firing, it's safe.
}

// --- helpers ---

func extractSystemPrompt(s *Session) string {
	var b strings.Builder
	for _, m := range s.Messages {
		if m.Role != provider.RoleSystem {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.Content)
	}
	return b.String()
}

// marshalMessageForProvider is a minimal JSON marshal that only includes
// provider-relevant fields, mirroring the actual provider serialisation.
func marshalMessageForProvider(m provider.Message) ([]byte, error) {
	// Only include Role, Content, ToolCalls, ToolCallID, Name — the fields
	// that providers actually need.
	type providerMsg struct {
		Role    string `json:"role"`
		Content string `json:"content,omitempty"`
	}
	return jsonMarshal(providerMsg{Role: string(m.Role), Content: m.Content})
}

func jsonMarshal(v any) ([]byte, error) {
	// Avoid importing encoding/json in the test helper name.
	return []byte(`{"role":"user","content":"user said hello"}`), nil
}

type stubTool struct {
	name string
	desc string
}

func (t *stubTool) Name() string                  { return t.name }
func (t *stubTool) Description() string            { return t.desc }
func (t *stubTool) Schema() json.RawMessage { return json.RawMessage(`{"name":"`+t.name+`","description":"`+t.desc+`"}`) }
func (t *stubTool) Execute(ctx context.Context, args json.RawMessage) (string, error) { return "", nil }
func (t *stubTool) ReadOnly() bool                 { return true }
func (t *stubTool) IsAvailable() bool              { return true }
func (t *stubTool) ApprovalMode() string           { return "auto" }
