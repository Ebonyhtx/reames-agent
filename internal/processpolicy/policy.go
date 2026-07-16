// Package processpolicy defines the least-privilege launch contract for
// package-owned Hook and MCP subprocesses. It deliberately keeps user-authored
// global hooks and MCP entries on their historical compatibility path while
// installed plugin packages get a filtered environment, explicit writable
// roots, sensitive-file read barriers, and fail-closed OS confinement.
package processpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"reames-agent/internal/sandbox"
)

var commandArgsWithOptions = sandbox.CommandArgsWithOptions

var sensitiveEnvKeyPattern = regexp.MustCompile(`(?i)((^|[_-])(api[_-]?key|access[_-]?key|private[_-]?key|secret|token|password|passwd)([_-]|$)|[_-]pwd([_-]|$))`)

var credentialEnvKeys = struct {
	sync.RWMutex
	keys map[string]struct{}
}{keys: map[string]struct{}{}}

// RegisterCredentialEnvKeys permanently marks variables that Reames Agent may
// load as application credentials. Registration is a process-lifetime union:
// concurrent workspaces cannot make another workspace's saved keys visible to
// a child process by rebuilding with a narrower config. Explicit per-process
// env may still add a required value after ProcessEnvironment returns.
func RegisterCredentialEnvKeys(keys []string) {
	credentialEnvKeys.Lock()
	defer credentialEnvKeys.Unlock()
	for _, key := range keys {
		if key = credentialEnvKey(key); key != "" {
			credentialEnvKeys.keys[key] = struct{}{}
		}
	}
}

// ProcessEnvironment returns the ambient environment for tool and integration
// subprocesses after removing every registered Reames Agent credential. It
// deliberately preserves unrelated variables (including developer CLI auth)
// for compatibility; callers overlay narrowly scoped explicit env afterwards.
func ProcessEnvironment() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if !ok || registeredCredentialEnvKey(key) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func credentialEnvKey(key string) string {
	return strings.ToUpper(strings.TrimSpace(key))
}

func registeredCredentialEnvKey(key string) bool {
	credentialEnvKeys.RLock()
	defer credentialEnvKeys.RUnlock()
	_, ok := credentialEnvKeys.keys[credentialEnvKey(key)]
	return ok
}

// PackagePolicy identifies one installed package's immutable code, managed
// runtime state, workspace, and host application home.
type PackagePolicy struct {
	Owner         string
	PackageRoot   string
	StateRoot     string
	WorkspaceRoot string
	HostHome      string
	Network       bool
}

// Enabled reports whether this launch belongs to an installed plugin package.
func (p PackagePolicy) Enabled() bool { return strings.TrimSpace(p.Owner) != "" }

// Prepare validates the trusted paths and creates the managed state directory
// through os.Root so a pre-planted symlink cannot redirect it outside HostHome.
func (p PackagePolicy) Prepare() error {
	if !p.Enabled() {
		return nil
	}
	if err := requireDirectory("package root", p.PackageRoot); err != nil {
		return fmt.Errorf("plugin package %q: %w", p.Owner, err)
	}
	if strings.TrimSpace(p.HostHome) == "" || !filepath.IsAbs(p.HostHome) {
		return fmt.Errorf("plugin package %q: host home must be an absolute path", p.Owner)
	}
	if strings.TrimSpace(p.StateRoot) == "" || !filepath.IsAbs(p.StateRoot) {
		return fmt.Errorf("plugin package %q: state root must be an absolute path", p.Owner)
	}
	if pathWithin(p.PackageRoot, p.StateRoot) {
		return fmt.Errorf("plugin package %q: state root must stay outside immutable package root", p.Owner)
	}
	rel, err := filepath.Rel(p.HostHome, p.StateRoot)
	if err != nil || rel == "." || relEscapes(rel) {
		return fmt.Errorf("plugin package %q: state root must stay within host home", p.Owner)
	}
	if err := os.MkdirAll(p.HostHome, 0o700); err != nil {
		return fmt.Errorf("plugin package %q: create host home: %w", p.Owner, err)
	}
	root, err := os.OpenRoot(p.HostHome)
	if err != nil {
		return fmt.Errorf("plugin package %q: open host home: %w", p.Owner, err)
	}
	defer root.Close()
	if err := root.MkdirAll(rel, 0o700); err != nil {
		return fmt.Errorf("plugin package %q: create managed state root: %w", p.Owner, err)
	}
	if err := root.MkdirAll(filepath.Join(rel, "tmp"), 0o700); err != nil {
		return fmt.Errorf("plugin package %q: create managed temp root: %w", p.Owner, err)
	}
	info, err := root.Stat(rel)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a directory")
		}
		return fmt.Errorf("plugin package %q: inspect managed state root: %w", p.Owner, err)
	}
	if strings.TrimSpace(p.WorkspaceRoot) != "" {
		if err := requireDirectory("workspace root", p.WorkspaceRoot); err != nil {
			return fmt.Errorf("plugin package %q: %w", p.Owner, err)
		}
	}
	return nil
}

