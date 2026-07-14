package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"reames-agent/internal/diff"
	fileenc "reames-agent/internal/fileutil/encoding"
	"reames-agent/internal/tool"
)

func init() { tool.RegisterBuiltin(applyPatch{}) }

const (
	maxPatchBytes = 2 * 1024 * 1024
	maxPatchFiles = 100
)

var unifiedHunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(?: .*)?$`)

type applyPatch struct {
	roots     []string
	guard     SessionDataGuard
	workDir   string
	writePlan func(*patchPlan) error
}

func (applyPatch) Name() string   { return "apply_patch" }
func (applyPatch) ReadOnly() bool { return false }

func (applyPatch) Description() string {
	return "Apply a unified diff patch to one or more files in the workspace as one validated transaction. Supports create, update, delete, dry-run preview, checkpointing, and rollback when a later file fails."
}

func (applyPatch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "patch":{"type":"string","description":"Unified diff content to apply"},
  "dry_run":{"type":"boolean","description":"If true, validate and preview changes without writing files"}
},
"required":["patch"]
}`)
}

func (applyPatch) SnipHint() tool.SnipHint {
	return tool.SnipHint{Head: 20, Tail: 5, HeadChars: 2000, TailChars: 500}
}

type applyPatchArgs struct {
	Patch  string `json:"patch"`
	DryRun bool   `json:"dry_run"`
}

func parseApplyPatchArgs(args json.RawMessage) (applyPatchArgs, error) {
	var p applyPatchArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return p, fmt.Errorf("apply_patch: invalid args: %w", err)
	}
	if strings.TrimSpace(p.Patch) == "" {
		return p, fmt.Errorf("apply_patch: patch is required")
	}
	if len([]byte(p.Patch)) > maxPatchBytes {
		return p, fmt.Errorf("apply_patch: patch exceeds %d bytes", maxPatchBytes)
	}
	return p, nil
}

func (a applyPatch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	p, err := parseApplyPatchArgs(args)
	if err != nil {
		return "", err
	}
	plans, err := a.buildPlans(p.Patch)
	if err != nil {
		return "", err
	}
	defer closePatchPlans(plans)
	if p.DryRun {
		return patchSummary(plans, true), nil
	}

	applied := make([]*patchPlan, 0, len(plans))
	for _, plan := range plans {
		writePlan := a.writePlan
		if writePlan == nil {
			writePlan = writePatchPlan
		}
		applyErr := writePlan(plan)
		if applyErr != nil {
			rollbackErr := rollbackPatchPlans(applied)
			if rollbackErr != nil {
				applyErr = errors.Join(applyErr, fmt.Errorf("rollback: %w", rollbackErr))
			}
			return "", fmt.Errorf("apply_patch: write failed for %s; rolled back: %w", plan.patch.Path, applyErr)
		}
		applied = append(applied, plan)
	}
	return patchSummary(plans, false), nil
}

func writePatchPlan(plan *patchPlan) error {
	if plan.newContent == nil {
		return plan.target.Remove()
	}
	return plan.target.replaceBytes(fileenc.Encode(*plan.newContent, plan.encoding), plan.mode)
}

// PreviewChanges implements tool.MultiPreviewer. Every affected file is
// returned so the agent persists all pre-edit snapshots before Execute starts.
func (a applyPatch) PreviewChanges(args json.RawMessage) ([]diff.Change, error) {
	p, err := parseApplyPatchArgs(args)
	if err != nil {
		return nil, err
	}
	if p.DryRun {
		return nil, nil
	}
	plans, err := a.buildPlans(p.Patch)
	if err != nil {
		return nil, err
	}
	defer closePatchPlans(plans)
	changes := make([]diff.Change, len(plans))
	for i, plan := range plans {
		changes[i] = plan.change
	}
	return changes, nil
}

type patchLine struct {
	Kind      byte
	Text      string
	NoNewline bool
}

type parsedHunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Lines              []patchLine
}

// parsedFile represents one file's patch from a unified diff. Path is the
// mutation target (the old path for deletes, new path otherwise).
type parsedFile struct {
	OldPath string
	NewPath string
	Path    string
	Added   int
	Removed int
	Hunks   []parsedHunk
}

