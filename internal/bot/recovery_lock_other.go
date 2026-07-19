//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package bot

func lockDeliveryLedgerFile(string) (func(), error) { return func() {}, nil }
