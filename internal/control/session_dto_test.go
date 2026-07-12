package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/provider"
)

func TestUpdateSessionMetaPreservesPersistenceState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	updatedAt := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	startedAt := updatedAt.Add(-time.Minute)
	if err := agent.SaveBranchMetaPreserveUpdated(path, agent.BranchMeta{
		ID:               "session",
		ForkTurn:         3,
		ForkMessageIndex: 7,
		CreatedAt:        updatedAt.Add(-time.Hour),
		UpdatedAt:        updatedAt,
		Scope:            "global",
		RecoveryDepth:    2,
		Revision:         9,
		ContentDigest:    "digest",
		WriterID:         "writer",
		SchemaVersion:    1,
		Turns:            5,
		Preview:          "preview",
		InFlightTurn: &agent.InFlightTurnMeta{
			StartMessageIndex: 4,
			PreserveUser:      true,
			StartedAt:         startedAt,
		},
	}); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	if err := UpdateSessionMeta(path, false, func(meta *SessionMeta) error {
		meta.Scope = "project"
		meta.WorkspaceRoot = "C:/work"
		meta.TopicID = "topic"
		meta.TokenMode = "economy"
		return nil
	}); err != nil {
		t.Fatalf("UpdateSessionMeta: %v", err)
	}
	raw, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta = %+v, %v, %v", raw, ok, err)
	}
	if raw.Scope != "project" || raw.WorkspaceRoot != "C:/work" || raw.TopicID != "topic" || raw.TokenMode != "economy" {
		t.Fatalf("updated meta = %+v", raw)
	}
	if !raw.UpdatedAt.Equal(updatedAt) || raw.Revision != 9 || raw.ContentDigest != "digest" || raw.WriterID != "writer" {
		t.Fatalf("persistence state changed = %+v", raw)
	}
	if raw.ID != "session" || raw.ForkTurn != 3 || raw.ForkMessageIndex != 7 || raw.RecoveryDepth != 2 || raw.SchemaVersion != 1 || raw.Turns != 5 || raw.Preview != "preview" {
		t.Fatalf("hidden metadata changed = %+v", raw)
	}
	if raw.InFlightTurn == nil || raw.InFlightTurn.StartMessageIndex != 4 || !raw.InFlightTurn.PreserveUser || !raw.InFlightTurn.StartedAt.Equal(startedAt) {
		t.Fatalf("in-flight marker changed = %+v", raw.InFlightTurn)
	}

	wantErr := errors.New("reject mutation")
	if err := UpdateSessionMeta(path, false, func(meta *SessionMeta) error {
		meta.TopicID = "should-not-save"
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("mutation error = %v", err)
	}
	meta, ok, err := LoadSessionMeta(path)
	if err != nil || !ok || meta.TopicID != "topic" || meta.DefaultScope() != "project" {
		t.Fatalf("stable meta after rejected mutation = %+v, %v, %v", meta, ok, err)
	}
}

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
	if err := SetSessionTopicBinding(path, SessionTopicBinding{
		Scope: "project", WorkspaceRoot: dir, TopicID: "parser", TopicTitle: "Parser work",
	}); err != nil {
		t.Fatalf("SetSessionTopicBinding: %v", err)
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
	if got.CreatedAt.IsZero() || got.ModTime.IsZero() || got.LastActivityAt.IsZero() {
		t.Fatal("stable session summary lost activity time")
	}
	if got.Scope != "project" || got.WorkspaceRoot != dir || got.TopicID != "parser" || got.TopicTitle != "Parser work" {
		t.Fatalf("stable session ownership = %+v", got)
	}
	if updatedAt, ok := SessionUpdatedAt(path); !ok || updatedAt.IsZero() {
		t.Fatalf("SessionUpdatedAt = %v, %v", updatedAt, ok)
	}
	ordered, err := ListSessionOrder(dir)
	if err != nil || len(ordered) != 1 || ordered[0].Path != path || ordered[0].TopicID != "parser" {
		t.Fatalf("ListSessionOrder = %+v, %v", ordered, err)
	}
	users, err := LoadSessionUserMessages(path)
	if err != nil || len(users) != 1 || users[0].Text != "inspect the parser" {
		t.Fatalf("LoadSessionUserMessages = %+v, %v", users, err)
	}
}

func TestSessionIdentityHelpersStayBehindControlBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "branch-123.jsonl")
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "task"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := BranchID(path); got != "branch-123" {
		t.Fatalf("BranchID = %q", got)
	}
	if err := RenameSession(path, "Control title"); err != nil {
		t.Fatal(err)
	}
	sessions, err := ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].CustomTitle != "Control title" {
		t.Fatalf("renamed sessions = %+v", sessions)
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
