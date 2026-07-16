package installsource

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"reames-agent/internal/pluginregistry"
)

const (
	maxRegistryGitFiles      = 4096
	maxRegistryGitBytes      = 64 << 20
	maxRegistryGitTreeOutput = 8 << 20
	maxRegistryGitPath       = 1024
)

type registryGitEntry struct {
	rel        string
	oid        string
	executable bool
	size       int64
	content    [sha256.Size]byte
}

type cappedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = w.buffer.Write(p)
	}
	return original, nil
}

func (w *cappedBuffer) String() string { return w.buffer.String() }

// secureGitCommand isolates source materialization from ambient Git settings.
// In particular, a project-controlled .gitattributes file must not be able to
// invoke a filter or credential helper inherited from the user's global/system
// configuration, and core.autocrlf must not vary the checkout by host policy.
func secureGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	base := []string{
		"-c", "core.autocrlf=false",
		"-c", "core.eol=lf",
		"-c", "core.safecrlf=false",
		"-c", "filter.lfs.smudge=",
		"-c", "filter.lfs.required=false",
		"-c", "advice.detachedHead=false",
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") || strings.EqualFold(key, "GCM_INTERACTIVE") {
			continue
		}
		env = append(env, item)
	}
	cmd.Env = append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_LFS_SKIP_SMUDGE=1",
		"GCM_INTERACTIVE=Never",
	)
	return cmd
}

func runSecureGitOutput(ctx context.Context, limit int64, args ...string) ([]byte, error) {
	cmd := secureGitCommand(ctx, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := cappedBuffer{limit: 64 << 10}
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	if readErr != nil || int64(len(body)) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if readErr != nil {
			return nil, readErr
		}
		return nil, fmt.Errorf("git output exceeds %d bytes", limit)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return body, nil
}

// RegistryGitSourceEvidence returns the exact current commit and the canonical
// cross-platform source digest an operator must place in a signed registry
// entry. The digest covers Git paths, executable-bit intent, sizes, and raw blob
// bytes; it never hashes a checkout transformed by core.autocrlf or local modes.
func RegistryGitSourceEvidence(ctx context.Context, repository, subpath string) (string, string, error) {
	revisionBytes, err := runSecureGitOutput(ctx, 1024, "-C", repository, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("resolve Git commit: %w", err)
	}
	revision := strings.ToLower(strings.TrimSpace(string(revisionBytes)))
	if !isFullGitHubCommit(revision) {
		return "", "", fmt.Errorf("Git HEAD %q is not a full 40-character commit", revision)
	}
	digest, err := registryGitTreeDigest(ctx, repository, revision, subpath)
	if err != nil {
		return "", "", err
	}
	return revision, digest, nil
}

func registryGitTreeDigest(ctx context.Context, repository, revision, subpath string) (string, error) {
	if !isFullGitHubCommit(revision) {
		return "", fmt.Errorf("revision %q is not a full 40-character commit", revision)
	}
	subpath = strings.TrimSpace(strings.ReplaceAll(subpath, "\\", "/"))
	if subpath != "" {
		if err := validatePortableRegistryGitPath(subpath); err != nil {
			return "", fmt.Errorf("subpath: %w", err)
		}
	}
	args := []string{"-C", repository, "ls-tree", "-rz", "-r", "--full-tree", revision, "--"}
	if subpath != "" {
		args = append(args, subpath)
	}
	tree, err := runSecureGitOutput(ctx, maxRegistryGitTreeOutput, args...)
	if err != nil {
		return "", fmt.Errorf("list registry Git tree: %w", err)
	}
	entries, err := parseRegistryGitTree(tree, subpath)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("registry Git tree contains no regular files")
	}
	if err := hashRegistryGitBlobs(ctx, repository, entries); err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	h := sha256.New()
	for _, entry := range entries {
		executable := 0
		if entry.executable {
			executable = 1
		}
		_, _ = fmt.Fprintf(h, "file\x00%s\x00%d\x00%d\x00%s\x00", entry.rel, entry.size, executable, hex.EncodeToString(entry.content[:]))
	}
	return pluginregistry.GitTreeDigestPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

func parseRegistryGitTree(body []byte, subpath string) ([]registryGitEntry, error) {
	records := bytes.Split(body, []byte{0})
	entries := make([]registryGitEntry, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	pathFolder := cases.Fold()
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		meta, rawPath, ok := bytes.Cut(record, []byte{'\t'})
		fields := bytes.Fields(meta)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("malformed git ls-tree record")
		}
		mode, objectType, oid := string(fields[0]), string(fields[1]), strings.ToLower(string(fields[2]))
		if objectType != "blob" || (mode != "100644" && mode != "100755") {
			return nil, fmt.Errorf("registry Git tree contains unsupported %s %s", objectType, mode)
		}
		if len(oid) != 40 && len(oid) != 64 {
			return nil, fmt.Errorf("registry Git tree contains invalid object id %q", oid)
		}
		if !utf8.Valid(rawPath) {
			return nil, fmt.Errorf("registry Git tree contains a non-UTF-8 path")
		}
		rel, err := registryGitRelativePath(string(rawPath), subpath)
		if err != nil {
			return nil, err
		}
		folded := pathFolder.String(rel)
		if _, duplicate := seen[folded]; duplicate {
			return nil, fmt.Errorf("registry Git tree contains a cross-platform path collision at %q", rel)
		}
		seen[folded] = struct{}{}
		entries = append(entries, registryGitEntry{rel: rel, oid: oid, executable: mode == "100755"})
		if len(entries) > maxRegistryGitFiles {
			return nil, fmt.Errorf("registry Git tree exceeds %d files", maxRegistryGitFiles)
		}
	}
	return entries, nil
}

