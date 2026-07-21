package control

import (
	"fmt"
	"path/filepath"
	"time"

	"reames-agent/internal/agent"
)

// RecoverySessionDefaultName is the stable display name for an automatic
// transcript recovery branch.
const RecoverySessionDefaultName = agent.RecoveryBranchDefaultName

// SessionMeta is the transport-stable projection of a session sidecar. It
// contains the ownership, profile, recovery, and activity fields needed by
// frontends without exposing the agent persistence type.
type SessionMeta struct {
	Name             string
	ParentID         string
	UpdatedAt        time.Time
	Scope            string
	WorkspaceRoot    string
	TopicID          string
	TopicTitle       string
	CustomTitle      string
	Model            string
	TokenMode        string
	Mode             string
	ToolApprovalMode string
	Goal             string
	Recovered        bool
	RecoveryReason   string
	RecoveryDigest   string
}

func (m SessionMeta) DefaultScope() string {
	if m.Scope == "project" {
		return "project"
	}
	return "global"
}

func sessionMetaFromAgent(meta agent.BranchMeta) SessionMeta {
	return SessionMeta{
		Name:             meta.Name,
		ParentID:         meta.ParentID,
		UpdatedAt:        meta.UpdatedAt,
		Scope:            meta.Scope,
		WorkspaceRoot:    meta.WorkspaceRoot,
		TopicID:          meta.TopicID,
		TopicTitle:       meta.TopicTitle,
		CustomTitle:      meta.CustomTitle,
		Model:            meta.Model,
		TokenMode:        meta.TokenMode,
		Mode:             meta.Mode,
		ToolApprovalMode: meta.ToolApprovalMode,
		Goal:             meta.Goal,
		Recovered:        meta.Recovered,
		RecoveryReason:   meta.RecoveryReason,
		RecoveryDigest:   meta.RecoveryDigest,
	}
}

func applySessionMetaToAgent(meta SessionMeta, target *agent.BranchMeta) {
	if target == nil {
		return
	}
	target.Name = meta.Name
	target.ParentID = meta.ParentID
	target.UpdatedAt = meta.UpdatedAt
	target.Scope = meta.Scope
	target.WorkspaceRoot = meta.WorkspaceRoot
	target.TopicID = meta.TopicID
	target.TopicTitle = meta.TopicTitle
	target.CustomTitle = meta.CustomTitle
	target.Model = meta.Model
	target.TokenMode = meta.TokenMode
	target.Mode = meta.Mode
	target.ToolApprovalMode = meta.ToolApprovalMode
	target.Goal = meta.Goal
	target.Recovered = meta.Recovered
	target.RecoveryReason = meta.RecoveryReason
	target.RecoveryDigest = meta.RecoveryDigest
}

func agentBranchMetaFromSession(meta SessionMeta) agent.BranchMeta {
	var out agent.BranchMeta
	applySessionMetaToAgent(meta, &out)
	return out
}

// LoadSessionMeta returns a stable snapshot of the session sidecar.
func LoadSessionMeta(path string) (SessionMeta, bool, error) {
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		return SessionMeta{}, ok, err
	}
	return sessionMetaFromAgent(meta), true, nil
}

// UpdateSessionMeta performs an atomic read-modify-write under the store's
// per-session metadata lock. Unprojected persistence fields are retained.
// touchActivity selects normal save semantics; false preserves UpdatedAt.
func UpdateSessionMeta(path string, touchActivity bool, mutate func(*SessionMeta) error) error {
	if mutate == nil {
		return nil
	}
	unlock := agent.LockSessionMetaPath(path)
	defer unlock()
	raw, err := agent.EnsureBranchMeta(path)
	if err != nil {
		return err
	}
	meta := sessionMetaFromAgent(raw)
	if err := mutate(&meta); err != nil {
		return err
	}
	applySessionMetaToAgent(meta, &raw)
	if touchActivity {
		return agent.SaveBranchMeta(path, raw)
	}
	return agent.SaveBranchMetaPreserveUpdated(path, raw)
}

// SessionInfo is the transport-stable session summary used by resume pickers.
// It intentionally exposes only presentation and identity fields; transports
// must not depend on the agent persistence model.
type SessionInfo struct {
	Path           string
	CreatedAt      time.Time
	LastActivityAt time.Time
	ModTime        time.Time
	Preview        string
	Turns          int
	Scope          string
	WorkspaceRoot  string
	TopicID        string
	TopicTitle     string
	CustomTitle    string
	Recovered      bool
	RecoveryReason string
	RecoveryDigest string
	ParentID       string
}

// ListSessions returns saved sessions in the agent store's canonical order as
// stable control-layer DTOs.
func ListSessions(dir string) ([]SessionInfo, error) {
	sessions, err := agent.ListSessions(dir)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	out := make([]SessionInfo, len(sessions))
	for i, session := range sessions {
		out[i] = SessionInfo{
			Path:           session.Path,
			CreatedAt:      session.CreatedAt,
			LastActivityAt: session.LastActivityAt,
			ModTime:        session.ModTime,
			Preview:        session.Preview,
			Turns:          session.Turns,
			Scope:          session.Scope,
			WorkspaceRoot:  session.WorkspaceRoot,
			TopicID:        session.TopicID,
			TopicTitle:     session.TopicTitle,
			CustomTitle:    session.CustomTitle,
			Recovered:      session.Recovered,
			RecoveryReason: session.RecoveryReason,
			RecoveryDigest: session.RecoveryDigest,
			ParentID:       session.ParentID,
		}
	}
	return out, nil
}

