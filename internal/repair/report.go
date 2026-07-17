package repair

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/pluginpkg"
)

type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Scope    string `json:"scope,omitempty"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
}

type BinaryStatus struct {
	Role    string `json:"role"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Regular bool   `json:"regular"`
	Size    int64  `json:"size,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Error   string `json:"error,omitempty"`
}

type StoreStatus struct {
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	FileCount  int    `json:"fileCount,omitempty"`
	Enabled    int    `json:"enabled,omitempty"`
	Disabled   int    `json:"disabled,omitempty"`
	Unreadable int    `json:"unreadable,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Report struct {
	SchemaVersion       int                `json:"schemaVersion"`
	GeneratedAt         string             `json:"generatedAt"`
	SafeModeRequested   bool               `json:"safeModeRequested"`
	SafeModeRecommended bool               `json:"safeModeRecommended"`
	Startup             StartupState       `json:"startup"`
	Config              ConfigReport       `json:"config"`
	PendingUpdate       *UpdateTransaction `json:"pendingUpdate,omitempty"`
	Binaries            []BinaryStatus     `json:"binaries"`
	Sessions            []StoreStatus      `json:"sessions"`
	Plugins             StoreStatus        `json:"plugins"`
	Findings            []Finding          `json:"findings"`
}

type InspectOptions struct {
	Root           string
	ExecutablePath string
	Now            func() time.Time
}

func Inspect(opts InspectOptions) (Report, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	report := Report{
		SchemaVersion:     1,
		GeneratedAt:       opts.Now().UTC().Format(time.RFC3339Nano),
		SafeModeRequested: config.SafeModeRequested(),
		Binaries:          []BinaryStatus{},
		Sessions:          []StoreStatus{},
		Findings:          []Finding{},
	}
	tracker := NewStartupTracker("")
	startup, startupErr := tracker.Read()
	if startupErr != nil {
		report.Findings = append(report.Findings, Finding{Severity: "error", Code: "startup.state_invalid", Scope: "startup", Message: startupErr.Error(), Action: "Run guard check and inspect the repair state directory."})
	} else {
		report.Startup = startup
		report.SafeModeRecommended = tracker.SafeModeRecommended()
	}
	configReport, err := InspectAndRepairConfig(ConfigOptions{Root: opts.Root})
	if err != nil {
		return report, err
	}
	report.Config = configReport
	for _, check := range configReport.Checks {
		if check.Exists && !check.Valid {
			report.Findings = append(report.Findings, Finding{Severity: "error", Code: "config.invalid", Scope: check.Scope, Message: check.Error, Action: "Run reames-agent-guard repair; project config changes require --project."})
		}
	}
	if tx, err := ReadPendingUpdate(); err == nil {
		report.PendingUpdate = tx
		report.Findings = append(report.Findings, Finding{Severity: "warning", Code: "update.probationary", Scope: "update", Message: fmt.Sprintf("update %s is awaiting healthy startup confirmation", tx.ToVersion), Action: "Do not delete repair backups; use guard rollback if the new release cannot start."})
	} else if !os.IsNotExist(err) {
		report.Findings = append(report.Findings, Finding{Severity: "error", Code: "update.metadata_invalid", Scope: "update", Message: err.Error(), Action: "Reinstall from a verified release; automatic rollback is disabled."})
	}
	if target := strings.TrimSpace(opts.ExecutablePath); target != "" {
		target = canonicalUpdatePath(target)
		report.Binaries = append(report.Binaries, inspectBinary("current", target), inspectBinary("previous", target+".previous"))
	}
	report.Sessions = inspectSessionStores(opts.Root)
	report.Plugins = inspectPluginStore()
	if report.Plugins.Error != "" {
		report.Findings = append(report.Findings, Finding{Severity: "error", Code: "plugins.state_invalid", Scope: "plugins", Message: report.Plugins.Error, Action: "Start in Safe Mode; do not launch plugin runtimes until the state is repaired."})
	}
	if report.SafeModeRecommended {
		report.Findings = append(report.Findings, Finding{Severity: "warning", Code: "startup.crash_loop", Scope: "startup", Message: "three incomplete startups occurred inside the bounded crash window", Action: "Launch in Safe Mode or roll back the correlated pending update."})
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		order := map[string]int{"error": 0, "warning": 1, "info": 2}
		if order[report.Findings[i].Severity] != order[report.Findings[j].Severity] {
			return order[report.Findings[i].Severity] < order[report.Findings[j].Severity]
		}
		return report.Findings[i].Code < report.Findings[j].Code
	})
	return report, nil
}

