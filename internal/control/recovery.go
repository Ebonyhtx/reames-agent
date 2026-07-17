package control

import (
	"os"

	"reames-agent/internal/repair"
)

// RecoveryStatus returns the same offline report consumed by Guard. It performs
// no Provider, MCP, plugin-host, Hook, or Agent work.
func (c *Controller) RecoveryStatus() (repair.Report, error) {
	executable, _ := os.Executable()
	root := ""
	if c != nil {
		root = c.WorkspaceRoot()
	}
	return repair.Inspect(repair.InspectOptions{Root: root, ExecutablePath: executable})
}
