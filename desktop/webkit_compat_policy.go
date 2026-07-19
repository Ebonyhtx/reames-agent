package main

import "os"

// configureWebKitRendererRecoveryForGPU keeps the Safe Mode decision separate
// from Linux GPU detection so the policy remains cross-platform testable.
func configureWebKitRendererRecoveryForGPU(safeMode, nvidiaGPU bool) {
	if !safeMode || !nvidiaGPU {
		return
	}
	if _, explicitlySet := os.LookupEnv("WEBKIT_DISABLE_DMABUF_RENDERER"); explicitlySet {
		return
	}
	_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
}
