package control

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/provider"
)

func TestTranscriptMessagesHidePromptInternalsAndPreserveDisplayData(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: "SYSTEM-SECRET"},
		{Role: provider.RoleUser, Content: "<reasoning-language>English</reasoning-language>\n\nvisible request", Edited: true, Original: "<reasoning-language>Chinese</reasoning-language>\n\noriginal request"},
		{Role: provider.RoleUser, Content: "Referenced context:\n<file path=\"secret.txt\">FILE-SECRET</file>\n\nexplain it"},
		{Role: provider.RoleUser, Content: "The previous assistant response finished without any visible answer text. Continue the same task now and provide a concise visible answer to the user."},
		{Role: provider.RoleUser, Content: agent.MidTurnSteerPrefix + "\nfocus tests"},
		{Role: provider.RoleAssistant, Content: "done", ReasoningContent: "reason", ReasoningBlocks: []provider.ReasoningBlock{
			{Type: "openai_reasoning", Text: "reason", Data: "OPENAI-OPAQUE-SECRET"},
			{Type: "redacted_thinking", Data: "ANTHROPIC-OPAQUE-SECRET"},
		}, ToolCalls: []provider.ToolCall{{
			ID: "call-1", Name: "write_file", Arguments: `{"path":"a.txt"}`, Diff: "+hello", Added: 1,
		}}, MemoryCitations: []provider.MemoryCitation{{ID: "mem-1", Source: "MEMORY.md", LineStart: 3, Note: "rule"}}},
		{Role: provider.RoleTool, Content: "ok", ToolCallID: "call-1", Name: "write_file"},
	}

	got := transcriptMessages(messages)
	if len(got) != len(messages) {
		t.Fatalf("transcript length = %d, want %d", len(got), len(messages))
	}
	if !got[0].Hidden || got[0].Content != "" {
		t.Fatalf("system entry leaked: %+v", got[0])
	}
	if got[1].Content != "visible request" || !got[1].Edited || got[1].Original != "original request" {
		t.Fatalf("visible edited entry = %+v", got[1])
	}
	if got[1].ReplayText != "visible request" {
		t.Fatalf("safe replay text = %q", got[1].ReplayText)
	}
	if got[2].Content != "explain it" || strings.Contains(got[2].Content, "FILE-SECRET") {
		t.Fatalf("referenced context leaked: %+v", got[2])
	}
	if got[2].ReplayText != "explain it" || strings.Contains(got[2].ReplayText, "FILE-SECRET") {
		t.Fatalf("referenced context leaked into replay text: %+v", got[2])
	}
	if !got[3].Hidden || got[3].Content != "" {
		t.Fatalf("synthetic entry leaked: %+v", got[3])
	}
	if got[4].SteerText != "focus tests" || got[4].Content != "focus tests" {
		t.Fatalf("steer entry = %+v", got[4])
	}
	if got[3].ReplayText != "" || got[4].ReplayText != "" {
		t.Fatalf("synthetic or steer message became replayable: synthetic=%+v steer=%+v", got[3], got[4])
	}
	if got[5].Reasoning != "reason" || len(got[5].ToolCalls) != 1 || got[5].ToolCalls[0].Diff != "+hello" {
		t.Fatalf("assistant display data = %+v", got[5])
	}
	if len(got[5].MemoryCitations) != 1 || got[5].MemoryCitations[0].Source != "MEMORY.md" {
		t.Fatalf("memory citations = %+v", got[5].MemoryCitations)
	}
	displayJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(displayJSON), "OPENAI-OPAQUE-SECRET") || strings.Contains(string(displayJSON), "ANTHROPIC-OPAQUE-SECRET") {
		t.Fatalf("opaque provider reasoning leaked into transcript DTO: %s", displayJSON)
	}
	if got[6].ToolCallID != "call-1" || got[6].ToolName != "write_file" || got[6].Content != "ok" {
		t.Fatalf("tool entry = %+v", got[6])
	}
	if messages[0].Content != "SYSTEM-SECRET" || !strings.Contains(messages[2].Content, "FILE-SECRET") {
		t.Fatal("transcript conversion mutated runtime history")
	}
}

func TestTranscriptMessageDisplayKeyAndReplayContract(t *testing.T) {
	memoryContract := `<memory-compiler-execution>{"planner_ir":{"source_event":"ship safely"}}</memory-compiler-execution>`
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "expanded prompt"},
		{Role: provider.RoleUser, Content: memoryContract},
	}

	got := transcriptMessages(messages)
	if got[0].DisplayKey != "4b1b46c9a040fb9d9813b9466a0c49061c91b426fb8e32d99a954dfdae792066" {
		t.Fatalf("display key = %q", got[0].DisplayKey)
	}
	if got[0].ReplayText != "expanded prompt" {
		t.Fatalf("normal replay text = %q", got[0].ReplayText)
	}
	if got[1].Content != "ship safely" || got[1].ReplayText != "" {
		t.Fatalf("memory compiler transcript = %+v", got[1])
	}
	if got[0].Index != 0 || got[1].Index != 1 {
		t.Fatalf("original indexes changed: %+v", got)
	}
	if messages[0].Content != "expanded prompt" || messages[1].Content != memoryContract {
		t.Fatal("transcript projection mutated runtime messages")
	}
	localOnly := TranscriptMessage{Role: TranscriptUser, Content: "display prompt", DisplayKey: got[0].DisplayKey, ReplayText: "expanded prompt"}
	encoded, err := json.Marshal(localOnly)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), got[0].DisplayKey) || strings.Contains(string(encoded), "replayText") || strings.Contains(string(encoded), "expanded prompt") {
		t.Fatalf("local transcript correlation metadata crossed JSON transport: %s", encoded)
	}
}

func TestLoadTranscriptAppliesDisplaySafetyToPersistedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	session := agent.NewSession("SYSTEM-SECRET")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "Referenced context:\n<file path=\"secret.txt\">FILE-SECRET</file>\n\nremember visible preference"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Hidden || got[1].Content != "remember visible preference" {
		t.Fatalf("persisted transcript projection = %+v", got)
	}
	if strings.Contains(got[1].Content, "FILE-SECRET") {
		t.Fatalf("referenced context leaked into persisted transcript projection: %+v", got[1])
	}
}
