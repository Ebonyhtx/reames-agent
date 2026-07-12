package control

import (
	"fmt"
	"time"

	"reames-agent/internal/agent"
)

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

// SessionUpdatedAt returns the branch sidecar activity time when available.
func SessionUpdatedAt(path string) (time.Time, bool) {
	meta, ok, err := agent.LoadBranchMeta(path)
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
	meta, err := agent.EnsureBranchMeta(path)
	if err != nil {
		return err
	}
	meta.Scope = binding.Scope
	meta.WorkspaceRoot = binding.WorkspaceRoot
	meta.TopicID = binding.TopicID
	meta.TopicTitle = binding.TopicTitle
	return agent.SaveBranchMeta(path, meta)
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