// ChildEnvironment returns a deterministic, explicit child environment. The
// ambient process contributes only OS startup variables; package-declared env
// is then overlaid, followed by trusted ownership/path values that a manifest
// cannot spoof.
func (p PackagePolicy) ChildEnvironment(overrides map[string]string) []string {
	env := CoreEnvironment(os.Environ())
	trusted := cloneMap(overrides)
	trusted["REAMES_AGENT_PLUGIN_NAME"] = strings.TrimSpace(p.Owner)
	trusted["REAMES_AGENT_PLUGIN_ROOT"] = p.PackageRoot
	trusted["REAMES_AGENT_PLUGIN_STATE"] = p.StateRoot
	trusted["TMPDIR"] = filepath.Join(p.StateRoot, "tmp")
	trusted["TMP"] = filepath.Join(p.StateRoot, "tmp")
	trusted["TEMP"] = filepath.Join(p.StateRoot, "tmp")
	if p.HostHome != "" {
		trusted["REAMES_AGENT_HOME"] = p.HostHome
	}
	if p.WorkspaceRoot != "" {
		trusted["REAMES_AGENT_WORKSPACE_ROOT"] = p.WorkspaceRoot
		trusted["CLAUDE_PROJECT_DIR"] = p.WorkspaceRoot
	}
	return MergeEnvironment(env, trusted)
}

// HostEnvironment is the environment used to start the trusted sandbox
// wrapper. Package-provided variables are serialized behind one reserved key,
// so their original names and values cannot affect the wrapper or appear in
// its argv; the in-sandbox helper restores them immediately before exec.
func (p PackagePolicy) HostEnvironment(childEnv []string) ([]string, error) {
	return sandbox.CommandHostEnvironment(CoreEnvironment(os.Environ()), childEnv)
}

// WrapCommand validates the policy and wraps argv in the platform sandbox.
// Sandbox unavailability is a hard failure for installed package processes;
// unlike interactive bash there is no escape approval path.
func (p PackagePolicy) WrapCommand(args []string, childEnv []string, dir string, writable bool) ([]string, []string, error) {
	if !p.Enabled() {
		return append([]string(nil), args...), nil, nil
	}
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return nil, nil, fmt.Errorf("plugin package %q: command is required", p.Owner)
	}
	if err := p.Prepare(); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(dir) == "" {
		dir = p.PackageRoot
	}
	wrapped, ok := commandArgsWithOptions(p.sandboxSpec(), args, sandbox.CommandOptions{
		Writable: writable,
		Env:      append([]string(nil), childEnv...),
		Dir:      dir,
	})
	if !ok {
		return nil, nil, fmt.Errorf("plugin package %q: OS sandbox requested but unavailable; refusing to run unconfined", p.Owner)
	}
	hostEnv, err := p.HostEnvironment(childEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("plugin package %q: %w", p.Owner, err)
	}
	return wrapped, hostEnv, nil
}

func (p PackagePolicy) sandboxSpec() sandbox.Spec {
	writeRoots := uniqueExistingDirs([]string{p.StateRoot, p.WorkspaceRoot})
	forbidRoots, forbidPaths := sensitiveReadBarriers(p.HostHome)
	return sandbox.Spec{
		Mode:            "enforce",
		WriteRoots:      writeRoots,
		ReadRoots:       uniqueExistingDirs([]string{p.PackageRoot}),
		ForbidReadRoots: forbidRoots,
		ForbidReadPaths: forbidPaths,
		Network:         p.Network,
		Strict:          true,
	}
}

// CoreEnvironment follows the same principle used by Codex's MCP launcher:
// inherit only variables needed to locate executables, shells, profiles, temp
// directories, locale, and Windows runtime components.
func CoreEnvironment(base []string) []string {
	allowed := unixCoreEnvironment
	if runtime.GOOS == "windows" {
		allowed = windowsCoreEnvironment
	}
	allow := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allow[canonicalEnvKey(key)] = true
	}
	out := make([]string, 0, len(allowed))
	seen := map[string]bool{}
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		canon := canonicalEnvKey(key)
		if !ok || key == "" || !allow[canon] || seen[canon] {
			continue
		}
		seen[canon] = true
		out = append(out, item)
	}
	sortEnvironment(out)
	return out
}

