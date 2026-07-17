// Command reames-agent-guard is the credential-free desktop recovery launcher.
package main

import (
	"os"

	"reames-agent/internal/guardcmd"
)

var version = "dev"

func main() {
	os.Exit(guardcmd.Run(os.Args[1:], version, os.Stdin, os.Stdout, os.Stderr))
}
