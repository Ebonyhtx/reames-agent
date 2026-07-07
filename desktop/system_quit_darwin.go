//go:build darwin

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void installReamesAgentSystemQuitHook(void);
*/
import "C"

import "sync"

var installSystemQuitHookOnce sync.Once

func installSystemQuitHook() {
	installSystemQuitHookOnce.Do(func() {
		C.installReamesAgentSystemQuitHook()
	})
}

//export ReamesAgentMarkSystemQuit
func ReamesAgentMarkSystemQuit() {
	markSystemQuitRequested()
}
