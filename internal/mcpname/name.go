// Package mcpname owns the stable model-visible naming contract for MCP tools.
// It is intentionally independent from the executable tool registry so
// transports and capability projections can parse names without importing
// runtime tool types.
package mcpname

import "strings"

// Prefix is the namespace every MCP tool name carries.
const Prefix = "mcp__"

// Split parses "mcp__<server>__<tool>" into its server and tool parts. ok is
// false for built-in names and malformed MCP names missing either part.
func Split(name string) (server, tool string, ok bool) {
	if !strings.HasPrefix(name, Prefix) {
		return "", "", false
	}
	rest := name[len(Prefix):]
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
