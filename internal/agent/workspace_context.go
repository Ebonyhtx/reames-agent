package agent

import (
	"context"
	"strings"

	"reames-agent/internal/tool"
)

type workspaceRootContextKey struct{}
type toolRegistryContextKey struct{}

// WithWorkspaceRoot carries the physical workspace used by one agent runtime.
// Nested writer subagents use it as the source for their own worktree instead
// of accidentally falling back to the original root session workspace.
func WithWorkspaceRoot(ctx context.Context, root string) context.Context {
	root = strings.TrimSpace(root)
	if ctx == nil || root == "" {
		return ctx
	}
	return context.WithValue(ctx, workspaceRootContextKey{}, root)
}

// WorkspaceRootFromContext returns the active runtime workspace, or fallback.
func WorkspaceRootFromContext(ctx context.Context, fallback string) string {
	if ctx != nil {
		if root, _ := ctx.Value(workspaceRootContextKey{}).(string); strings.TrimSpace(root) != "" {
			return strings.TrimSpace(root)
		}
	}
	return strings.TrimSpace(fallback)
}

func withToolRegistry(ctx context.Context, registry *tool.Registry) context.Context {
	if ctx == nil || registry == nil {
		return ctx
	}
	return context.WithValue(ctx, toolRegistryContextKey{}, registry)
}

// ToolRegistryFromContext returns the actual registry of the invoking runtime.
// Nested delegation uses it to preserve the parent's reduced capability scope.
func ToolRegistryFromContext(ctx context.Context, fallback *tool.Registry) *tool.Registry {
	if ctx != nil {
		if registry, _ := ctx.Value(toolRegistryContextKey{}).(*tool.Registry); registry != nil {
			return registry
		}
	}
	return fallback
}
