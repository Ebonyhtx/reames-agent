// Package mcpname owns the stable model-visible naming contract for MCP tools.
// It is intentionally independent from the executable tool registry so
// transports and capability projections can parse names without importing
// runtime tool types.
package mcpname

import "strings"

// Prefix is the namespace every MCP tool name carries.
const Prefix = "mcp__"

// Split parses the unambiguous "mcp__<server>__<tool>" contract. The remaining
// payload must contain exactly one, non-overlapping "__" delimiter; allowing a
// second delimiter would let two different server/tool pairs share one model
// name and would make prefix removal, trust receipts, and approvals disagree.
func Split(name string) (server, tool string, ok bool) {
	if !strings.HasPrefix(name, Prefix) {
		return "", "", false
	}
	rest := name[len(Prefix):]
	boundary := -1
	for i := 0; i+1 < len(rest); i++ {
		if rest[i] != '_' || rest[i+1] != '_' {
			continue
		}
		if boundary >= 0 {
			return "", "", false
		}
		boundary = i
	}
	if boundary <= 0 || boundary+2 >= len(rest) {
		return "", "", false
	}
	return rest[:boundary], rest[boundary+2:], true
}
