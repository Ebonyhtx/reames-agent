//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package repair

func lockRepairStateFile(string) (func(), error) { return func() {}, nil }
