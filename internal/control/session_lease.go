package control

import (
	"errors"
	"os"

	"reames-agent/internal/agent"
)

// SessionLease is an opaque single-writer ownership handle for Desktop tabs.
type SessionLease struct{ lease *agent.SessionLease }

func TryAcquireSessionLease(path string) (*SessionLease, error) {
	lease, err := agent.TryAcquireSessionLease(path)
	if err != nil {
		return nil, err
	}
	return &SessionLease{lease: lease}, nil
}

func TryReclaimCurrentProcessSessionLease(path string) (*SessionLease, error) {
	lease, err := agent.TryReclaimCurrentProcessSessionLease(path)
	if err != nil {
		return nil, err
	}
	return &SessionLease{lease: lease}, nil
}

func (l *SessionLease) Path() string {
	if l == nil || l.lease == nil {
		return ""
	}
	return l.lease.Path()
}

func (l *SessionLease) Release() {
	if l != nil && l.lease != nil {
		l.lease.Release()
	}
}

// SessionLeaseReclaimCandidate reports whether a lease error has no readable
// owner or names this process. The OS lock remains the final arbiter during the
// actual reclaim; foreign readable ownership is never a candidate.
func SessionLeaseReclaimCandidate(err error) bool {
	if !errors.Is(err, agent.ErrSessionLeaseHeld) {
		return false
	}
	var leaseErr *agent.SessionLeaseError
	if !errors.As(err, &leaseErr) || leaseErr == nil {
		return false
	}
	if leaseErr.Info == nil {
		return true
	}
	return leaseErr.Info.PID == os.Getpid() && leaseErr.Info.WriterID == agent.SessionWriterID()
}