func registryGitRelativePath(full, subpath string) (string, error) {
	rel := full
	if subpath != "" {
		prefix := strings.TrimSuffix(subpath, "/") + "/"
		if !strings.HasPrefix(full, prefix) {
			return "", fmt.Errorf("Git path %q is outside subpath %q", full, subpath)
		}
		rel = strings.TrimPrefix(full, prefix)
	}
	if err := validatePortableRegistryGitPath(rel); err != nil {
		return "", err
	}
	return rel, nil
}

func validatePortableRegistryGitPath(value string) error {
	if value == "" || len(value) > maxRegistryGitPath || path.Clean(value) != value || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "../") || strings.Contains(value, "\\") {
		return fmt.Errorf("registry Git path %q is not a portable clean relative path", value)
	}
	if !norm.NFC.IsNormalString(value) {
		return fmt.Errorf("registry Git path %q must use Unicode NFC normalization", value)
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return fmt.Errorf("registry Git path %q contains control or formatting characters", value)
		}
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || len(component) > 255 || strings.TrimRight(component, ". ") != component || strings.ContainsAny(component, `<>:"|?*`) || isWindowsReservedPathComponent(component) {
			return fmt.Errorf("registry Git path %q is not portable across supported platforms", value)
		}
	}
	return nil
}

func isWindowsReservedPathComponent(component string) bool {
	base, _, _ := strings.Cut(component, ".")
	base = strings.ToUpper(base)
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return true
	}
	return false
}

func hashRegistryGitBlobs(ctx context.Context, repository string, entries []registryGitEntry) error {
	cmd := secureGitCommand(ctx, "-C", repository, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := cappedBuffer{limit: 64 << 10}
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	writeDone := make(chan error, 1)
	go func() {
		writer := bufio.NewWriter(stdin)
		for _, entry := range entries {
			if _, err := fmt.Fprintln(writer, entry.oid); err != nil {
				_ = stdin.Close()
				writeDone <- err
				return
			}
		}
		err := writer.Flush()
		closeErr := stdin.Close()
		if err == nil {
			err = closeErr
		}
		writeDone <- err
	}()
	reader := bufio.NewReader(stdout)
	var total int64
	for i := range entries {
		header, readErr := reader.ReadString('\n')
		fields := strings.Fields(strings.TrimSpace(header))
		if readErr != nil || len(fields) != 3 || strings.ToLower(fields[0]) != entries[i].oid || fields[1] != "blob" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			<-writeDone
			return fmt.Errorf("read registry Git blob %s: malformed cat-file response", entries[i].oid)
		}
		size, parseErr := strconv.ParseInt(fields[2], 10, 64)
		if parseErr != nil || size < 0 || total+size > maxRegistryGitBytes {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			<-writeDone
			return fmt.Errorf("registry Git tree exceeds %d bytes", maxRegistryGitBytes)
		}
		h := sha256.New()
		if _, err := io.CopyN(h, reader, size); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			<-writeDone
			return fmt.Errorf("read registry Git blob %s: %w", entries[i].oid, err)
		}
		separator, err := reader.ReadByte()
		if err != nil || separator != '\n' {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			<-writeDone
			return fmt.Errorf("read registry Git blob %s terminator", entries[i].oid)
		}
		entries[i].size = size
		copy(entries[i].content[:], h.Sum(nil))
		total += size
	}
	writeErr := <-writeDone
	waitErr := cmd.Wait()
	if writeErr != nil {
		return fmt.Errorf("write registry Git blob request: %w", writeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("read registry Git blobs: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func isFullGitHubCommit(value string) bool {
	if len(value) != 40 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
