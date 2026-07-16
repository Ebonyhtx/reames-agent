//go:build windows

package sandbox

import (
	"fmt"
	"os"
)

// RunChildExecHelper is not used on Windows: the native sandbox helper installs
// the child environment while creating the restricted process itself.
func RunChildExecHelper(_ []string, _ *os.File, _ *os.File, stderr *os.File) int {
	fmt.Fprintln(stderr, ChildExecHelperCommand+" is not a Windows launch path")
	return 126
}
