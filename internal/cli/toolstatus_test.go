package cli

import (
	"strings"
	"testing"
)

func TestFormatToolStatusSuccess(t *testing.T) {
	s := FormatToolStatus("bash", "go build ./...", 1200, false)
	if !strings.Contains(s, "⚡") || !strings.Contains(s, "run") || !strings.Contains(s, "1.2s") {
		t.Fatalf("unexpected: %q", s)
	}
	if strings.Contains(s, "FAILED") {
		t.Fatal("should not say FAILED")
	}
}

func TestFormatToolStatusFailed(t *testing.T) {
	s := FormatToolStatus("grep", "pattern", 500, true)
	if !strings.Contains(s, "FAILED") {
		t.Fatalf("should say FAILED: %q", s)
	}
}

func TestFormatToolStatusUnknown(t *testing.T) {
	s := FormatToolStatus("nonexistent", "", 0, false)
	if !strings.Contains(s, "🔧") {
		t.Fatalf("should have wrench emoji: %q", s)
	}
}

func TestFormatToolStatusLongSubject(t *testing.T) {
	long := strings.Repeat("x", 50)
	s := FormatToolStatus("read_file", long, 100, false)
	if strings.Contains(s, long) {
		t.Fatal("subject should be truncated")
	}
	if !strings.Contains(s, "...") {
		t.Fatal("truncated subject should end with ...")
	}
}

func TestToolEmojiCoverage(t *testing.T) {
	tools := []string{"bash", "read_file", "write_file", "edit_file", "grep", "web_fetch", "web_search",
		"apply_patch", "list_jobs", "cronjob", "task", "complete_step", "ls", "glob", "move_file"}
	for _, name := range tools {
		if ToolEmoji[name] == "" {
			t.Errorf("missing emoji for %q", name)
		}
		if ToolVerb[name] == "" {
			t.Errorf("missing verb for %q", name)
		}
	}
}
