//go:build linux

package systemdnotify

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxClientSendsRealUnixDatagram(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify.sock")
	receiver, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	if err := receiver.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	c := New(testEnv(map[string]string{"NOTIFY_SOCKET": path}), 42, nil)
	if sent, err := c.Ready("Gateway running"); err != nil || !sent {
		t.Fatalf("Ready = %t, %v", sent, err)
	}
	buf := make([]byte, 256)
	n, _, err := receiver.ReadFromUnix(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "READY=1\nSTATUS=Gateway running" {
		t.Fatalf("datagram = %q", got)
	}
}

func TestLinuxClientSendsAbstractUnixDatagram(t *testing.T) {
	name := fmt.Sprintf("@reames-notify-%d", time.Now().UnixNano())
	receiver, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: name, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	if err := receiver.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	c := New(testEnv(map[string]string{"NOTIFY_SOCKET": name}), 42, nil)
	if sent, err := c.Stopping("Gateway stopping"); err != nil || !sent {
		t.Fatalf("Stopping = %t, %v", sent, err)
	}
	buf := make([]byte, 256)
	n, _, err := receiver.ReadFromUnix(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "STOPPING=1\nSTATUS=Gateway stopping" {
		t.Fatalf("datagram = %q", got)
	}
}