// LoadSessionInfo returns the canonical event-log-aware summary for one exact
// transcript path, including transcripts temporarily stored in an archive
// bundle directory.
func LoadSessionInfo(path string) (SessionInfo, error) {
	infos, err := ListSessions(filepath.Dir(path))
	if err != nil {
		return SessionInfo{}, err
	}
	want := CanonicalSessionPath(path)
	for _, info := range infos {
		if CanonicalSessionPath(info.Path) == want {
			return info, nil
		}
	}
	return SessionInfo{}, fmt.Errorf("session not found: %s", filepath.Base(path))
}

// SessionOrderInfo is the stable metadata-only ordering record used by prompt
// history and Desktop indexes. It does not decode transcript bodies.
type SessionOrderInfo struct {
	Path           string
	CreatedAt      time.Time
	LastActivityAt time.Time
	ModTime        time.Time
	Scope          string
	WorkspaceRoot  string
	TopicID        string
	TopicTitle     string
	CustomTitle    string
	Recovered      bool
	RecoveryReason string
	RecoveryDigest string
	ParentID       string
	Turns          int
	Preview        string
	SchemaVersion  int
}

func ListSessionOrder(dir string) ([]SessionOrderInfo, error) {
	sessions, err := agent.ListSessionOrder(dir)
	if err != nil {
		return nil, fmt.Errorf("list session order: %w", err)
	}
	out := make([]SessionOrderInfo, len(sessions))
	for i, session := range sessions {
		out[i] = SessionOrderInfo{
			Path:           session.Path,
			CreatedAt:      session.CreatedAt,
			LastActivityAt: session.LastActivityAt,
			ModTime:        session.ModTime,
			Scope:          session.Scope,
			WorkspaceRoot:  session.WorkspaceRoot,
			TopicID:        session.TopicID,
			TopicTitle:     session.TopicTitle,
			CustomTitle:    session.CustomTitle,
			Recovered:      session.Recovered,
			RecoveryReason: session.RecoveryReason,
			RecoveryDigest: session.RecoveryDigest,
			ParentID:       session.ParentID,
			Turns:          session.Turns,
			Preview:        session.Preview,
			SchemaVersion:  session.SchemaVersion,
		}
	}
	return out, nil
}

// SessionUserMessage is an event-log-aware user prompt with its best-known
// persisted timestamp. Zero At means the caller should apply its own fallback.
type SessionUserMessage struct {
	Text string
	At   time.Time
}

func LoadSessionUserMessages(path string) ([]SessionUserMessage, error) {
	messages, err := agent.LoadSessionUserMessages(path)
	if err != nil {
		return nil, err
	}
	out := make([]SessionUserMessage, len(messages))
	for i, message := range messages {
		out[i] = SessionUserMessage{Text: message.Text, At: message.At}
	}
	return out, nil
}

// SessionUserTask removes coordinator handoff framing from a persisted user
// message while preserving ordinary input verbatim.
func SessionUserTask(text string) string { return agent.HandoffTask(text) }

// SessionPreview returns the canonical first-user preview and turn count.
func SessionPreview(path string) (string, int) { return agent.SessionPreview(path) }

// SessionModel returns the model reference stored with a session.
func SessionModel(path string) (string, bool) { return agent.LoadSessionModel(path) }

// SessionUpdatedAt returns the branch sidecar activity time when available.
func SessionUpdatedAt(path string) (time.Time, bool) {
	meta, ok, err := LoadSessionMeta(path)
	if err != nil || !ok || meta.UpdatedAt.IsZero() {
		return time.Time{}, false
	}
	return meta.UpdatedAt, true
}

// SessionTopicBinding is the transport-safe ownership metadata attached to a
// newly forked or rebound Desktop transcript.
type SessionTopicBinding struct {
	Scope         string
	WorkspaceRoot string
	TopicID       string
	TopicTitle    string
}

func SetSessionTopicBinding(path string, binding SessionTopicBinding) error {
	return UpdateSessionMeta(path, true, func(meta *SessionMeta) error {
		meta.Scope = binding.Scope
		meta.WorkspaceRoot = binding.WorkspaceRoot
		meta.TopicID = binding.TopicID
		meta.TopicTitle = binding.TopicTitle
		return nil
	})
}

// RenameSession updates the display title without exposing the persistence
// sidecar model to a transport.
func RenameSession(path, title string) error {
	return agent.RenameSession(path, title)
}

// ResumeSessionPath loads a persisted session behind the control boundary and
// makes it active. beforeResume runs only after the target loads successfully,
// allowing a transport to snapshot its outgoing session and atomically move
// its writer lease without exposing the loaded agent session.
func (c *Controller) ResumeSessionPath(path string, beforeResume func() error) error {
	session, err := agent.LoadSession(path)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if beforeResume != nil {
		if err := beforeResume(); err != nil {
			return err
		}
	}
	c.Resume(session, path)
	return nil
}
