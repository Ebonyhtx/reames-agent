//go:build windows

package fileutil

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRenameCrossesDeviceClassifiesFilterDriverError(t *testing.T) {
	notSameDevice := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: windows.ERROR_NOT_SAME_DEVICE}
	if !renameCrossesDevice(notSameDevice) {
		t.Fatal("ERROR_NOT_SAME_DEVICE must be classified as cross-device")
	}
	sharing := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: windows.ERROR_SHARING_VIOLATION}
	if renameCrossesDevice(sharing) {
		t.Fatal("a sharing violation is transient, not cross-device")
	}
	accessDenied := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: windows.ERROR_ACCESS_DENIED}
	if renameCrossesDevice(accessDenied) {
		t.Fatal("access denied is transient (AV/indexer), not cross-device")
	}
}
