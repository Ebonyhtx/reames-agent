package builtin

import (
	"testing"
)

func TestWebSearchSchema(t *testing.T) {
	ws := webSearch{}
	if ws.Name() != "web_search" {
		t.Fatalf("name: %s", ws.Name())
	}
	if !ws.ReadOnly() {
		t.Fatal("web_search should be read-only")
	}
	schema := string(ws.Schema())
	if schema == "" {
		t.Fatal("empty schema")
	}
}

func TestParseDuckDuckGoEmpty(t *testing.T) {
	r := parseDuckDuckGo("", 5)
	if len(r) != 0 {
		t.Fatalf("expected empty, got %d", len(r))
	}
}

func TestParseDuckDuckGoGarbage(t *testing.T) {
	r := parseDuckDuckGo("<html><body>no results here</body></html>", 5)
	if len(r) != 0 {
		t.Fatalf("expected empty from garbage html, got %d", len(r))
	}
}

func TestCleanHTML(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"<b>bold</b>", "bold"},
		{"<a href='x'>link</a>", "link"},
		{"hello &amp; world", "hello & world"},
	}
	for _, tt := range tests {
		got := cleanHTML(tt.in)
		if got != tt.want {
			t.Errorf("cleanHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatResults(t *testing.T) {
	r := []searchResult{
		{Title: "Test", URL: "https://example.com", Snippet: "A test result"},
	}
	out := formatResults(r)
	if out == "" {
		t.Fatal("empty output")
	}
}
