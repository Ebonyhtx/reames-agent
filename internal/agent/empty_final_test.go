package agent

import (
	"context"
	"strings"
	"testing"

	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestRunRetriesReasoningOnlyFinalAnswer(t *testing.T) {
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{
			{Type: provider.ChunkReasoning, Text: "I should answer the user."},
			{Type: provider.ChunkDone},
		},
		{
			{Type: provider.ChunkText, Text: "visible reply"},
			{Type: provider.ChunkDone},
		},
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)

	if err := a.Run(context.Background(), "answer me"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prov.call != 2 {
		t.Fatalf("provider calls = %d, want retry after reasoning-only answer", prov.call)
	}
	if got := lastAssistantContent(a.session); got != "visible reply" {
		t.Fatalf("last assistant content = %q, want visible reply", got)
	}
	if !sessionHasUserMessageContaining(a.session, "visible answer") {
		t.Fatal("missing synthetic visible-answer retry message")
	}
}

func TestRunPrefixesReasoningLanguageOnSyntheticRetry(t *testing.T) {
	prov := &mockProvider{name: "p", streams: [][]provider.Chunk{
		{
			{Type: provider.ChunkReasoning, Text: "I should answer the user."},
			{Type: provider.ChunkDone},
		},
		{
			{Type: provider.ChunkText, Text: "visible reply"},
			{Type: provider.ChunkDone},
		},
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{ReasoningLanguage: "zh"}, event.Discard)

	if err := a.Run(context.Background(), "answer me"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(prov.requests))
	}
	for i, req := range prov.requests {
		got := lastUser(req)
		if !strings.HasPrefix(got, "<reasoning-language>") || !strings.Contains(got, "简体中文") {
			t.Fatalf("request %d last user = %q, want reasoning-language prefix", i, got)
		}
	}
	if !strings.Contains(lastUser(prov.requests[1]), "visible answer") {
		t.Fatalf("retry request last user = %q, want visible-answer retry", lastUser(prov.requests[1]))
	}
}

func TestRunStopsAfterRepeatedEmptyFinalAnswers(t *testing.T) {
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{{Type: provider.ChunkReasoning, Text: "thinking 1"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkReasoning, Text: "thinking 2"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkReasoning, Text: "thinking 3"}, {Type: provider.ChunkDone}},
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)

	err := a.Run(context.Background(), "answer me")
	if err == nil {
		t.Fatal("expected repeated empty final answers to stop the run")
	}
	if !strings.Contains(err.Error(), "visible final answer") {
		t.Fatalf("error = %v, want visible final answer", err)
	}
	if prov.call != 3 {
		t.Fatalf("provider calls = %d, want three empty-answer attempts", prov.call)
	}
}

func lastAssistantContent(s *Session) string {
	var out string
	for _, m := range s.Messages {
		if m.Role == provider.RoleAssistant {
			out = m.Content
		}
	}
	return out
}

type deepseekThinkingProvider struct{ *scriptedProvider }

func (deepseekThinkingProvider) RequiresToolCallReasoning() bool { return true }

func TestRunHonoursDeepSeekReasoningOnlyStop(t *testing.T) {
	prov := &scriptedProvider{name: "deepseek", turns: [][]provider.Chunk{{
		{Type: provider.ChunkReasoning, Text: "The requested answer was completed in the reasoning stream."},
		{Type: provider.ChunkUsage, Usage: &provider.Usage{FinishReason: "stop", TotalTokens: 10}},
		{Type: provider.ChunkDone},
	}}}
	a := New(deepseekThinkingProvider{prov}, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)

	if err := a.Run(context.Background(), "answer me"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prov.call != 1 {
		t.Fatalf("provider calls = %d, want 1 after an explicit DeepSeek stop", prov.call)
	}
	if sessionHasUserMessageContaining(a.session, "visible answer") {
		t.Fatal("must not inject an empty-answer retry after an explicit DeepSeek reasoning stop")
	}
}

func TestRunRetriesReasoningOnlyStopOutsideDeepSeekPolicy(t *testing.T) {
	prov := &scriptedProvider{name: "gateway", turns: [][]provider.Chunk{
		{
			{Type: provider.ChunkReasoning, Text: "reasoning only"},
			{Type: provider.ChunkUsage, Usage: &provider.Usage{FinishReason: "stop", TotalTokens: 10}},
			{Type: provider.ChunkDone},
		},
		{{Type: provider.ChunkText, Text: "visible reply"}, {Type: provider.ChunkDone}},
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)

	if err := a.Run(context.Background(), "answer me"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prov.call != 2 {
		t.Fatalf("provider calls = %d, want retry outside DeepSeek policy", prov.call)
	}
	if !sessionHasUserMessageContaining(a.session, "visible answer") {
		t.Fatal("non-DeepSeek provider lost the empty-answer retry guard")
	}
}

func BenchmarkHasVisibleFinalAnswer(b *testing.B) {
	cases := []struct {
		name string
		text string
	}{
		{"normal", "visible reply"},
		{"leading-space", strings.Repeat(" ", 256) + "visible reply"},
		{"all-space", strings.Repeat(" \n\t", 256)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var got bool
			for i := 0; i < b.N; i++ {
				got = hasVisibleFinalAnswer(tc.text)
			}
			_ = got
		})
	}
}