func (r Report) HasErrors() bool {
	for _, finding := range r.Findings {
		if finding.Severity == "error" {
			return true
		}
	}
	return false
}

func inspectBinary(role, path string) BinaryStatus {
	status := BinaryStatus{Role: role, Path: path}
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			status.Error = err.Error()
		}
		return status
	}
	status.Exists = true
	status.Regular = info.Mode().IsRegular()
	status.Size = info.Size()
	if !status.Regular {
		status.Error = "path is not a regular file"
		return status
	}
	f, err := os.Open(path)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		status.Error = err.Error()
		return status
	}
	status.SHA256 = hex.EncodeToString(h.Sum(nil))
	return status
}

func inspectSessionStores(root string) []StoreStatus {
	paths := []string{config.SessionDir()}
	if strings.TrimSpace(root) != "" && root != "." {
		if project := config.ProjectSessionDir(root); project != "" && project != paths[0] {
			paths = append(paths, project)
		}
	}
	seen := map[string]bool{}
	var out []StoreStatus
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		status := StoreStatus{Path: path}
		entries, err := os.ReadDir(path)
		if err != nil {
			if !os.IsNotExist(err) {
				status.Error = err.Error()
			}
			out = append(out, status)
			continue
		}
		status.Exists = true
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
				continue
			}
			status.FileCount++
			f, err := os.Open(filepath.Join(path, entry.Name()))
			if err != nil {
				status.Unreadable++
				continue
			}
			if err := f.Close(); err != nil {
				status.Unreadable++
			}
		}
		out = append(out, status)
	}
	return out
}

func inspectPluginStore() StoreStatus {
	home := config.ReamesAgentHomeDir()
	path := pluginpkg.StatePath(home)
	status := StoreStatus{Path: path}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			status.Error = err.Error()
		}
		return status
	}
	status.Exists = true
	state, err := pluginpkg.LoadState(home)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.FileCount = len(state.Plugins)
	for _, plugin := range state.Plugins {
		if plugin.Enabled {
			status.Enabled++
		} else {
			status.Disabled++
		}
	}
	return status
}

func RebuildDerivedState(target string) ([]string, error) {
	paths := derivedStatePaths()
	target = strings.ToLower(strings.TrimSpace(target))
	var names []string
	if target == "all" {
		for name := range paths {
			names = append(names, name)
		}
		sort.Strings(names)
	} else if _, ok := paths[target]; ok {
		names = []string{target}
	} else {
		return nil, fmt.Errorf("unknown derived-state target %q (want tabs|projects|window|zoom|all)", target)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	var applied []string
	for _, name := range names {
		path := paths[name]
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return applied, err
		}
		quarantine := path + ".reames-rebuild-" + stamp
		if err := os.Rename(path, quarantine); err != nil {
			return applied, err
		}
		applied = append(applied, quarantine)
	}
	return applied, nil
}

func derivedStatePaths() map[string]string {
	home := config.ReamesAgentHomeDir()
	state := config.MemoryUserDir()
	return map[string]string{
		"tabs":     filepath.Join(home, "desktop-tabs.json"),
		"projects": filepath.Join(home, "desktop-projects.json"),
		"window":   filepath.Join(state, "desktop-window.json"),
		"zoom":     filepath.Join(state, "desktop-zoom.json"),
	}
}

func MarshalReport(report Report) ([]byte, error) {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// DisableAllPlugins is the Guard's explicit extension quarantine operation.
// The plugin lifecycle owns the atomic state mutation and cross-process lock.
func DisableAllPlugins() ([]string, error) {
	return pluginpkg.DisableAll(config.ReamesAgentHomeDir())
}
