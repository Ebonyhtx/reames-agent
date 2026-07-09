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
	case "submit":
		return feedbackSubmitCommand(args[1:])
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

type feedbackMetadataFlags []string

func (f *feedbackMetadataFlags) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *feedbackMetadataFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func feedbackSubmitCommand(args []string) int {
	fs := flag.NewFlagSet("feedback submit", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print stored record as JSON")
	home := fs.String("home", "", "Reames Agent home to write")
	kind := fs.String("kind", "feedback", "feedback kind: crash, exception, feedback, performance, bot, metrics")
	source := fs.String("source", "cli", "report source")
	label := fs.String("label", "", "short label")
	version := fs.String("version", "", "application version")
	osName := fs.String("os", "", "operating system")
	arch := fs.String("arch", "", "architecture")
	channel := fs.String("channel", "", "channel name")
	message := fs.String("message", "", "human-readable report message")
	errorType := fs.String("error-type", "", "error type or class")
	errorMessage := fs.String("error-message", "", "error message")
	topFrame := fs.String("top-frame", "", "top stack frame or failure location")
	occurredAt := fs.String("occurred-at", "", "event time")
	var metadata feedbackMetadataFlags
	fs.Var(&metadata, "metadata", "metadata key=value; may be repeated")
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

	record, err := feedback.NewStore("").Append(feedback.ReportInput{
		Kind:         *kind,
		Source:       *source,
		Label:        *label,
		Version:      *version,
		OS:           *osName,
		Arch:         *arch,
		Channel:      *channel,
		Message:      *message,
		ErrorType:    *errorType,
		ErrorMessage: *errorMessage,
		TopFrame:     *topFrame,
		OccurredAt:   *occurredAt,
		Metadata:     parseFeedbackMetadata(metadata),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(record); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}
	fmt.Printf("feedback record stored: id=%s fingerprint=%s kind=%s\n", record.ID, record.Fingerprint, record.Kind)
	return 0
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

func parseFeedbackMetadata(flags []string) map[string]string {
	if len(flags) == 0 {
		return nil
	}
	out := make(map[string]string, len(flags))
	for _, item := range flags {
		key, value, ok := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
  reames-agent feedback submit  [--kind feedback] --message TEXT [--home PATH]
  reames-agent feedback summary [--json] [--limit N] [--home PATH]
  reames-agent feedback draft   [--json] [--limit N] [--home PATH]

Subcommands:
  submit   append one sanitized local feedback record
  summary  show local feedback totals and duplicate clusters
  draft    write a local Markdown maintenance draft; never opens an issue

Examples:
  reames-agent feedback submit --home ~/.reames-agent --message "Feishu delivery failed"
  reames-agent feedback summary --home ~/.reames-agent
  reames-agent feedback draft --home ~/.reames-agent --limit 20
`)
}
