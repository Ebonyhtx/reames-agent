package mcpname

import "testing"

func TestSplit(t *testing.T) {
	tests := []struct {
		name       string
		wantServer string
		wantTool   string
		wantOK     bool
	}{
		{name: "mcp__github__issue_read", wantServer: "github", wantTool: "issue_read", wantOK: true},
		{name: "mcp__server__nested__tool", wantServer: "server", wantTool: "nested__tool", wantOK: true},
		{name: "read_file"},
		{name: "mcp____tool"},
		{name: "mcp__server__"},
		{name: "mcp__server"},
	}
	for _, tt := range tests {
		server, tool, ok := Split(tt.name)
		if server != tt.wantServer || tool != tt.wantTool || ok != tt.wantOK {
			t.Errorf("Split(%q) = %q, %q, %v; want %q, %q, %v", tt.name, server, tool, ok, tt.wantServer, tt.wantTool, tt.wantOK)
		}
	}
}
