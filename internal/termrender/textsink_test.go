package termrender

import (
	"strings"
	"testing"

	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

type fixedRenderer struct{}

func (fixedRenderer) Render(string) string { return "RENDERED" }

func TestTextSinkRendersEventContract(t *testing.T) {
	var output strings.Builder
	sink := NewTextSink(&output, nil, 80)
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.Text, Text: "answer"})
	sink.Emit(event.Event{Kind: event.Message, Text: "answer"})
	sink.Emit(event.Event{Kind: event.Usage, Usage: &provider.Usage{
		PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CacheHitTokens: 80,
	}})
	if got := output.String(); got != "answer\n  · 120 tok · in 100 (80 cached / 20 new) · out 20\n" {
		t.Fatalf("terminal event output = %q", got)
	}
}

func TestTextSinkRedrawUsesVisibleTerminalWidth(t *testing.T) {
	var output strings.Builder
	sink := NewTextSink(&output, fixedRenderer{}, 10)
	text := strings.Repeat("中", 6)
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.Text, Text: text})
	sink.Emit(event.Event{Kind: event.Message, Text: text})
	if got := output.String(); got != text+"\r\033[1A\033[0JRENDERED" {
		t.Fatalf("wide-character redraw = %q", got)
	}
}
