package cli

import (
	"fmt"
	"strings"
)

// ToolEmoji maps tool names to display emoji for compact status lines.
var ToolEmoji = map[string]string{
	"bash":          "⚡",
	"bash_output":   "📋",
	"kill_shell":    "⏹",
	"wait":          "⏳",
	"list_jobs":     "📊",
	"read_file":     "📖",
	"write_file":    "✏️",
	"edit_file":     "✂️",
	"multi_edit":    "✂️",
	"apply_patch":   "🩹",
	"delete_file":   "🗑",
	"delete_range":  "🗑",
	"delete_symbol": "🗑",
	"move_file":     "📦",
	"glob":          "🔍",
	"grep":          "🔎",
	"ls":            "📂",
	"web_fetch":     "🌐",
	"web_search":    "🔎",
	"code_index":    "🏷",
	"todo_write":    "✅",
	"complete_step": "✔️",
	"cronjob":       "⏰",
	"ask_user":      "❓",
	"task":          "🤖",
	"read_only_task": "👁",
	"notebook_edit": "📓",
}

// ToolVerb maps tool names to action verbs for status lines.
var ToolVerb = map[string]string{
	"bash":          "run",
	"bash_output":   "read",
	"kill_shell":    "kill",
	"wait":          "wait",
	"list_jobs":     "list",
	"read_file":     "read",
	"write_file":    "write",
	"edit_file":     "edit",
	"multi_edit":    "edit",
	"apply_patch":   "patch",
	"delete_file":   "delete",
	"delete_range":  "delete",
	"delete_symbol": "delete",
	"move_file":     "move",
	"glob":          "glob",
	"grep":          "grep",
	"ls":            "ls",
	"web_fetch":     "fetch",
	"web_search":    "search",
	"code_index":    "index",
	"todo_write":    "plan",
	"complete_step": "done",
	"cronjob":       "cron",
	"ask_user":      "ask",
	"task":          "task",
}

// FormatToolStatus returns a compact status line for a completed tool call.
// Format: "⚡ run    make build     (1.2s)"
func FormatToolStatus(name, subject string, durationMs int64, failed bool) string {
	emoji := ToolEmoji[name]
	if emoji == "" {
		emoji = "🔧"
	}
	verb := ToolVerb[name]
	if verb == "" {
		verb = name
	}

	var b strings.Builder
	b.WriteString(emoji)
	b.WriteString(" ")
	b.WriteString(verb)

	// Pad verb to 6 chars for alignment
	pad := 6 - len(verb)
	if pad > 0 {
		b.WriteString(strings.Repeat(" ", pad))
	}

	if subject != "" {
		// Truncate subject to 40 chars
		s := subject
		if len(s) > 40 {
			s = s[:37] + "..."
		}
		b.WriteString(" ")
		b.WriteString(s)
	}

	if durationMs > 0 {
		d := float64(durationMs) / 1000.0
		b.WriteString(fmt.Sprintf("  (%.1fs)", d))
	}

	if failed {
		b.WriteString("  ❌ FAILED")
	}

	return b.String()
}
