package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"reames-agent/internal/feedback"
)

func feedbackCommand(args []string) int {
	if len(args) == 0 {
		feedbackUsage()
		return 2
	}
	switch args[0] {
	case "summary":
		return feedbackSummaryCommand(args[1:])
	case "draft":
		return feedbackDraftCommand(args[1:])
	case "help", "--help", "-h":
		feedbackUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown feedback subcommand %q\n\n", args[0])
		feedbackUsage()
		return 2
	}
}

func feedbackSummaryCommand(args []string) int {
	fs := flag.NewFlagSet("feedback summary", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print summary as JSON")
	limit := fs.Int("limit", 50, "maximum duplicate groups to show")
	home := fs.String("home", "", "Reames Agent home to inspect")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		feedbackUsage()
		return 2
	}
	restore, err := setTemporaryReamesAgentHome(*home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer restore()

	summary, err := feedback.NewStore("").Summary(feedbackCLILimit(*limit))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}
	printFeedbackSummary(summary)
	return 0
}

func feedbackDraftCommand(args []string) int {
	fs := flag.NewFlagSet("feedback draft", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print draft metadata and markdown as JSON")
	limit := fs.Int("limit", 50, "maximum duplicate groups to include")
	home := fs.String("home", "", "Reames Agent home to inspect")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		feedbackUsage()
		return 2
	}
	restore, err := setTemporaryReamesAgentHome(*home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer restore()

	draft, err := feedback.NewStore("").WriteDraft(feedbackCLILimit(*limit))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(draft); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}
	fmt.Printf("feedback maintenance draft written: %s\n", draft.Path)
	fmt.Printf("records=%d groups=%d created_at=%s\n", draft.Total, draft.Groups, draft.CreatedAt)
	return 0
}

func feedbackCLILimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func printFeedbackSummary(summary feedback.Summary) {
	fmt.Println("feedback summary")
	fmt.Println("ledger:", summary.Path)
	fmt.Println("total records:", summary.Total)
	fmt.Println("duplicate groups:", len(summary.Groups))
	for i, group := range summary.Groups {
		parts := []string{
			fmt.Sprintf("#%d", i+1),
			fmt.Sprintf("count=%d", group.Count),
			"kind=" + group.Kind,
			"fingerprint=" + group.Fingerprint,
		}
		if group.Label != "" {
			parts = append(parts, "label="+group.Label)
		}
		if group.ErrorType != "" {
			parts = append(parts, "error_type="+group.ErrorType)
		}
		if group.TopFrame != "" {
			parts = append(parts, "top_frame="+group.TopFrame)
		}
		fmt.Println("- " + strings.Join(parts, " "))
	}
}

func feedbackUsage() {
	fmt.Print(`reames-agent feedback — inspect sanitized self-hosted feedback

Usage:
  reames-agent feedback summary [--json] [--limit N] [--home PATH]
  reames-agent feedback draft   [--json] [--limit N] [--home PATH]

Subcommands:
  summary  show local feedback totals and duplicate clusters
  draft    write a local Markdown maintenance draft; never opens an issue

Examples:
  reames-agent feedback summary --home ~/.reames-agent
  reames-agent feedback draft --home ~/.reames-agent --limit 20
`)
}
