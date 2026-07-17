package repair

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"reames-agent/internal/config"
	"reames-agent/internal/trust"
)

type DisplayOptions struct {
	Root           string
	ExecutablePath string
}

// RedactReportForDisplay preserves the canonical Report schema while removing
// host-specific absolute paths and known secret shapes before crossing a UI or
// diagnostics boundary. The underlying recovery evidence remains unchanged.
func RedactReportForDisplay(report Report, opts DisplayOptions) Report {
	redact := newDisplayRedactor(opts)
	out := report
	out.Startup.Error = redact.text(out.Startup.Error)
	out.Config.Checks = append([]ConfigCheck{}, report.Config.Checks...)
	for i := range out.Config.Checks {
		out.Config.Checks[i].Path = redact.text(out.Config.Checks[i].Path)
		out.Config.Checks[i].Error = redact.text(out.Config.Checks[i].Error)
		out.Config.Checks[i].SnapshotPath = redact.text(out.Config.Checks[i].SnapshotPath)
	}
	out.Config.Applied = redact.strings(report.Config.Applied)
	out.Config.Transaction = redact.transaction(report.Config.Transaction)
	out.ConfigSnapshots = append([]ConfigSnapshot{}, report.ConfigSnapshots...)
	for i := range out.ConfigSnapshots {
		out.ConfigSnapshots[i].Path = redact.text(out.ConfigSnapshots[i].Path)
		out.ConfigSnapshots[i].SourcePath = redact.text(out.ConfigSnapshots[i].SourcePath)
	}
	out.LastRepair = redact.transaction(report.LastRepair)
	out.PendingUpdate = redact.update(report.PendingUpdate)
	out.Binaries = append([]BinaryStatus{}, report.Binaries...)
	for i := range out.Binaries {
		out.Binaries[i].Path = redact.text(out.Binaries[i].Path)
		out.Binaries[i].Error = redact.text(out.Binaries[i].Error)
	}
	out.Sessions = append([]StoreStatus{}, report.Sessions...)
	for i := range out.Sessions {
		out.Sessions[i].Path = redact.text(out.Sessions[i].Path)
		out.Sessions[i].Error = redact.text(out.Sessions[i].Error)
	}
	out.Plugins.Path = redact.text(out.Plugins.Path)
	out.Plugins.Error = redact.text(out.Plugins.Error)
	out.Findings = append([]Finding{}, report.Findings...)
	for i := range out.Findings {
		out.Findings[i].Message = redact.text(out.Findings[i].Message)
		out.Findings[i].Action = redact.text(out.Findings[i].Action)
	}
	return out
}

func RedactTextForDisplay(text string, opts DisplayOptions) string {
	return newDisplayRedactor(opts).text(text)
}

type displayReplacement struct {
	path  string
	label string
}

type displayRedactor struct {
	replacements []displayReplacement
}

func newDisplayRedactor(opts DisplayOptions) displayRedactor {
	var replacements []displayReplacement
	add := func(path, label string) {
		path = strings.TrimSpace(path)
		if path == "" || path == "." {
			return
		}
		if absolute, err := filepath.Abs(path); err == nil {
			path = filepath.Clean(absolute)
		}
		replacements = append(replacements, displayReplacement{path: path, label: label})
	}
	add(config.MemoryUserDir(), "$REAMES_AGENT_STATE")
	add(config.ReamesAgentHomeDir(), "$REAMES_AGENT_HOME")
	add(opts.Root, "$WORKSPACE")
	if executable := strings.TrimSpace(opts.ExecutablePath); executable != "" {
		add(filepath.Dir(executable), "$INSTALL")
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home, "~")
	}
	sort.SliceStable(replacements, func(i, j int) bool { return len(replacements[i].path) > len(replacements[j].path) })
	return displayRedactor{replacements: replacements}
}

func (r displayRedactor) text(value string) string {
	value = trust.RedactSecrets(value)
	for _, replacement := range r.replacements {
		value = replaceDisplayPath(value, replacement.path, replacement.label)
		if filepath.Separator == '\\' {
			value = replaceDisplayPath(value, filepath.ToSlash(replacement.path), replacement.label)
		}
	}
	return value
}

func (r displayRedactor) strings(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = r.text(value)
	}
	return out
}

func (r displayRedactor) transaction(tx *RepairTransaction) *RepairTransaction {
	if tx == nil {
		return nil
	}
	out := *tx
	out.Changes = append([]RepairChange{}, tx.Changes...)
	for i := range out.Changes {
		out.Changes[i].TargetPath = r.text(out.Changes[i].TargetPath)
		out.Changes[i].PreviousPath = r.text(out.Changes[i].PreviousPath)
	}
	return &out
}

func (r displayRedactor) update(tx *UpdateTransaction) *UpdateTransaction {
	if tx == nil {
		return nil
	}
	out := *tx
	out.TargetPath = r.text(out.TargetPath)
	out.BackupPath = r.text(out.BackupPath)
	out.Files = append([]UpdateTransactionFile{}, tx.Files...)
	for i := range out.Files {
		out.Files[i].TargetPath = r.text(out.Files[i].TargetPath)
		out.Files[i].BackupPath = r.text(out.Files[i].BackupPath)
	}
	return &out
}

func replaceDisplayPath(value, path, label string) string {
	if path == "" || value == "" {
		return value
	}
	if runtime.GOOS != "windows" {
		return strings.ReplaceAll(value, path, label)
	}
	lowerValue := strings.ToLower(value)
	lowerPath := strings.ToLower(path)
	for {
		index := strings.Index(lowerValue, lowerPath)
		if index < 0 {
			return value
		}
		value = value[:index] + label + value[index+len(path):]
		lowerValue = strings.ToLower(value)
	}
}
