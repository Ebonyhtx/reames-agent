package control

import (
	"errors"

	"reames-agent/internal/agent"
)

var errSessionPendingCleanup = errors.New("session is pending cleanup")

// LoadedSession is an opaque persisted runtime transcript. Transports may test
// presence/emptiness and hand it back to control, but cannot mutate agent
// messages or borrow the persistence baseline.
type LoadedSession struct{ session *agent.Session }

func LoadSession(path string) (*LoadedSession, error) {
	if agent.IsCleanupPending(path) {
		return nil, errSessionPendingCleanup
	}
	session, err := agent.LoadSession(path)
	if err != nil {
		return nil, err
	}
	return &LoadedSession{session: session}, nil
}

func (s *LoadedSession) Empty() bool {
	return s == nil || s.session == nil || s.session.Len() == 0
}

// AdoptLoadedSessionWithCurrentSystemPrompt resumes an opaque disk transcript
// while preserving the loaded persistence baseline and legacy system-less rule.
func AdoptLoadedSessionWithCurrentSystemPrompt(controller *Controller, loaded *LoadedSession, path string) {
	if controller == nil || loaded == nil || loaded.session == nil {
		return
	}
	controller.AdoptLoadedHistoryWithCurrentSystemPrompt(loaded.session.Snapshot(), path)
}
