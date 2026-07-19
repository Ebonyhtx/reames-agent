//go:build windows

package hook

import (
	"context"
	"os/exec"
	"syscall"
)

func windowsBatchCommand(ctx context.Context, command string) (*exec.Cmd, bool) {
	commandLine, ok := windowsBatchCommandLine(command)
	if !ok {
		return nil, false
	}
	cmd := exec.CommandContext(ctx, "cmd.exe")
	// cmd.exe does not use CommandLineToArgvW. Supply the exact command line so
	// Go does not backslash-escape the leading quoted batch path.
	cmd.Args = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: commandLine}
	return cmd, true
}
