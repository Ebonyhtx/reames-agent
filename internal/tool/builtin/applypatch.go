package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reames-agent/internal/tool"
)

func init() { tool.RegisterBuiltin(applyPatch{}) }

type applyPatch struct{}

func (applyPatch) Name() string        { return "apply_patch" }
func (applyPatch) ReadOnly() bool      { return false }

func (applyPatch) Description() string {
	return "Apply a unified diff patch to one or more files in the workspace. Returns a summary of files changed, hunks applied, and any failures."
}

func (applyPatch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "patch":{"type":"string","description":"Unified diff content to apply"},
  "dry_run":{"type":"boolean","description":"If true, preview changes without writing files"}
},
"required":["patch"]
}`)
}

func (applyPatch) SnipHint() tool.SnipHint {
	return tool.SnipHint{Head: 20, Tail: 5, HeadChars: 2000, TailChars: 500}
}

func (applyPatch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Patch  string `json:"patch"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("apply_patch: %w", err)
	}
	if p.Patch == "" {
		return "", fmt.Errorf("apply_patch: patch is required")
	}

	// Parse hunks from unified diff
	files, err := parseUnifiedDiff(p.Patch)
	if err != nil {
		return "", fmt.Errorf("apply_patch: %w", err)
	}

	var report strings.Builder
	totalHunks := 0
	failedHunks := 0

	for _, f := range files {
		totalHunks += len(f.Hunks)
		if p.DryRun {
			fmt.Fprintf(&report, "## %s (+%d -%d, %d hunks)\n",
				f.Path, f.Added, f.Removed, len(f.Hunks))
			for _, h := range f.Hunks {
				fmt.Fprintf(&report, "  @@ -%d,%d +%d,%d @@\n",
					h.OldStart, h.OldCount, h.NewStart, h.NewCount)
			}
			report.WriteString("\n")
			continue
		}

		if err := f.apply(); err != nil {
			fmt.Fprintf(&report, "## %s FAILED: %v\n", f.Path, err)
			failedHunks += len(f.Hunks)
			continue
		}
		fmt.Fprintf(&report, "✓ %s (+%d -%d, %d hunks)\n",
			f.Path, f.Added, f.Removed, len(f.Hunks))
	}
	report.WriteString("\n")

	if p.DryRun {
		fmt.Fprintf(&report, "Dry run: %d files, %d hunks would be applied.\n", len(files), totalHunks)
	} else {
		fmt.Fprintf(&report, "Applied: %d files, %d/%d hunks succeeded.\n",
			len(files), totalHunks-failedHunks, totalHunks)
	}

	return strings.TrimSpace(report.String()), nil
}

// parsedFile represents one file's patch from a unified diff.
type parsedFile struct {
	Path    string
	Added   int
	Removed int
	Hunks   []parsedHunk
}

type parsedHunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Lines              []string
}

// parseUnifiedDiff extracts file-level patches from unified diff text.
func parseUnifiedDiff(diff string) ([]parsedFile, error) {
	lines := strings.Split(diff, "\n")
	var files []parsedFile
	var cur *parsedFile
	var curHunk *parsedHunk

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			if strings.HasPrefix(line, "+++ ") {
				path := strings.TrimSpace(line[4:])
				// Strip "b/" prefix common in git diffs
				path = strings.TrimPrefix(path, "b/")
				if cur == nil || cur.Path != path {
					cur = &parsedFile{Path: path}
					files = append(files, *cur)
					cur = &files[len(files)-1]
				}
			}
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if cur == nil {
				return nil, fmt.Errorf("hunk before file header")
			}
			var oldStart, oldCount, newStart, newCount int
			n, err := fmt.Sscanf(line, "@@ -%d,%d +%d,%d", &oldStart, &oldCount, &newStart, &newCount)
			if err != nil || n < 2 {
				n, err = fmt.Sscanf(line, "@@ -%d +%d,%d", &oldStart, &newStart, &newCount)
				if err == nil && n >= 2 {
					oldCount = 1
				}
			}
			if n >= 2 {
				h := parsedHunk{OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount}
				cur.Hunks = append(cur.Hunks, h)
				curHunk = &cur.Hunks[len(cur.Hunks)-1]
			}
			continue
		}
		if curHunk != nil && cur != nil {
			if strings.HasPrefix(line, "+") {
				curHunk.Lines = append(curHunk.Lines, line)
				cur.Added++
			} else if strings.HasPrefix(line, "-") {
				curHunk.Lines = append(curHunk.Lines, line)
				cur.Removed++
			} else if strings.HasPrefix(line, " ") || line == "" {
				curHunk.Lines = append(curHunk.Lines, line)
			}
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in patch")
	}
	return files, nil
}

// apply applies this file's patch hunks.
func (f *parsedFile) apply() error {
	original, err := os.ReadFile(f.Path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", f.Path, err)
	}
	origLines := strings.Split(string(original), "\n")

	var result []string
	origIdx := 0

	for _, hunk := range f.Hunks {
		// Copy lines before this hunk
		for origIdx < hunk.OldStart-1 && origIdx < len(origLines) {
			result = append(result, origLines[origIdx])
			origIdx++
		}

		// Skip old context lines, add new lines
		oldLineIdx := hunk.OldStart - 1
		for _, l := range hunk.Lines {
			if strings.HasPrefix(l, "-") {
				oldLineIdx++
			} else if strings.HasPrefix(l, "+") {
				result = append(result, l[1:])
			} else {
				// Context line — both old and new
				if oldLineIdx < len(origLines) {
					result = append(result, origLines[oldLineIdx])
				}
				oldLineIdx++
			}
		}
		origIdx = oldLineIdx
	}

	// Copy remaining lines
	result = append(result, origLines[origIdx:]...)

	// Ensure directory exists
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	newContent := strings.Join(result, "\n")
	return os.WriteFile(f.Path, []byte(newContent), 0o644)
}
