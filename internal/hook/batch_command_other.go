//go:build !windows

package hook

import (
	"context"
	"os/exec"
)

func windowsBatchCommand(context.Context, string) (*exec.Cmd, bool) {
	return nil, false
}
