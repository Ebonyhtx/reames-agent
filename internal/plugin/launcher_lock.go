package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/mod/semver"

	"reames-agent/internal/mcptrust"
	"reames-agent/internal/processpolicy"
)

type launcherLocator struct {
	kind    string
	value   string
	arg     int
	prefix  string
	command string
}

// LauncherTrustInfo is a redacted classification used by the controller to
// decide whether workspace trust needs a locked re-preflight.
type LauncherTrustInfo struct {
	Kind      string
	Immutable bool
}

func MutableLauncherForTrust(spec Spec) (LauncherTrustInfo, bool) {
	locator, ok := mutableLauncherLocator(spec)
	if !ok {
		return LauncherTrustInfo{}, false
	}
	return LauncherTrustInfo{Kind: locator.kind, Immutable: immutableLauncherLocator(locator)}, true
}

var (
	pep508Package                  = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)(\[[^]]+\])?(?:==([^\s]+))?$`)
	fullGitCommit                  = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
	pypiBaseURL                    = "https://pypi.org/pypi"
	resolveLauncherLocatorForTrust = resolveLauncherLocator
)

func effectiveLaunchArgs(spec Spec) []string {
	if spec.LaunchArgs != nil {
		return spec.LaunchArgs
	}
	return spec.Args
}

func exactPEP440Version(version string) bool {
	version = strings.TrimSpace(version)
	return version != "" && !strings.Contains(version, "*")
}

func mutableLauncherLocator(spec Spec) (launcherLocator, bool) {
	command := strings.TrimSuffix(strings.ToLower(filepath.Base(strings.TrimSpace(spec.Command))), ".exe")
	var kind string
	switch command {
	case "npx", "bunx", "uvx":
		kind = command
	default:
		return launcherLocator{}, false
	}
	args := spec.Args
	if kind == "uvx" {
		for i, arg := range args {
			if arg == "--from" && i+1 < len(args) {
				return launcherLocator{kind: kind, value: args[i+1], arg: i + 1, command: command}, true
			}
			if strings.HasPrefix(arg, "--from=") {
				return launcherLocator{kind: kind, value: strings.TrimPrefix(arg, "--from="), arg: i, prefix: "--from=", command: command}, true
			}
		}
	}
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			return launcherLocator{kind: kind, value: args[i+1], arg: i + 1, command: command}, true
		}
		if strings.HasPrefix(arg, "-") {
			if strings.Contains(arg, "=") || safeLauncherFlag(kind, arg) {
				continue
			}
			return launcherLocator{kind: kind, command: command}, true
		}
		return launcherLocator{kind: kind, value: arg, arg: i, command: command}, true
	}
	return launcherLocator{kind: kind, command: command}, true
}

func immutableLauncherLocator(locator launcherLocator) bool {
	value := strings.TrimSpace(locator.value)
	if strings.HasPrefix(value, "git+") {
		if !strings.HasPrefix(strings.ToLower(value), "git+https://") {
			return false
		}
		at := strings.LastIndex(value, "@")
		return at > len("git+https://") && fullGitCommit.MatchString(value[at+1:])
	}
	switch locator.kind {
	case "npx", "bunx":
		name := npmPackageName(value)
		return name != "" && len(value) > len(name)+1 && value[len(name)] == '@' && semver.IsValid("v"+value[len(name)+1:])
	case "uvx":
		match := pep508Package.FindStringSubmatch(value)
		return match != nil && exactPEP440Version(match[3])
	default:
		return false
	}
}

func safeLauncherFlag(kind, flag string) bool {
	switch kind {
	case "npx":
		return flag == "-y" || flag == "--yes" || flag == "--quiet" || flag == "--silent" || flag == "--offline" || flag == "--prefer-offline"
	case "bunx":
		return flag == "--bun" || flag == "--no-install" || flag == "--silent"
	case "uvx":
		return flag == "--offline" || flag == "--refresh" || flag == "--no-cache"
	default:
		return false
	}
}

func preparePersistentLauncher(ctx context.Context, spec Spec) (Spec, *mcptrust.LauncherLock, error) {
	locator, mutable := mutableLauncherLocator(spec)
	if !mutable {
		return spec, nil, nil
	}
	if strings.TrimSpace(locator.value) == "" {
		return spec, nil, fmt.Errorf("%s package locator was not found", locator.kind)
	}
	resolved, digest, err := resolveLauncherLocatorForTrust(ctx, spec, locator)
	if err != nil {
		return spec, nil, err
	}
	lock := &mcptrust.LauncherLock{Server: spec.Name, Locator: digestText(locator.value), ResolvedVersion: resolved, ContentSHA256: digest}
	lock.Workspace = spec.TrustManager.WorkspaceFingerprint()
	applyLauncherResolution(&spec, locator, *lock, false)
	return spec, lock, nil
}

func applyStoredLauncherLock(spec Spec) (Spec, error) {
	if strings.TrimSpace(spec.LauncherDigest) != "" || spec.TrustManager == nil {
		return spec, nil
	}
	locator, mutable := mutableLauncherLocator(spec)
	if !mutable || strings.TrimSpace(locator.value) == "" {
		return spec, nil
	}
	lock, ok, err := spec.TrustManager.GetLauncherLock(spec.Name, digestText(locator.value))
	if err != nil || !ok {
		return spec, err
	}
	applyLauncherResolution(&spec, locator, lock, true)
	return spec, nil
}

// identityLaunchArgs removes only the offline flag Reames injects while
// replaying a stored launcher lock. The exact resolved locator remains bound,
// so online trust preflight and later offline execution share one identity.
func identityLaunchArgs(spec Spec) []string {
	args := append([]string(nil), effectiveLaunchArgs(spec)...)
	if spec.LaunchArgs == nil || strings.TrimSpace(spec.LauncherDigest) == "" {
		return args
	}
	locator, ok := mutableLauncherLocator(spec)
	if !ok || hasLauncherOfflineFlag(locator.kind, spec.Args) {
		return args
	}
	flag := "--offline"
	if locator.kind == "bunx" {
		flag = "--no-install"
	}
	for i, arg := range args {
		if arg == flag {
			return append(args[:i], args[i+1:]...)
		}
	}
	return args
}

func applyLauncherResolution(spec *Spec, locator launcherLocator, lock mcptrust.LauncherLock, offline bool) {
	args := append([]string(nil), spec.Args...)
	resolved := lock.ResolvedVersion
	if strings.HasPrefix(locator.value, "git+") && fullGitCommit.MatchString(resolved) {
		if at := strings.LastIndex(locator.value, "@"); at > len("git+https://") {
			resolved = locator.value[:at] + "@" + resolved
		}
	}
	args[locator.arg] = locator.prefix + resolved
	if offline && !hasLauncherOfflineFlag(locator.kind, args) {
		flag := "--offline"
		if locator.kind == "bunx" {
			flag = "--no-install"
		}
		args = append(args[:locator.arg], append([]string{flag}, args[locator.arg:]...)...)
	}
	spec.LaunchArgs = args
	spec.LauncherLocator = lock.Locator
	spec.LauncherResolvedVersion = lock.ResolvedVersion
	spec.LauncherDigest = mcptrust.LauncherLockFingerprint(lock)
}

func hasLauncherOfflineFlag(kind string, args []string) bool {
	for _, arg := range args {
		if arg == "--offline" || (kind == "bunx" && arg == "--no-install") {
			return true
		}
	}
	return false
}

func resolveLauncherLocator(ctx context.Context, spec Spec, locator launcherLocator) (string, string, error) {
	if strings.HasPrefix(locator.value, "git+") {
		return resolveGitLocator(ctx, spec, locator.value)
	}
	switch locator.kind {
	case "npx", "bunx":
		return resolveNPMPackage(ctx, spec, locator.value)
	case "uvx":
		return resolvePyPIPackage(ctx, locator.value)
	default:
		return "", "", fmt.Errorf("unsupported mutable launcher %q", locator.kind)
	}
}

func resolveNPMPackage(ctx context.Context, spec Spec, locator string) (string, string, error) {
	name := npmPackageName(locator)
	if name == "" {
		return "", "", fmt.Errorf("unsupported npm package locator %q", locator)
	}
	env := enrichStdioShellPATH(ctx, mergeEnv(processpolicy.ProcessEnvironment(), spec.Env))
	npm, ok := lookPathInEnv("npm", env)
	if !ok {
		return "", "", fmt.Errorf("npm is required to lock %q", locator)
	}
	command := exec.CommandContext(ctx, npm, "view", locator, "version", "dist.integrity", "--json")
	command.Env = env
	out, err := command.Output()
	if err != nil {
		return "", "", fmt.Errorf("resolve npm package %q: %w", locator, err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		return "", "", fmt.Errorf("parse npm resolution for %q: %w", locator, err)
	}
	version, _ := result["version"].(string)
	integrity, _ := result["dist.integrity"].(string)
	if integrity == "" {
		if dist, ok := result["dist"].(map[string]any); ok {
			integrity, _ = dist["integrity"].(string)
		}
	}
	if version == "" || integrity == "" {
		return "", "", fmt.Errorf("npm did not return an exact version and integrity for %q", locator)
	}
	return name + "@" + version, digestText(integrity), nil
}

func npmPackageName(locator string) string {
	locator = strings.TrimSpace(locator)
	if locator == "" || strings.Contains(locator, ":") || (strings.Contains(locator, "/") && !strings.HasPrefix(locator, "@")) {
		return ""
	}
	if strings.HasPrefix(locator, "@") {
		slash := strings.Index(locator, "/")
		if slash < 2 {
			return ""
		}
		if at := strings.LastIndex(locator, "@"); at > slash {
			return locator[:at]
		}
		return locator
	}
	if at := strings.LastIndex(locator, "@"); at > 0 {
		return locator[:at]
	}
	return locator
}

func resolvePyPIPackage(ctx context.Context, locator string) (string, string, error) {
	match := pep508Package.FindStringSubmatch(strings.TrimSpace(locator))
	if match == nil {
		return "", "", fmt.Errorf("unsupported uvx package locator %q", locator)
	}
	name, extras, requestedVersion := match[1], match[2], match[3]
	if requestedVersion != "" && !exactPEP440Version(requestedVersion) {
		return "", "", fmt.Errorf("uvx locator %q uses a wildcard version; pin one exact version", locator)
	}
	endpoint := strings.TrimRight(pypiBaseURL, "/") + "/" + url.PathEscape(name)
	if requestedVersion != "" {
		endpoint += "/" + url.PathEscape(requestedVersion)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/json", nil)
	if err != nil {
		return "", "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", "", fmt.Errorf("resolve PyPI package %q: %w", locator, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("resolve PyPI package %q: %s", locator, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return "", "", err
	}
	var result struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
		URLs []struct {
			Digests struct {
				SHA256 string `json:"sha256"`
			} `json:"digests"`
		} `json:"urls"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("parse PyPI resolution for %q: %w", locator, err)
	}
	version := strings.TrimSpace(result.Info.Version)
	if requestedVersion != "" && version != requestedVersion {
		return "", "", fmt.Errorf("PyPI resolved %q to unexpected version %q", locator, version)
	}
	var digests []string
	for _, file := range result.URLs {
		if value := strings.TrimSpace(file.Digests.SHA256); value != "" {
			digests = append(digests, value)
		}
	}
	sort.Strings(digests)
	if version == "" || len(digests) == 0 {
		return "", "", fmt.Errorf("PyPI did not return an exact version and file digests for %q", locator)
	}
	return name + extras + "==" + version, digestText(strings.Join(digests, "\n")), nil
}

