//go:build !windows

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// RunChildExecHelper replaces this trusted helper with the requested child
// after restoring its explicit environment. The helper is launched inside
// bubblewrap or Seatbelt, so syscall.Exec preserves the confinement boundary.
func RunChildExecHelper(args []string, _ *os.File, _ *os.File, stderr *os.File) int {
	if len(args) < 2 || args[0] != "--" || strings.TrimSpace(args[1]) == "" {
		fmt.Fprintln(stderr, "usage: reamesAgent "+ChildExecHelperCommand+" -- <command> [args...]")
		return 2
	}
	env, err := takeChildEnvironment(true)
	if err != nil {
		fmt.Fprintln(stderr, "sandbox child environment:", err)
		return 126
	}
	child := args[1:]
	executable, err := executableInEnvironment(child[0], env)
	if err != nil {
		fmt.Fprintln(stderr, "sandbox child command:", err)
		return 126
	}
	if err := syscall.Exec(executable, child, env); err != nil {
		fmt.Fprintln(stderr, "sandbox child exec:", err)
		return 126
	}
	return 0
}

func executableInEnvironment(command string, env []string) (string, error) {
	if strings.ContainsRune(command, filepath.Separator) {
		return command, nil
	}
	pathValue := ""
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok && key == "PATH" {
			pathValue = value
			break
		}
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, command)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%q not found on child PATH", command)
}
