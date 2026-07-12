package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"reames-agent/internal/config"
	"reames-agent/internal/fileutil"
)

var zoomFactorWriteMu sync.Mutex

// DesktopZoomFactor persists the user's WebView2 zoom factor preference across
// restarts. The frontend writes it; main.go reads it before wails.Run() to set
// the Windows ZoomFactor option.
type DesktopZoomFactor struct {
	ZoomFactor float64 `json:"zoomFactor"`
}

func zoomFactorPath() string {
	return filepath.Join(config.MemoryUserDir(), "desktop-zoom.json")
}

// loadZoomFactor reads the saved zoom factor. The bool is false when no saved
// value exists (first launch, missing file, corrupt JSON). Callers should fall
// back to 1.0 (no zoom) in that case.
func loadZoomFactor() (float64, bool) {
	path := zoomFactorPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var zf DesktopZoomFactor
	if err := json.Unmarshal(data, &zf); err != nil {
		return 0, false
	}
	if zf.ZoomFactor < 0.5 || zf.ZoomFactor > 2.0 {
		return 0, false
	}
	return zf.ZoomFactor, true
}

// GetDesktopZoomFactor returns the currently persisted restart zoom factor,
// or 1.0 if none is saved.
func (a *App) GetDesktopZoomFactor() float64 {
	zf, ok := loadZoomFactor()
	if !ok {
		return 1.0
	}
	return zf
}

// SetDesktopZoomFactor persists a zoom factor for the next launch. The value
// is clamped to [0.5, 2.0] (50% – 200%) for safety.
func (a *App) SetDesktopZoomFactor(factor float64) error {
	if math.IsNaN(factor) || math.IsInf(factor, 0) {
		return fmt.Errorf("desktop zoom factor must be finite")
	}
	if factor < 0.5 {
		factor = 0.5
	}
	if factor > 2.0 {
		factor = 2.0
	}
	data, err := json.Marshal(DesktopZoomFactor{ZoomFactor: factor})
	if err != nil {
		return fmt.Errorf("encode desktop zoom factor: %w", err)
	}
	zoomFactorWriteMu.Lock()
	defer zoomFactorWriteMu.Unlock()
	if err := fileutil.AtomicWriteFile(zoomFactorPath(), data, 0o600); err != nil {
		return fmt.Errorf("save desktop zoom factor: %w", err)
	}
	return nil
}

// RestartApplication restarts the whole process so the already-persisted
// ZoomFactor takes effect in the WebView2 window options.
func (a *App) RestartApplication() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
