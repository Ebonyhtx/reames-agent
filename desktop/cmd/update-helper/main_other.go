//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "reamesAgent-update-helper is only used by Windows desktop builds")
	os.Exit(2)
}
