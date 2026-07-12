// Command reamesAgent is a config- and plugin-driven coding agent CLI.
package main

import (
	"os"

	"reames-agent/internal/cli"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], version))
}
