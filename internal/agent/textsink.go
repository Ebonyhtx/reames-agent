package agent

import (
	"io"

	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/termrender"
)

// Renderer is retained as an internal compatibility alias. Terminal rendering
// is implemented by termrender rather than the agent runtime.
type Renderer = termrender.Renderer

type TextSink = termrender.TextSink

func NewTextSink(out io.Writer, renderer Renderer, termWidth int) *TextSink {
	return termrender.NewTextSink(out, renderer, termWidth)
}

func FormatUsageLine(usage *provider.Usage, pricing *provider.Pricing, diagnostics *event.CacheDiagnostics) string {
	return termrender.FormatUsageLine(usage, pricing, diagnostics)
}

func CompactArgs(args string) string { return termrender.CompactArgs(args) }