func (f parsedFile) action() string {
	switch {
	case f.OldPath == "/dev/null":
		return "create"
	case f.NewPath == "/dev/null":
		return "delete"
	default:
		return "update"
	}
}

func parseUnifiedDiff(patch string) ([]parsedFile, error) {
	patch = strings.ReplaceAll(patch, "\r\n", "\n")
	lines := strings.Split(patch, "\n")
	files := make([]parsedFile, 0)
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "--- ") {
			i++
			continue
		}
		oldPath, err := normalizePatchPath(strings.TrimPrefix(lines[i], "--- "))
		if err != nil {
			return nil, err
		}
		i++
		if i >= len(lines) || !strings.HasPrefix(lines[i], "+++ ") {
			return nil, fmt.Errorf("expected +++ header after --- %s", oldPath)
		}
		newPath, err := normalizePatchPath(strings.TrimPrefix(lines[i], "+++ "))
		if err != nil {
			return nil, err
		}
		i++
		if oldPath == "/dev/null" && newPath == "/dev/null" {
			return nil, fmt.Errorf("invalid /dev/null to /dev/null patch")
		}
		if oldPath != "/dev/null" && newPath != "/dev/null" && filepath.Clean(oldPath) != filepath.Clean(newPath) {
			return nil, fmt.Errorf("rename patch from %s to %s is not supported; use move_file, then patch the destination", oldPath, newPath)
		}
		targetPath := newPath
		if newPath == "/dev/null" {
			targetPath = oldPath
		}
		file := parsedFile{OldPath: oldPath, NewPath: newPath, Path: targetPath}
		for i < len(lines) && !strings.HasPrefix(lines[i], "--- ") {
			if strings.TrimSpace(lines[i]) == "" {
				i++
				continue
			}
			match := unifiedHunkHeader.FindStringSubmatch(lines[i])
			if match == nil {
				return nil, fmt.Errorf("invalid hunk header for %s: %q", targetPath, lines[i])
			}
			hunk, err := parsedHunkFromHeader(match)
			if err != nil {
				return nil, fmt.Errorf("invalid hunk header for %s: %w", targetPath, err)
			}
			i++
			for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") && !strings.HasPrefix(lines[i], "--- ") {
				line := lines[i]
				if line == `\ No newline at end of file` {
					if len(hunk.Lines) == 0 {
						return nil, fmt.Errorf("newline marker before content in %s", targetPath)
					}
					hunk.Lines[len(hunk.Lines)-1].NoNewline = true
					i++
					continue
				}
				if line == "" {
					if i == len(lines)-1 {
						i++
						break
					}
					return nil, fmt.Errorf("malformed empty hunk line for %s", targetPath)
				}
				kind := line[0]
				if kind != ' ' && kind != '+' && kind != '-' {
					return nil, fmt.Errorf("invalid hunk line prefix %q for %s", kind, targetPath)
				}
				hunk.Lines = append(hunk.Lines, patchLine{Kind: kind, Text: line[1:]})
				switch kind {
				case '+':
					file.Added++
				case '-':
					file.Removed++
				}
				i++
			}
			if err := validateHunkCounts(hunk); err != nil {
				return nil, fmt.Errorf("invalid hunk for %s: %w", targetPath, err)
			}
			file.Hunks = append(file.Hunks, hunk)
		}
		if len(file.Hunks) == 0 {
			return nil, fmt.Errorf("no hunks for %s", targetPath)
		}
		files = append(files, file)
		if len(files) > maxPatchFiles {
			return nil, fmt.Errorf("too many files (max %d)", maxPatchFiles)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no unified diff file headers found")
	}
	return files, nil
}

func normalizePatchPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if tab := strings.IndexByte(path, '\t'); tab >= 0 {
		path = path[:tab]
	}
	if strings.HasPrefix(path, `"`) {
		unquoted, err := strconv.Unquote(path)
		if err != nil {
			return "", fmt.Errorf("invalid quoted patch path %q: %w", path, err)
		}
		path = unquoted
	}
	if path == "/dev/null" {
		return path, nil
	}
	path = strings.TrimPrefix(strings.TrimPrefix(path, "a/"), "b/")
	path = filepath.FromSlash(path)
	if strings.TrimSpace(path) == "" || path == "." {
		return "", fmt.Errorf("invalid empty patch path")
	}
	return path, nil
}

