package plugin

import (
	"encoding/json"
	"sort"
	"strings"

	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

// sortToolsByName returns a new slice of tools sorted alphabetically by Name().
func sortToolsByName(tools []tool.Tool) []tool.Tool {
	sorted := make([]tool.Tool, len(tools))
	copy(sorted, tools)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name() < sorted[j].Name()
	})
	return sorted
}

// canonicalizeSchema recursively stabilizes a JSON Schema so the same logical
// schema always produces the same byte representation — important for cache
// fingerprint stability across MCP sessions.
func canonicalizeSchema(raw json.RawMessage) json.RawMessage {
	return provider.CanonicalizeSchema(raw)
}

func normalizeAndValidateToolSchema(raw json.RawMessage) (json.RawMessage, error) {
	schema := canonicalizeSchema(raw)
	if err := provider.ValidateToolSchema(schema); err != nil {
		return nil, err
	}
	return schema, nil
}

func schemaValidationError(err error) string {
	const maxRunes = 512
	msg := strings.TrimSpace(err.Error())
	runes := []rune(msg)
	if len(runes) > maxRunes {
		msg = string(runes[:maxRunes]) + "..."
	}
	return "invalid input schema: " + msg
}
