package builtin

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"reames-agent/internal/fileutil"
	fileenc "reames-agent/internal/fileutil/encoding"
)

// rootedTarget binds a writer target to an already-opened writable root and a
// root-relative path. Production workspace tools always populate root; the nil
// root form preserves the process-cwd behavior of zero-value built-ins used by
// tests and embedders that have not supplied a workspace.
type rootedTarget struct {
	path     string
	resolved string
	rel      string
	rootPath string
	root     *os.Root
}

func openRootedWriteTarget(roots []string, guard SessionDataGuard, target string) (*rootedTarget, error) {
	if len(roots) == 0 {
		if err := guard.Check(target); err != nil {
			return nil, err
		}
		return &rootedTarget{path: target}, nil
	}

	resolved, err := realPath(target)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", target, err)
	}
	var selected, rel string
	for _, candidate := range roots {
		candidateRel, relErr := filepath.Rel(candidate, resolved)
		if relErr == nil && filepath.IsLocal(candidateRel) {
			selected, rel = candidate, candidateRel
			break
		}
	}
	if selected == "" {
		return nil, fmt.Errorf("path %q is outside the writable roots (writes are confined to %s); write inside the workspace or a configured allow_write root, or widen [sandbox] workspace_root / allow_write in reames-agent.toml", target, joinRoots(roots))
	}
	if rel == "." {
		return nil, fmt.Errorf("path %q names a writable root directory, not a file", target)
	}
	if err := guard.Check(resolved); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(selected, 0o755); err != nil {
		return nil, fmt.Errorf("create writable root %s: %w", selected, err)
	}
	root, err := os.OpenRoot(selected)
	if err != nil {
		return nil, fmt.Errorf("open writable root %s: %w", selected, err)
	}
	return &rootedTarget{path: filepath.Clean(target), resolved: resolved, rel: rel, rootPath: selected, root: root}, nil
}

func joinRoots(roots []string) string {
	if len(roots) == 0 {
		return ""
	}
	out := roots[0]
	for _, root := range roots[1:] {
		out += ", " + root
	}
	return out
}

func (t *rootedTarget) Close() error {
	if t == nil || t.root == nil {
		return nil
	}
	return t.root.Close()
}

func (t *rootedTarget) ReadFile() ([]byte, error) {
	if t.root != nil {
		return t.root.ReadFile(t.rel)
	}
	return os.ReadFile(t.path)
}

func (t *rootedTarget) Open() (*os.File, error) {
	if t.root != nil {
		return t.root.Open(t.rel)
	}
	return os.Open(t.path)
}

func (t *rootedTarget) OpenFile(flag int, perm os.FileMode) (*os.File, error) {
	if t.root != nil {
		return t.root.OpenFile(t.rel, flag, perm)
	}
	return os.OpenFile(t.path, flag, perm)
}

func (t *rootedTarget) Stat() (os.FileInfo, error) {
	if t.root != nil {
		return t.root.Stat(t.rel)
	}
	return os.Stat(t.path)
}

func (t *rootedTarget) Lstat() (os.FileInfo, error) {
	if t.root != nil {
		return t.root.Lstat(t.rel)
	}
	return os.Lstat(t.path)
}

func (t *rootedTarget) MkdirParent(perm os.FileMode) error {
	dir := filepath.Dir(t.rel)
	if t.root != nil {
		return t.root.MkdirAll(dir, perm)
	}
	dir = filepath.Dir(t.path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, perm)
}

func (t *rootedTarget) Remove() error {
	if t.root != nil {
		return t.root.Remove(t.rel)
	}
	return os.Remove(t.path)
}

func (t *rootedTarget) readEncoded() (string, fileenc.Kind, error) {
	b, err := t.ReadFile()
	if err != nil {
		return "", 0, err
	}
	enc, _ := fileenc.Detect(b)
	return string(fileenc.Decode(b, enc)), enc, nil
}

func (t *rootedTarget) writeEncoded(content string, enc fileenc.Kind, defaultPerm os.FileMode) error {
	return t.writeBytes(fileenc.Encode(content, enc), defaultPerm)
}

func (t *rootedTarget) writeBytes(data []byte, defaultPerm os.FileMode) error {
	perm := defaultPerm
	if info, err := t.Stat(); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", t.path)
		}
		perm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	return t.replaceBytes(data, perm)
}

func (t *rootedTarget) replaceBytes(data []byte, perm os.FileMode) error {
	if t.root != nil {
		return fileutil.AtomicWriteRootFile(t.root, t.rel, data, perm)
	}
	return fileutil.AtomicWriteFile(t.path, data, perm)
}

func (t *rootedTarget) renameTo(dst *rootedTarget) error {
	if t.root != nil && dst.root != nil && sameRootPath(t.rootPath, dst.rootPath) {
		return t.root.Rename(t.rel, dst.rel)
	}
	if t.root == nil && dst.root == nil {
		return renameFile(t.path, dst.path)
	}
	return fmt.Errorf("rename crosses writable roots")
}

func sameRootPath(a, b string) bool {
	if foldPaths {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func copyRootedFileAndRemoveSource(src, dst *rootedTarget, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cross-root fallback only supports regular files")
	}
	in, err := src.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	if err := dst.MkdirParent(0o755); err != nil {
		return err
	}
	out, err := dst.OpenFile(os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	removeDst := true
	defer func() {
		if removeDst {
			_ = dst.Remove()
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	if err := src.Remove(); err != nil {
		return err
	}
	removeDst = false
	return nil
}