// MergeEnvironment overlays key/value pairs without leaving duplicate entries.
func MergeEnvironment(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	names := make(map[string]string, len(base)+len(overrides))
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		canon := canonicalEnvKey(key)
		values[canon] = value
		names[canon] = key
	}
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" || strings.Contains(key, "=") || strings.IndexByte(key, 0) >= 0 || strings.IndexByte(value, 0) >= 0 {
			continue
		}
		canon := canonicalEnvKey(key)
		values[canon] = value
		names[canon] = key
	}
	keys := make([]string, 0, len(values))
	for canon := range values {
		keys = append(keys, canon)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, canon := range keys {
		out = append(out, names[canon]+"="+values[canon])
	}
	return out
}

// EnvKeySensitive reports whether a variable name conventionally carries a
// credential. Bare PWD/OLDPWD are excluded; DB_PWD and similar names match.
func EnvKeySensitive(key string) bool {
	key = strings.TrimSpace(key)
	return key != "" && sensitiveEnvKeyPattern.MatchString(key)
}

// SensitiveValues extracts package-declared credential values for exact-value
// diagnostic redaction. Ambient credentials never enter the child environment.
func SensitiveValues(env map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for key, value := range env {
		value = strings.TrimSpace(value)
		if !EnvKeySensitive(key) || value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// RedactValues masks exact configured credential values from bounded process
// diagnostics before they reach UI/model surfaces.
func RedactValues(text string, values []string) string {
	for _, value := range values {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[redacted]")
		}
	}
	return text
}

var unixCoreEnvironment = []string{
	"HOME", "LOGNAME", "PATH", "SHELL", "USER", "LANG", "LC_ALL", "LC_CTYPE",
	"TERM", "TMPDIR", "TEMP", "TMP", "TZ", "__CF_USER_TEXT_ENCODING",
}

var windowsCoreEnvironment = []string{
	"PATH", "PATHEXT", "SHELL", "COMSPEC", "SYSTEMROOT", "SYSTEMDRIVE",
	"USERNAME", "USERDOMAIN", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
	"PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMW6432", "PROGRAMDATA",
	"LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "TMPDIR", "POWERSHELL", "PWSH",
}

func sensitiveReadBarriers(hostHome string) ([]string, []string) {
	var dirs, files []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, rel := range []string{
			".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker", filepath.Join(".config", "gcloud"), filepath.Join(".config", "gh"),
		} {
			dirs = append(dirs, filepath.Join(home, rel))
		}
		for _, rel := range []string{".netrc", ".npmrc", ".pypirc", ".git-credentials"} {
			files = append(files, filepath.Join(home, rel))
		}
	}
	if strings.TrimSpace(hostHome) != "" {
		for _, rel := range []string{".env", "credentials", "credentials.enc", "config.toml", filepath.Join("bot", "pairing.json")} {
			files = append(files, filepath.Join(hostHome, rel))
		}
		for _, rel := range []string{filepath.Join("weixin", "accounts"), filepath.Join("bot", "secrets")} {
			dirs = append(dirs, filepath.Join(hostHome, rel))
		}
	}
	return uniqueExistingDirs(dirs), uniqueExistingFiles(files)
}

func uniqueExistingDirs(paths []string) []string {
	return uniqueExisting(paths, true)
}

func uniqueExistingFiles(paths []string) []string {
	return uniqueExisting(paths, false)
}

func uniqueExisting(paths []string, wantDir bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			abs = real
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() != wantDir {
			continue
		}
		key := canonicalPath(abs)
		if !seen[key] {
			seen[key] = true
			out = append(out, abs)
		}
	}
	sort.Slice(out, func(i, j int) bool { return canonicalPath(out[i]) < canonicalPath(out[j]) })
	return out
}

func requireDirectory(label, path string) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be an absolute path", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", label)
	}
	return nil
}

func pathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && !relEscapes(rel)
}

func relEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

func canonicalPath(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func canonicalEnvKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(strings.TrimSpace(key))
	}
	return strings.TrimSpace(key)
}

func sortEnvironment(env []string) {
	sort.Slice(env, func(i, j int) bool {
		left, _, _ := strings.Cut(env[i], "=")
		right, _, _ := strings.Cut(env[j], "=")
		return canonicalEnvKey(left) < canonicalEnvKey(right)
	})
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+6)
	for key, value := range in {
		out[key] = value
	}
	return out
}
