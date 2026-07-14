package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reames-agent/internal/diff"
	"reames-agent/internal/tool"
)

func init() { tool.RegisterBuiltin(moveFile{}) }

var renameFile = os.Rename

// moveFile moves or renames one file. roots, when non-empty, confine both the
// source and destination to the workspace; guard rejects Reames Agent session-data
// endpoints on either side (a move out of the store mutates it too); workDir
// resolves relative paths.
type moveFile struct {
	roots   []string
	guard   SessionDataGuard
	workDir string
}

func (moveFile) Name() string { return "move_file" }

func (moveFile) Description() string {
	return "Move or rename a file from source_path to destination_path. Creates the destination parent directory as needed. Use instead of shell mv, Move-Item, or ren for file moves so workspace confinement and file-edit permissions apply."
}

func (moveFile) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"source_path":{"type":"string","description":"Existing file path to move"},"destination_path":{"type":"string","description":"Destination file path; must not already exist"}},"required":["source_path","destination_path"]}`)
}

func (moveFile) ReadOnly() bool { return false }

type moveFileArgs struct {
	SourcePath      string `json:"source_path"`
	DestinationPath string `json:"destination_path"`
}

func (m moveFile) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	p, err := parseMoveFileArgs(args)
	if err != nil {
		return "", err
	}
	src, dst, err := m.openTargets(p)
	if err != nil {
		return "", err
	}
	defer src.Close()
	defer dst.Close()
	info, err := src.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", src.path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory; move_file only moves files", src.path)
	}
	if filepath.Clean(src.path) == filepath.Clean(dst.path) {
		return fmt.Sprintf("%s is already at %s; no changes made", src.path, dst.path), nil
	}
	sameFileDestination := false
	if dstInfo, err := dst.Stat(); err == nil {
		if !os.SameFile(info, dstInfo) {
			return "", fmt.Errorf("destination %s already exists", dst.path)
		}
		sameFileDestination = true
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", dst.path, err)
	}
	if err := dst.MkdirParent(0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(dst.path), err)
	}
	sameRoot := (src.root == nil && dst.root == nil) || (src.root != nil && dst.root != nil && sameRootPath(src.rootPath, dst.rootPath))
	if !sameRoot {
		if err := copyRootedFileAndRemoveSource(src, dst, info); err != nil {
			return "", fmt.Errorf("move %s to %s: %w", src.path, dst.path, err)
		}
		return fmt.Sprintf("moved %s to %s", src.path, dst.path), nil
	}
	if err := src.renameTo(dst); err != nil {
		if sameFileDestination {
			if rerr := renameSameRootedFileDestination(src, dst); rerr != nil {
				return "", fmt.Errorf("move %s to %s: %w", src.path, dst.path, rerr)
			}
			return fmt.Sprintf("moved %s to %s", src.path, dst.path), nil
		}
		if isCrossDeviceMove(err) {
			if cerr := copyRootedFileAndRemoveSource(src, dst, info); cerr != nil {
				return "", fmt.Errorf("move %s to %s: %w", src.path, dst.path, cerr)
			}
			return fmt.Sprintf("moved %s to %s", src.path, dst.path), nil
		}
		return "", fmt.Errorf("move %s to %s: %w", src.path, dst.path, err)
	}
	return fmt.Sprintf("moved %s to %s", src.path, dst.path), nil
}

func parseMoveFileArgs(args json.RawMessage) (moveFileArgs, error) {
	var p moveFileArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return p, fmt.Errorf("invalid args: %w", err)
	}
	if p.SourcePath == "" {
		return p, fmt.Errorf("source_path is required")
	}
	if p.DestinationPath == "" {
		return p, fmt.Errorf("destination_path is required")
	}
	return p, nil
}

func (m moveFile) openTargets(p moveFileArgs) (*rootedTarget, *rootedTarget, error) {
	src, err := openRootedWriteTarget(m.roots, m.guard, resolveIn(m.workDir, p.SourcePath))
	if err != nil {
		return nil, nil, err
	}
	dst, err := openRootedWriteTarget(m.roots, m.guard, resolveIn(m.workDir, p.DestinationPath))
	if err != nil {
		src.Close()
		return nil, nil, err
	}
	return src, dst, nil
}

// PreviewChanges describes both sides of a move so the agent snapshots the
// source bytes and the destination's prior non-existence before execution.
func (m moveFile) PreviewChanges(args json.RawMessage) ([]diff.Change, error) {
	p, err := parseMoveFileArgs(args)
	if err != nil {
		return nil, err
	}
	src, dst, err := m.openTargets(p)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	defer dst.Close()
	info, err := src.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", src.path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", src.path)
	}
	if filepath.Clean(src.path) == filepath.Clean(dst.path) {
		return nil, nil
	}
	if dstInfo, statErr := dst.Stat(); statErr == nil {
		if !os.SameFile(info, dstInfo) {
			return nil, fmt.Errorf("destination %s already exists", dst.path)
		}
		content, _, readErr := src.readEncoded()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", src.path, readErr)
		}
		return []diff.Change{diff.Build(src.path, content, "", diff.Delete)}, nil
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("stat %s: %w", dst.path, statErr)
	}
	content, _, err := src.readEncoded()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", src.path, err)
	}
	return []diff.Change{
		diff.Build(src.path, content, "", diff.Delete),
		diff.Build(dst.path, "", content, diff.Create),
	}, nil
}

func renameSameRootedFileDestination(src, dst *rootedTarget) error {
	if src.root == nil || dst.root == nil {
		return renameSameFileDestination(src.path, dst.path)
	}
	dir := filepath.Dir(src.rel)
	var tmpName string
	for attempt := range 100 {
		candidate := filepath.Join(dir, fmt.Sprintf(".reamesAgent-move-%d-%d", time.Now().UnixNano(), attempt))
		f, err := src.root.OpenFile(candidate, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			_ = src.root.Remove(candidate)
			return err
		}
		if err := src.root.Remove(candidate); err != nil {
			return err
		}
		tmpName = candidate
		break
	}
	if tmpName == "" {
		return fmt.Errorf("could not allocate move temporary path")
	}
	if err := src.root.Rename(src.rel, tmpName); err != nil {
		return err
	}
	if err := src.root.Remove(dst.rel); err != nil && !os.IsNotExist(err) {
		if restoreErr := src.root.Rename(tmpName, src.rel); restoreErr != nil {
			return fmt.Errorf("%w; restore %s: %v", err, src.path, restoreErr)
		}
		return err
	}
	if err := src.root.Rename(tmpName, dst.rel); err != nil {
		if restoreErr := src.root.Rename(tmpName, src.rel); restoreErr != nil {
			return fmt.Errorf("%w; restore %s: %v", err, src.path, restoreErr)
		}
		return err
	}
	return nil
}

func renameSameFileDestination(src, dst string) error {
	tmp, err := os.CreateTemp(filepath.Dir(src), ".reamesAgent-move-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Remove(tmpName); err != nil {
		return err
	}

	if err := renameFile(src, tmpName); err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		if restoreErr := renameFile(tmpName, src); restoreErr != nil {
			return fmt.Errorf("%w; restore %s: %v", err, src, restoreErr)
		}
		return err
	}
	if err := renameFile(tmpName, dst); err != nil {
		if restoreErr := renameFile(tmpName, src); restoreErr != nil {
			return fmt.Errorf("%w; restore %s: %v", err, src, restoreErr)
		}
		return err
	}
	return nil
}

func isCrossDeviceMove(err error) bool {
	var linkErr *os.LinkError
	if !errors.As(err, &linkErr) {
		return false
	}
	msg := strings.ToLower(linkErr.Err.Error())
	return strings.Contains(msg, "cross-device") ||
		strings.Contains(msg, "different device") ||
		strings.Contains(msg, "different disk") ||
		strings.Contains(msg, "not same device")
}
