//go:build !linux && !darwin && !windows

package homebackup

import "errors"

func renameNoReplace(_, _ string) error {
	return errors.New("atomic no-replace restore is unsupported on this platform")
}
