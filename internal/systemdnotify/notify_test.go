package systemdnotify

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func testEnv(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestDisabledClientIsNoOp(t *testing.T) {
	c := New(testEnv(nil), 42, nil)
	if c.Enabled() || c.WatchdogEnabled() || c.WatchdogCadence() != 0 {
		t.Fatalf("disabled client = %+v", c)
	}
	if sent, err := c.Ready("ready"); err != nil || sent {
		t.Fatalf("Ready = %t, %v, want false nil", sent, err)
	}
}

func TestWatchdogEnvironmentAndPID(t *testing.T) {
	env := map[string]string{
		"NOTIFY_SOCKET": "/tmp/reames-notify",
		"WATCHDOG_USEC": "2000000",
		"WATCHDOG_PID":  "42",
	}
	c := New(testEnv(env), 42, func(_, _ string) error { return nil })
	if !c.Enabled() || !c.WatchdogEnabled() || c.WatchdogCadence() != time.Second {
		t.Fatalf("watchdog client = enabled:%t watchdog:%t cadence:%s", c.Enabled(), c.WatchdogEnabled(), c.WatchdogCadence())
	}

	env["WATCHDOG_PID"] = "41"
	c = New(testEnv(env), 42, func(_, _ string) error { return nil })
	if !c.Enabled() || c.WatchdogEnabled() {
		t.Fatalf("mismatched WATCHDOG_PID = enabled:%t watchdog:%t", c.Enabled(), c.WatchdogEnabled())
	}
}

func TestInvalidWatchdogValuesDisableFeeding(t *testing.T) {
	for _, raw := range []string{"", "0", "-1", "not-a-number", "18446744073709551615"} {
		t.Run(raw, func(t *testing.T) {
			c := New(testEnv(map[string]string{
				"NOTIFY_SOCKET": "/tmp/reames-notify",
				"WATCHDOG_USEC": raw,
			}), 42, func(_, _ string) error { return nil })
			if !c.Enabled() || c.WatchdogEnabled() {
				t.Fatalf("value %q = enabled:%t watchdog:%t", raw, c.Enabled(), c.WatchdogEnabled())
			}
		})
	}
}

func TestLifecycleMessagesAreBoundedAndSanitized(t *testing.T) {
	var socket string
	var messages []string
	c := New(testEnv(map[string]string{
		"NOTIFY_SOCKET": "@reames-notify",
		"WATCHDOG_USEC": "1000000",
	}), 42, func(gotSocket, message string) error {
		socket = gotSocket
		messages = append(messages, message)
		return nil
	})
	if _, err := c.Ready("Gateway\nrunning"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Watchdog("1 adapter\r\nhealthy"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Stopping(""); err != nil {
		t.Fatal(err)
	}
	if socket != "@reames-notify" {
		t.Fatalf("socket = %q", socket)
	}
	want := []string{
		"READY=1\nSTATUS=Gateway running",
		"WATCHDOG=1\nSTATUS=1 adapter healthy",
		"STOPPING=1\nSTATUS=Gateway stopping",
	}
	if strings.Join(messages, "|") != strings.Join(want, "|") {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
}

func TestNotifyReportsTransportFailure(t *testing.T) {
	want := errors.New("socket unavailable")
	c := New(testEnv(map[string]string{"NOTIFY_SOCKET": "/tmp/reames-notify"}), 42, func(_, _ string) error { return want })
	if sent, err := c.Status("starting"); sent || !errors.Is(err, want) {
		t.Fatalf("Status = %t, %v", sent, err)
	}
	if sent, err := c.Notify("bad\x00field"); sent || err == nil {
		t.Fatalf("NUL Notify = %t, %v", sent, err)
	}
}
