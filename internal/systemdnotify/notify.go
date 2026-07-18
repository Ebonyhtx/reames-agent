// Package systemdnotify implements the small, optional subset of sd_notify
// used by the Reames Agent Gateway. It has no CGO or libsystemd dependency.
package systemdnotify

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type datagramSender func(socket, message string) error

// Client is an immutable projection of systemd's notification environment.
// A zero Client is disabled and every notification is a no-op.
type Client struct {
	socket   string
	watchdog time.Duration
	send     datagramSender
}

// FromEnvironment reads NOTIFY_SOCKET, WATCHDOG_USEC, and WATCHDOG_PID for the
// current process. WATCHDOG_PID affects watchdog feeding only; readiness and
// stopping notifications can still be sent when the socket is configured.
func FromEnvironment() Client {
	return New(os.Getenv, os.Getpid(), nil)
}

// New constructs a Client from an environment reader. It is exported so hosts
// and tests can project a controlled environment without mutating process state.
func New(getenv func(string) string, pid int, sender func(socket, message string) error) Client {
	if getenv == nil {
		return Client{}
	}
	socket := strings.TrimSpace(getenv("NOTIFY_SOCKET"))
	if socket == "" {
		return Client{}
	}
	if sender == nil {
		sender = sendDatagram
	}
	c := Client{socket: socket, send: sender}

	rawUsec := strings.TrimSpace(getenv("WATCHDOG_USEC"))
	if rawUsec == "" {
		return c
	}
	usec, err := strconv.ParseUint(rawUsec, 10, 64)
	if err != nil || usec == 0 || usec > uint64((1<<63-1)/int64(time.Microsecond)) {
		return c
	}
	if rawPID := strings.TrimSpace(getenv("WATCHDOG_PID")); rawPID != "" {
		watchdogPID, err := strconv.Atoi(rawPID)
		if err != nil || watchdogPID <= 0 || watchdogPID != pid {
			return c
		}
	}
	c.watchdog = time.Duration(usec) * time.Microsecond
	return c
}

// Enabled reports whether systemd supplied a notification socket.
func (c Client) Enabled() bool { return c.socket != "" && c.send != nil }

// WatchdogEnabled reports whether systemd assigned this process a watchdog.
func (c Client) WatchdogEnabled() bool { return c.Enabled() && c.watchdog > 0 }

// WatchdogCadence returns half of systemd's watchdog timeout. The small floor
// prevents a malformed external environment from creating a busy loop.
func (c Client) WatchdogCadence() time.Duration {
	if !c.WatchdogEnabled() {
		return 0
	}
	cadence := c.watchdog / 2
	if cadence < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return cadence
}

// Notify sends one datagram. Disabled clients return (false, nil).
func (c Client) Notify(message string) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}
	if message == "" {
		return false, fmt.Errorf("systemd notify message is empty")
	}
	if strings.ContainsRune(message, '\x00') {
		return false, fmt.Errorf("systemd notify message contains NUL")
	}
	if err := c.send(c.socket, message); err != nil {
		return false, err
	}
	return true, nil
}

// Ready reports successful Gateway startup.
func (c Client) Ready(status string) (bool, error) {
	return c.Notify("READY=1\nSTATUS=" + sanitizeStatus(status, "Gateway running"))
}

// Watchdog reports continued main-loop and adapter-health progress.
func (c Client) Watchdog(status string) (bool, error) {
	if !c.WatchdogEnabled() {
		return false, nil
	}
	return c.Notify("WATCHDOG=1\nSTATUS=" + sanitizeStatus(status, "Gateway running"))
}

// Status updates systemd without claiming readiness or watchdog health.
func (c Client) Status(status string) (bool, error) {
	return c.Notify("STATUS=" + sanitizeStatus(status, "Gateway status unavailable"))
}

// Stopping reports that orderly shutdown has begun.
func (c Client) Stopping(status string) (bool, error) {
	return c.Notify("STOPPING=1\nSTATUS=" + sanitizeStatus(status, "Gateway stopping"))
}

func sanitizeStatus(status, fallback string) string {
	status = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '\x00':
			return ' '
		default:
			return r
		}
	}, status)
	status = strings.Join(strings.Fields(status), " ")
	if status == "" {
		return fallback
	}
	return status
}
