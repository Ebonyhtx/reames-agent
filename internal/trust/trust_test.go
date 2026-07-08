package trust

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"<script>alert(1)</script>hello", "hello"},
		{"<style>body{}</style>text", "text"},
		{"<b>bold</b> text", "bold text"},
		{"<a href='x'>link</a>", "link"},
		{"normal text", "normal text"},
		{"<script>hack</script><p>safe</p><style>x</style>", "safe"},
	}
	for _, tt := range tests {
		got := SanitizeHTML(tt.in)
		if got != tt.want {
			t.Errorf("SanitizeHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWrapUntrusted(t *testing.T) {
	got := WrapUntrusted("hello world", "web_search")
	if !strings.Contains(got, "UNTRUSTED_WEB_SEARCH_OUTPUT") {
		t.Errorf("missing envelope: %s", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("missing content: %s", got)
	}
}

func TestRedactSecrets(t *testing.T) {
	tests := []struct{ in, contains string }{
		{"my key is sk-abc123def45678901234567890123456", "REDACTED"},
		{"token: ghp_abcdefghijklmnopqrstuvwxyz1234567890", "REDACTED"},
		{"normal text without secrets", ""},
	}
	for _, tt := range tests {
		got := RedactSecrets(tt.in)
		if tt.contains == "" {
			if got != tt.in {
				t.Errorf("unexpected redaction: %q -> %q", tt.in, got)
			}
		} else if !strings.Contains(got, tt.contains) {
			t.Errorf("expected %q in %q", tt.contains, got)
		}
	}
}
