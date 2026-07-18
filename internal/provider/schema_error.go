package provider

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"reames-agent/internal/mcpname"
)

var providerToolIndexPattern = regexp.MustCompile(`(?i)\btool\s+(\d+)\s+function\b`)

// AnnotateToolSchemaError resolves provider messages such as "Tool 12 function
// has invalid parameters schema" back to the stable Reames/MCP tool identity.
func AnnotateToolSchemaError(err error, tools []ToolSchema) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || (apiErr.Status != 400 && apiErr.Status != 422) {
		return err
	}
	match := providerToolIndexPattern.FindStringSubmatch(apiErr.Body)
	if len(match) != 2 {
		return err
	}
	index, parseErr := strconv.Atoi(match[1])
	if parseErr != nil || index < 0 || index >= len(tools) {
		return err
	}

	tool := tools[index]
	context := fmt.Sprintf("Provider tool %d maps to Reames Agent tool %q.", index, tool.Name)
	if server, rawName, ok := splitMCPToolName(tool.Name); ok {
		context = fmt.Sprintf("Provider tool %d maps to Reames Agent tool %q (MCP server %q, tool %q).", index, tool.Name, server, rawName)
	}
	annotated := *apiErr
	annotated.ToolContext = context
	return &annotated
}

func splitMCPToolName(name string) (server, tool string, ok bool) {
	return mcpname.Split(name)
}
