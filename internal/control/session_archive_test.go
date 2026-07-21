package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/provider"
	"reames-agent/internal/store"
)

func TestArchiveSessionRoundTripMovesCompleteBundle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "archive me"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{store.SessionEventLog(path), store.SessionAppServerMeta(path)} {
		if err := os.WriteFile(artifact, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(store.SessionCheckpointDir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	archived, err := ArchiveSession(dir, path)
	if err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("live transcript remains after archive: %v", err)
	}
	if _, err := os.Stat(archived.Path); err != nil {
		t.Fatalf("archived transcript: %v", err)
	}
	listed, err := ListArchivedSessions(dir)
	if err != nil || len(listed) != 1 || listed[0].Path != archived.Path {
		t.Fatalf("ListArchivedSessions = %+v, %v", listed, err)
	}
	restored, err := UnarchiveSession(dir, archived.Path)
	if err != nil || restored != path {
		t.Fatalf("UnarchiveSession = %q, %v", restored, err)
	}
	for _, artifact := range []string{path, store.SessionEventLog(path), store.SessionAppServerMeta(path), store.SessionCheckpointDir(path)} {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("restored artifact %s: %v", artifact, err)
		}
	}
}

func TestArchiveSessionRollsBackMovedArtifactsOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollback.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "keep me"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	sidecar := store.SessionEventLog(path)
	if err := os.WriteFile(sidecar, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalRename := archiveRename
	t.Cleanup(func() { archiveRename = originalRename })
	calls := 0
	archiveRename = func(src, dst string) error {
		calls++
		if calls == 2 {
			return errors.New("injected archive move failure")
		}
		return originalRename(src, dst)
	}
	if _, err := ArchiveSession(dir, path); err == nil {
		t.Fatal("ArchiveSession succeeded with injected move failure")
	}
	for _, artifact := range []string{path, sidecar} {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("rollback lost %s: %v", artifact, err)
		}
	}
	if archived, err := ListArchivedSessions(dir); err != nil || len(archived) != 0 {
		t.Fatalf("archive residue = %+v, %v", archived, err)
	}
}

func TestArchiveSessionBundleKeepsRecoveryOriginAndActiveTogether(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.jsonl")
	active := filepath.Join(dir, "active-recovery.jsonl")
	for _, path := range []string{origin, active} {
		session := agent.NewSession("system")
		session.Add(provider.Message{Role: provider.RoleUser, Content: filepath.Base(path)})
		if err := session.Save(path); err != nil {
			t.Fatal(err)
		}
	}
	archived, err := ArchiveSessionBundle(dir, active, origin)
	if err != nil {
		t.Fatalf("ArchiveSessionBundle: %v", err)
	}
	for _, path := range []string{origin, active} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("live bundle member remains %s: %v", path, err)
		}
	}
	if _, err := UnarchiveSession(dir, archived.Path); err != nil {
		t.Fatalf("UnarchiveSession: %v", err)
	}
	for _, path := range []string{origin, active} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("restored bundle member %s: %v", path, err)
		}
	}
}

func TestUnarchiveSessionRollsBackRestoredArtifactsOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unarchive-rollback.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "keep archived"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.SessionEventLog(path), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	archived, err := ArchiveSession(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	originalRename := archiveRename
	t.Cleanup(func() { archiveRename = originalRename })
	calls := 0
	archiveRename = func(src, dst string) error {
		calls++
		if calls == 2 {
			return errors.New("injected restore move failure")
		}
		return originalRename(src, dst)
	}
	if _, err := UnarchiveSession(dir, archived.Path); err == nil {
		t.Fatal("UnarchiveSession succeeded with injected move failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed restore published live transcript: %v", err)
	}
	listed, err := ListArchivedSessions(dir)
	if err != nil || len(listed) != 1 || listed[0].Path != archived.Path {
		t.Fatalf("archived bundle after rollback = %+v, %v", listed, err)
	}
}

func TestArchiveSessionRejectsSymlinkedArchiveRoot(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, sessionArchiveDir)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	path := filepath.Join(dir, "contained.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "stay contained"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveSession(dir, path); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("ArchiveSession symlink root error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, filepath.Base(path))); !os.IsNotExist(err) {
		t.Fatalf("archive escaped through symlink: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("rejected archive mutated transcript: %v", err)
	}
}
