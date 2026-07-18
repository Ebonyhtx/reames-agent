//go:build !linux

package systemdnotify

import "fmt"

func sendDatagram(_, _ string) error {
	return fmt.Errorf("systemd notification sockets are unavailable on this platform")
}