func parsedHunkFromHeader(match []string) (parsedHunk, error) {
	values := make([]int, 4)
	for i, raw := range match[1:] {
		if raw == "" {
			values[i] = 1
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return parsedHunk{}, err
		}
		values[i] = value
	}
	return parsedHunk{OldStart: values[0], OldCount: values[1], NewStart: values[2], NewCount: values[3]}, nil
}

func validateHunkCounts(h parsedHunk) error {
	oldCount, newCount := 0, 0
	for _, line := range h.Lines {
		if line.Kind != '+' {
			oldCount++
		}
		if line.Kind != '-' {
			newCount++
		}
	}
	if oldCount != h.OldCount || newCount != h.NewCount {
		return fmt.Errorf("header counts old=%d new=%d do not match body old=%d new=%d", h.OldCount, h.NewCount, oldCount, newCount)
	}
	return nil
}

type patchPlan struct {
	patch          parsedFile
	target         *rootedTarget
	originalExists bool
	originalBytes  []byte
	originalText   string
	newContent     *string
	encoding       fileenc.Kind
	mode           os.FileMode
	change         diff.Change
}

func (a applyPatch) buildPlans(patch string) ([]*patchPlan, error) {
	files, err := parseUnifiedDiff(patch)
	if err != nil {
		return nil, fmt.Errorf("apply_patch: %w", err)
	}
	roots := a.roots
	if len(roots) == 0 {
		base := a.workDir
		if base == "" {
			base, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("apply_patch: resolve workspace: %w", err)
			}
		}
		roots = realRoots([]string{base})
	}
	plans := make([]*patchPlan, 0, len(files))
	seen := map[string]bool{}
	fail := func(err error) ([]*patchPlan, error) {
		closePatchPlans(plans)
		return nil, err
	}
	for _, file := range files {
		target, openErr := openRootedWriteTarget(roots, a.guard, resolveIn(a.workDir, file.Path))
		if openErr != nil {
			return fail(fmt.Errorf("apply_patch: %w", openErr))
		}
		identityPath := target.resolved
		if identityPath == "" {
			identityPath = target.path
		}
		identity := canonicalWritableIdentity(identityPath)
		if seen[identity] {
			target.Close()
			return fail(fmt.Errorf("apply_patch: duplicate file patch for %s", file.Path))
		}
		seen[identity] = true
		plan := &patchPlan{patch: file, target: target, encoding: fileenc.UTF8, mode: 0o644}
		info, statErr := target.Stat()
		switch {
		case statErr == nil:
			if !info.Mode().IsRegular() {
				target.Close()
				return fail(fmt.Errorf("apply_patch: refusing to patch non-regular file %s", file.Path))
			}
			plan.originalExists = true
			plan.mode = info.Mode().Perm()
			plan.originalBytes, err = target.ReadFile()
			if err != nil {
				target.Close()
				return fail(fmt.Errorf("apply_patch: read %s: %w", file.Path, err))
			}
			plan.encoding, _ = fileenc.Detect(plan.originalBytes)
			plan.originalText = string(fileenc.Decode(plan.originalBytes, plan.encoding))
		case os.IsNotExist(statErr):
		default:
			target.Close()
			return fail(fmt.Errorf("apply_patch: stat %s: %w", file.Path, statErr))
		}
		switch file.action() {
		case "create":
			if plan.originalExists {
				target.Close()
				return fail(fmt.Errorf("apply_patch: refusing to create existing file %s", file.Path))
			}
		case "update", "delete":
			if !plan.originalExists {
				target.Close()
				return fail(fmt.Errorf("apply_patch: file not found %s", file.Path))
			}
		}
		updated, applyErr := applyPatchToText(plan.originalText, file)
		if applyErr != nil {
			target.Close()
			return fail(fmt.Errorf("apply_patch: %w", applyErr))
		}
		kind := diff.Modify
		if file.action() == "create" {
			kind = diff.Create
		}
		if file.action() == "delete" {
			if updated != "" {
				target.Close()
				return fail(fmt.Errorf("apply_patch: delete patch for %s does not remove the complete file", file.Path))
			}
			kind = diff.Delete
			plan.newContent = nil
			plan.change = diff.Build(target.path, plan.originalText, "", kind)
		} else {
			plan.newContent = &updated
			plan.change = diff.Build(target.path, plan.originalText, updated, kind)
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func canonicalWritableIdentity(path string) string {
	path = filepath.Clean(path)
	if foldPaths {
		path = strings.ToLower(path)
	}
	return path
}

func applyPatchToText(original string, file parsedFile) (string, error) {
	lineSep := "\n"
	if strings.Contains(original, "\r\n") {
		lineSep = "\r\n"
	}
	normalized := strings.ReplaceAll(original, "\r\n", "\n")
	originalEndsNewline := strings.HasSuffix(normalized, "\n")
	if originalEndsNewline {
		normalized = strings.TrimSuffix(normalized, "\n")
	}
	var originalLines []string
	if normalized != "" {
		originalLines = strings.Split(normalized, "\n")
	}
	result := make([]string, 0, len(originalLines)+file.Added-file.Removed)
	cursor := 0
	newEndsNewline := originalEndsNewline
	for _, hunk := range file.Hunks {
		start := hunk.OldStart - 1
		if hunk.OldStart == 0 {
			start = 0
		}
		if start < cursor {
			return "", fmt.Errorf("overlapping hunk for %s", file.Path)
		}
		if start > len(originalLines) {
			return "", fmt.Errorf("hunk starts beyond end of %s", file.Path)
		}
		result = append(result, originalLines[cursor:start]...)
		cursor = start
		for _, line := range hunk.Lines {
			switch line.Kind {
			case ' ':
				if cursor >= len(originalLines) || originalLines[cursor] != line.Text {
					return "", fmt.Errorf("context mismatch in %s at line %d", file.Path, cursor+1)
				}
				result = append(result, originalLines[cursor])
				cursor++
			case '-':
				if cursor >= len(originalLines) || originalLines[cursor] != line.Text {
					return "", fmt.Errorf("removal mismatch in %s at line %d", file.Path, cursor+1)
				}
				cursor++
			case '+':
				result = append(result, line.Text)
			}
		}
		if cursor == len(originalLines) {
			newEndsNewline = true
			for i := len(hunk.Lines) - 1; i >= 0; i-- {
				if hunk.Lines[i].Kind != '-' {
					newEndsNewline = !hunk.Lines[i].NoNewline
					break
				}
			}
		}
	}
	result = append(result, originalLines[cursor:]...)
	if len(result) == 0 {
		return "", nil
	}
	updated := strings.Join(result, lineSep)
	if newEndsNewline {
		updated += lineSep
	}
	return updated, nil
}

func closePatchPlans(plans []*patchPlan) {
	for _, plan := range plans {
		if plan != nil && plan.target != nil {
			_ = plan.target.Close()
		}
	}
}

func rollbackPatchPlans(plans []*patchPlan) error {
	var rollbackErr error
	for i := len(plans) - 1; i >= 0; i-- {
		plan := plans[i]
		if plan.originalExists {
			if err := plan.target.replaceBytes(plan.originalBytes, plan.mode); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %s: %w", plan.patch.Path, err))
			}
			continue
		}
		if err := plan.target.Remove(); err != nil && !os.IsNotExist(err) {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove %s: %w", plan.patch.Path, err))
		}
	}
	return rollbackErr
}

func patchSummary(plans []*patchPlan, preview bool) string {
	mode := "applied"
	if preview {
		mode = "preview"
	}
	hunks := 0
	for _, plan := range plans {
		hunks += len(plan.patch.Hunks)
	}
	var report strings.Builder
	fmt.Fprintf(&report, "[apply_patch] %s: %d file(s), %d hunk(s)", mode, len(plans), hunks)
	for _, plan := range plans {
		before, after := len(plan.originalBytes), 0
		if plan.newContent != nil {
			after = len(fileenc.Encode(*plan.newContent, plan.encoding))
		}
		fmt.Fprintf(&report, "\n  %s: %s (%d -> %d bytes)", plan.patch.action(), plan.patch.Path, before, after)
	}
	return report.String()
}
