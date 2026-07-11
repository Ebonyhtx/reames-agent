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
	LastActivityAt time.Time
	ModTime        time.Time
	Preview        string
	Turns          int
	Scope          string
	TopicTitle     string
	CustomTitle    string
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
			LastActivityAt: session.LastActivityAt,
			ModTime:        session.ModTime,
			Preview:        session.Preview,
			Turns:          session.Turns,
			Scope:          session.Scope,
			TopicTitle:     session.TopicTitle,
			CustomTitle:    session.CustomTitle,
		}
	}
	return out, nil
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
