//go:build !linux

package main

import (
	"os"
	"testing"
)

func TestConfigureWebKitRendererRecoveryIsNoop(t *testing.T) {
	const key = "WEBKIT_DISABLE_DMABUF_RENDERER"
	restoreWebKitEnv(t, key)

	configureWebKitRendererRecovery(true)
	if value, exists := os.LookupEnv(key); exists {
		t.Fatalf("non-Linux recovery unexpectedly set %s=%q", key, value)
	}
}
