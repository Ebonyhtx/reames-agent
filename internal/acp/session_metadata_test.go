package acp

import (
	"strings"
	"testing"

	"reames-agent/internal/control"
)

func TestTitleFromTranscriptSkipsHiddenPromptInternals(t *testing.T) {
	transcript := []control.TranscriptMessage{
		{Role: control.TranscriptSystem, Content: "SYSTEM-SECRET", Hidden: true},
		{Role: control.TranscriptUser, Content: "REFERENCED-CONTEXT-SECRET", Hidden: true},
		{Role: control.TranscriptUser, Content: "visible session task"},
	}

	got := titleFromTranscript(transcript)
	if got != "visible session task" {
		t.Fatalf("title = %q, want visible user content", got)
	}
	if strings.Contains(got, "SECRET") {
		t.Fatalf("hidden prompt internals leaked into title: %q", got)
	}
}