func resolveGitLocator(ctx context.Context, spec Spec, locator string) (string, string, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(locator)), "git+https://") {
		return "", "", fmt.Errorf("git launcher locator %q must use git+https", locator)
	}
	at := strings.LastIndex(locator, "@")
	if at < len("git+https://") || at == len(locator)-1 {
		return "", "", fmt.Errorf("git launcher locator %q requires an explicit ref", locator)
	}
	repository, ref := locator[:at], locator[at+1:]
	if fullGitCommit.MatchString(ref) {
		commit := strings.ToLower(ref)
		return commit, digestText(commit), nil
	}
	env := enrichStdioShellPATH(ctx, mergeEnv(processpolicy.ProcessEnvironment(), spec.Env))
	git, ok := lookPathInEnv("git", env)
	if !ok {
		return "", "", fmt.Errorf("git is required to resolve %q", locator)
	}
	command := exec.CommandContext(ctx, git, "ls-remote", strings.TrimPrefix(repository, "git+"), ref)
	command.Env = env
	out, err := command.Output()
	if err != nil {
		return "", "", fmt.Errorf("resolve git ref %q: %w", locator, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 1 || !fullGitCommit.MatchString(fields[0]) {
		return "", "", fmt.Errorf("git ref %q did not resolve to one exact commit", locator)
	}
	commit := strings.ToLower(fields[0])
	return commit, digestText(commit), nil
}

func digestText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
