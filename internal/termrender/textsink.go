// Package termrender renders the shared event stream for terminal frontends.
package termrender

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"

	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

// Renderer redraws completed assistant text as styled terminal output.
type Renderer interface {
	Render(text string) string
}

// TextSink renders a turn's event stream to ANSI text on an io.Writer.
type TextSink struct {
	out       io.Writer
	renderer  Renderer
	termWidth int

	wroteReasoningHeader bool
	wroteReasoningBody   bool
	textWritten          bool
	showReasoning        bool
	wroteAnything        bool
}

// NewTextSink builds a TextSink writing to out. renderer/termWidth drive the
// post-stream markdown redraw; pass a nil renderer to keep the raw stream.
func NewTextSink(out io.Writer, renderer Renderer, termWidth int) *TextSink {
	return &TextSink{out: out, renderer: renderer, termWidth: termWidth}
}

func (s *TextSink) SetShowReasoning(show bool) { s.showReasoning = show }

func (s *TextSink) Emit(e event.Event) {
	switch e.Kind {
	case event.TurnStarted:
		s.wroteReasoningHeader = false
		s.wroteReasoningBody = false
		s.textWritten = false
		s.wroteAnything = false

	case event.Reasoning:
		if !s.wroteReasoningHeader {
			fmt.Fprintln(s.out, dimText("  ▎ thinking"))
			s.wroteReasoningHeader = true
		}
		if s.showReasoning && e.Text != "" {
			fmt.Fprint(s.out, dimText(e.Text))
			s.wroteReasoningBody = true
		}
		s.wroteAnything = true

	case event.Text:
		if s.wroteReasoningHeader && s.wroteReasoningBody && !s.textWritten {
			fmt.Fprintln(s.out)
		}
		fmt.Fprint(s.out, e.Text)
		s.textWritten = true
		s.wroteAnything = true

	case event.Message:
		s.closeTextStream(e.Text, e.Reasoning)

	case event.ToolDispatch:
		if e.Tool.Partial {
			break
		}
		fmt.Fprintf(s.out, "  -> %s %s\n", e.Tool.Name, CompactArgs(e.Tool.Args))
		s.wroteAnything = true

	case event.ToolResult:
		if e.Tool.Err != "" {
			fmt.Fprintf(s.out, "  ⊘ %s %s\n", e.Tool.Name, e.Tool.Err)
			s.wroteAnything = true
		}

	case event.Usage:
		if s.textWritten {
			fmt.Fprintln(s.out)
			s.textWritten = false
		}
		s.usageLine(e.Usage, e.Pricing, e.CacheDiagnostics)

	case event.Notice:
		glyph := "·"
		if e.Level == event.LevelWarn {
			glyph = "!"
		}
		fmt.Fprintf(s.out, "  %s %s\n", glyph, e.Text)
		s.wroteAnything = true

	case event.Phase:
		if s.wroteAnything {
			fmt.Fprintln(s.out)
		}
		fmt.Fprintf(s.out, "[%s]\n", e.Text)
		s.wroteAnything = true

	case event.CompactionStarted:
		fmt.Fprintln(s.out, dimText("  ⋯ compacting conversation…"))
		s.wroteAnything = true

	case event.CompactionDone:
		c := e.Compaction
		if c.Summary == "" {
			break
		}
		fmt.Fprintln(s.out, dimText(fmt.Sprintf("  ⋯ compacted %d messages (%s)", c.Messages, c.Trigger)))
		for _, line := range strings.Split(strings.TrimRight(c.Summary, "\n"), "\n") {
			fmt.Fprintln(s.out, dimText("    "+line))
		}
		s.wroteAnything = true
	}
}

func (s *TextSink) closeTextStream(text, reasoning string) {
	defer func() {
		s.wroteReasoningHeader = false
		s.wroteReasoningBody = false
		s.textWritten = false
	}()
	if len(text) > 0 {
		s.wroteAnything = true
	}
	if len(text) > 0 && s.renderer != nil {
		if moved := streamedRows(text, s.termWidth); moved < 200 {
			if moved == 0 {
				fmt.Fprint(s.out, "\r\033[0J")
			} else {
				fmt.Fprintf(s.out, "\r\033[%dA\033[0J", moved)
			}
			fmt.Fprint(s.out, s.renderer.Render(text))
			return
		}
	}
	if len(text) > 0 || (len(reasoning) > 0 && s.wroteReasoningBody) {
		fmt.Fprintln(s.out)
	}
}

func (s *TextSink) usageLine(usage *provider.Usage, pricing *provider.Pricing, diagnostics *event.CacheDiagnostics) {
	if line := FormatUsageLine(usage, pricing, diagnostics); line != "" {
		fmt.Fprintln(s.out, line)
		s.wroteAnything = true
	}
}

// FormatUsageLine renders a per-turn token/cache summary without a trailing
// newline, or an empty string when usage is unset.
func FormatUsageLine(usage *provider.Usage, pricing *provider.Pricing, diagnostics *event.CacheDiagnostics) string {
	if usage == nil || usage.TotalTokens == 0 {
		return ""
	}
	cache := ""
	if usage.PromptTokens > 0 {
		cached := usage.CacheHitTokens
		fresh := usage.CacheMissTokens
		if fresh == 0 {
			if delta := usage.PromptTokens - cached; delta > 0 {
				fresh = delta
			}
		}
		cache = fmt.Sprintf(" (%d cached / %d new)", cached, fresh)
	}
	reasoning := ""
	if usage.ReasoningTokens > 0 {
		reasoning = fmt.Sprintf(" (%d reasoning)", usage.ReasoningTokens)
	}
	cost := ""
	if pricing != nil {
		cost = fmt.Sprintf(" · %s%.4f", pricing.Symbol(), pricing.Cost(usage))
	}
	churn := ""
	if diagnostics != nil && diagnostics.PrefixChanged {
		reasons := strings.Join(diagnostics.PrefixChangeReasons, "+")
		if reasons == "" {
			reasons = "unknown"
		}
		churn = fmt.Sprintf(" · cache prefix changed: %s", reasons)
	}
	return fmt.Sprintf("  · %d tok · in %d%s · out %d%s%s%s",
		usage.TotalTokens, usage.PromptTokens, cache, usage.CompletionTokens, reasoning, cost, churn)
}

func dimText(text string) string { return "\x1b[2m" + text + "\x1b[0m" }

// CompactArgs trims and caps raw tool JSON for a dispatch line.
func CompactArgs(args string) string {
	args = strings.TrimSpace(args)
	runes := []rune(args)
	if len(runes) > 120 {
		return string(runes[:120]) + "..."
	}
	return args
}

var ansiSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

func visibleWidth(text string) int {
	return runewidth.StringWidth(ansiSGR.ReplaceAllString(text, ""))
}

// streamedRows counts how many rows the cursor descended after raw output.
func streamedRows(text string, width int) int {
	if width <= 0 {
		width = 80
	}
	rows := strings.Count(text, "\n")
	for _, line := range strings.Split(text, "\n") {
		if columns := visibleWidth(line); columns > 0 {
			rows += (columns - 1) / width
		}
	}
	return rows
}
