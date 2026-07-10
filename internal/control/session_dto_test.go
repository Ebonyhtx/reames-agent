package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/provider"
)

func TestListSessionsReturnsStableSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "saved.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "inspect the parser"})
	if err := session.Save(path); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := agent.RenameSession(path, "Parser audit"); err != nil {
		t.Fatalf("rename session: %v", err)
	}

	sessions, err := ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions)
	}
	got := sessions[0]
	if got.Path != path || got.Turns != 1 || got.Preview != "inspect the parser" || got.CustomTitle != "Parser audit" {
		t.Fatalf("stable session summary = %+v", got)
	}
	if got.ModTime.IsZero() {
		t.Fatal("stable session summary lost modification time")
	}
}

func TestResumeSessionPathLoadsBeforeBinding(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	targetPath := filepath.Join(dir, "target.jsonl")
	target := agent.NewSession("system")
	target.Add(provider.Message{Role: provider.RoleUser, Content: "resumed task"})
	if err := target.Save(targetPath); err != nil {
		t.Fatalf("save target: %v", err)
	}

	controller := New(Options{SessionPath: oldPath, SessionDir: dir})
	bound := false
	if err := controller.ResumeSessionPath(targetPath, func() error {
		bound = true
		if controller.SessionPath() != oldPath {
			t.Fatalf("controller switched before bind callback: %q", controller.SessionPath())
		}
		return nil
	}); err != nil {
		t.Fatalf("ResumeSessionPath: %v", err)
	}
	if !bound || controller.SessionPath() != targetPath {
		t.Fatalf("bound=%v path=%q, want target", bound, controller.SessionPath())
	}
}

func TestResumeSessionPathDoesNotBindInvalidTarget(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	controller := New(Options{SessionPath: oldPath, SessionDir: dir})
	bound := false
	err := controller.ResumeSessionPath(filepath.Join(dir, "missing.jsonl"), func() error {
		bound = true
		return nil
	})
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ResumeSessionPath missing error = %v", err)
	}
	if bound || controller.SessionPath() != oldPath {
		t.Fatalf("invalid target changed state: bound=%v path=%q", bound, controller.SessionPath())
	}
}

func TestResumeSessionPathAbortsWhenBindingFails(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	targetPath := filepath.Join(dir, "target.jsonl")
	target := agent.NewSession("system")
	target.Add(provider.Message{Role: provider.RoleUser, Content: "resumed task"})
	if err := target.Save(targetPath); err != nil {
		t.Fatalf("save target: %v", err)
	}

	controller := New(Options{SessionPath: oldPath, SessionDir: dir})
	want := errors.New("lease held")
	err := controller.ResumeSessionPath(targetPath, func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("ResumeSessionPath bind error = %v", err)
	}
	if controller.SessionPath() != oldPath {
		t.Fatalf("binding failure changed path to %q", controller.SessionPath())
	}
	if !strings.Contains(err.Error(), "lease held") {
		t.Fatalf("binding error lost detail: %v", err)
	}
}
