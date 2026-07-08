package lsp

import (
	"testing"
)

func TestDiagKey(t *testing.T) {
	k1 := diagKey{file: "a.go", line: 10, message: "unused var"}
	k2 := diagKey{file: "a.go", line: 10, message: "unused var"}
	k3 := diagKey{file: "a.go", line: 11, message: "unused var"}
	if k1 != k2 {
		t.Fatal("same keys should be equal")
	}
	if k1 == k3 {
		t.Fatal("different keys should not be equal")
	}
}

func TestBaselineInit(t *testing.T) {
	baseline.mu.Lock()
	defer baseline.mu.Unlock()
	if baseline.sets == nil {
		t.Fatal("baseline sets not initialized")
	}
}
