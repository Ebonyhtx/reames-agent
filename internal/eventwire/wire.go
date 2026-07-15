// Package eventwire defines the shared frontend JSON contract for event.Event.
package eventwire

import (
	"maps"

	"reames-agent/internal/control"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

// Version is the current stable event-envelope version. Additive fields remain
// backward compatible; incompatible semantic changes require a new version.
const Version = 1

// Event is the JSON-friendly form shared by event frontends.
type Event struct {
	Version           int                `json:"version"`
	Kind              string             `json:"kind"`
	Source            string             `json:"source,omitempty"`
	Text              string             `json:"text,omitempty"`
	Reasoning         string             `json:"reasoning,omitempty"`
	MemoryCitations   []MemoryCitation   `json:"memoryCitations,omitempty"`
	MemoryCompiler    *MemoryCompiler    `json:"memoryCompiler,omitempty"`
	Level             string             `json:"level,omitempty"`
	Tool              *Tool              `json:"tool,omitempty"`
	Usage             *Usage             `json:"usage,omitempty"`
	Approval          *Approval          `json:"approval,omitempty"`
	Ask               *Ask               `json:"ask,omitempty"`
	Compaction        *Compaction        `json:"compaction,omitempty"`
	Guardian          *Guardian          `json:"guardian,omitempty"`
	Err               string             `json:"err,omitempty"`
	Error             *control.ErrorInfo `json:"error,omitempty"`
	RetryAttempt      int                `json:"retryAttempt,omitempty"`
	RetryMax          int                `json:"retryMax,omitempty"`
	CacheDiagnostics  *CacheDiagnostics  `json:"cacheDiagnostics,omitempty"`
	SessionHitTokens  int                `json:"sessionHitTokens,omitempty"`
	SessionMissTokens int                `json:"sessionMissTokens,omitempty"`
}

// ToWire converts a typed runtime event into the shared frontend JSON contract.
func ToWire(e event.Event) Event {
	w := Event{
		Version: Version, Kind: kindNames[e.Kind], Source: e.Source,
		Text: e.Text, Reasoning: e.Reasoning,
		SessionHitTokens: e.SessionHit, SessionMissTokens: e.SessionMiss,
	}
	if len(e.MemoryCitations) > 0 {
		w.MemoryCitations = ToWireMemoryCitations(e.MemoryCitations)
	}
	switch e.Kind {
	case event.Notice:
		if e.Level == event.LevelWarn {
			w.Level = "warn"
		} else {
			w.Level = "info"
		}
	case event.ToolDispatch, event.ToolResult, event.ToolProgress:
		wt := &Tool{
			ID: e.Tool.ID, Name: e.Tool.Name, Args: e.Tool.Args,
			Output: e.Tool.Output, Err: e.Tool.Err,
			ReadOnly: e.Tool.ReadOnly, Truncated: e.Tool.Truncated,
			DurationMs: e.Tool.DurationMs, Partial: e.Tool.Partial,
			ParentID: e.Tool.ParentID,
			Diff:     e.Tool.Diff, Added: e.Tool.Added, Removed: e.Tool.Removed,
		}
		if e.Tool.Profile != nil {
			wt.Profile = &Profile{Model: e.Tool.Profile.Model, Effort: e.Tool.Profile.Effort}
		}
		w.Tool = wt
	case event.Usage:
		if u := e.Usage; u != nil {
			w.Usage = &Usage{
				PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
				TotalTokens: u.TotalTokens, CacheHitTokens: u.CacheHitTokens,
				CacheMissTokens: u.CacheMissTokens, ReasoningTokens: u.ReasoningTokens,
				Source:                e.UsageSource,
				SessionCacheHitTokens: e.SessionHit, SessionCacheMissTokens: e.SessionMiss,
			}
			if e.CacheDiagnostics != nil {
				w.Usage.CacheDiagnostics = ToWireCacheDiagnostics(e.CacheDiagnostics)
			}
			if e.Pricing != nil {
				cost := e.Pricing.Cost(u)
				w.Usage.Cost = cost
				w.Usage.Currency = e.Pricing.Symbol()
				w.Usage.CostUSD = cost
			}
		}
	case event.MemoryCompilerStatsEvent:
		if m := e.MemoryCompiler; m != nil {
			w.MemoryCompiler = &MemoryCompiler{
				Injected:         m.Injected,
				UsefulIR:         m.UsefulIR,
				CompiledTokens:   m.CompiledTokens,
				IROverheadTokens: m.IROverheadTokens,
				MemoryReferences: m.MemoryReferences,
				Constraints:      m.Constraints,
				RiskNotes:        m.RiskNotes,
				ExecutionSteps:   m.ExecutionSteps,
				TotalNodes:       m.TotalNodes,
				HighSignalNodes:  m.HighSignalNodes,
				ToolResultNodes:  m.ToolResultNodes,
				DecisionNodes:    m.DecisionNodes,
				StrategyCount:    m.StrategyCount,
				LearningCount:    m.LearningCount,
			}
		}
	case event.ApprovalRequest:
		w.Approval = &Approval{
			ID: e.Approval.ID, Tool: e.Approval.Tool, Subject: e.Approval.Subject, Reason: e.Approval.Reason,
			Diff: e.Approval.Diff, Added: e.Approval.Added, Removed: e.Approval.Removed,
		}
		if e.Approval.Plan != nil {
			w.Approval.Plan = toWireApprovalPlan(e.Approval.Plan)
		}
	case event.AskRequest:
		w.Ask = ToWireAsk(e.Ask)
	case event.CompactionStarted, event.CompactionDone:
		w.Compaction = &Compaction{
			Trigger: e.Compaction.Trigger, Messages: e.Compaction.Messages,
			Summary: e.Compaction.Summary, Archive: e.Compaction.Archive,
		}
	case event.GuardianAssessment:
		w.Guardian = ToWireGuardian(e.Guardian)
	case event.TurnDone:
		if e.Err != nil {
			w.Err = e.Err.Error()
			info := control.ClassifyError(e.Err)
			w.Error = &info
		}
	case event.Retrying:
		w.RetryAttempt = e.RetryAttempt
		w.RetryMax = e.RetryMax
	case event.CacheUpdated:
		if e.CacheDiagnostics != nil {
			w.CacheDiagnostics = ToWireCacheDiagnostics(e.CacheDiagnostics)
		}
	}
	return w
}

// MemoryCitation is the JSON form of provider.MemoryCitation.
type MemoryCitation struct {
	ID        string `json:"id,omitempty"`
	Source    string `json:"source"`
	LineStart int    `json:"lineStart,omitempty"`
	LineEnd   int    `json:"lineEnd,omitempty"`
	Note      string `json:"note,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

// MemoryCompiler is the JSON form of content-free Memory v5 usage metrics.
type MemoryCompiler struct {
	Injected         bool `json:"injected"`
	UsefulIR         bool `json:"usefulIR"`
	CompiledTokens   int  `json:"compiledTokens"`
	IROverheadTokens int  `json:"irOverheadTokens"`
	MemoryReferences int  `json:"memoryReferences"`
	Constraints      int  `json:"constraints"`
	RiskNotes        int  `json:"riskNotes"`
	ExecutionSteps   int  `json:"executionSteps"`
	TotalNodes       int  `json:"totalNodes"`
	HighSignalNodes  int  `json:"highSignalNodes"`
	ToolResultNodes  int  `json:"toolResultNodes"`
	DecisionNodes    int  `json:"decisionNodes"`
	StrategyCount    int  `json:"strategyCount"`
	LearningCount    int  `json:"learningCount"`
}

// ToWireMemoryCitations converts local memory references into frontend JSON.
func ToWireMemoryCitations(in []provider.MemoryCitation) []MemoryCitation {
	out := make([]MemoryCitation, 0, len(in))
	for _, c := range in {
		if c.Source == "" && c.ID == "" && c.Note == "" {
			continue
		}
		out = append(out, MemoryCitation{
			ID:        c.ID,
			Source:    c.Source,
			LineStart: c.LineStart,
			LineEnd:   c.LineEnd,
			Note:      c.Note,
			Kind:      c.Kind,
		})
	}
	return out
}

// Compaction is the JSON form of an event.Compaction.
type Compaction struct {
	Trigger  string `json:"trigger,omitempty"`
	Messages int    `json:"messages,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Archive  string `json:"archive,omitempty"`
}

// AskOption is one JSON-formatted choice in a structured ask request.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskQuestion is one JSON-formatted structured ask question.
type AskQuestion struct {
	ID      string      `json:"id"`
	Header  string      `json:"header,omitempty"`
	Prompt  string      `json:"prompt"`
	Options []AskOption `json:"options"`
	Multi   bool        `json:"multi,omitempty"`
}

// Ask is the JSON form of an event.Ask.
type Ask struct {
	ID        string        `json:"id"`
	Questions []AskQuestion `json:"questions"`
}

// Profile carries the subagent model/effort resolved for a tool call.
type Profile struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// Tool is the JSON form of an event.Tool.
type Tool struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Args       string   `json:"args,omitempty"`
	Output     string   `json:"output,omitempty"`
	Err        string   `json:"err,omitempty"`
	ReadOnly   bool     `json:"readOnly"`
	Truncated  bool     `json:"truncated,omitempty"`
	DurationMs int64    `json:"durationMs,omitempty"`
	Partial    bool     `json:"partial,omitempty"`
	ParentID   string   `json:"parentId,omitempty"`
	Diff       string   `json:"diff,omitempty"`
	Added      int      `json:"added,omitempty"`
	Removed    int      `json:"removed,omitempty"`
	Profile    *Profile `json:"profile,omitempty"`
}

// Usage is the JSON form of provider usage telemetry.
type Usage struct {
	PromptTokens     int               `json:"promptTokens"`
	CompletionTokens int               `json:"completionTokens"`
	TotalTokens      int               `json:"totalTokens"`
	CacheHitTokens   int               `json:"cacheHitTokens"`
	CacheMissTokens  int               `json:"cacheMissTokens"`
	ReasoningTokens  int               `json:"reasoningTokens,omitempty"`
	Source           string            `json:"source,omitempty"`
	CacheDiagnostics *CacheDiagnostics `json:"cacheDiagnostics,omitempty"`
	// Session-cumulative cache tokens keep status displays steadier than one-turn values.
	SessionCacheHitTokens  int     `json:"sessionCacheHitTokens"`
	SessionCacheMissTokens int     `json:"sessionCacheMissTokens"`
	Cost                   float64 `json:"cost,omitempty"`
	Currency               string  `json:"currency,omitempty"`
	// CostUSD is a compatibility alias for older consumers; it mirrors Cost.
	CostUSD float64 `json:"costUsd,omitempty"`
}

// CacheDiagnostics is the JSON form of cache prefix diagnostics.
type CacheDiagnostics struct {
	PrefixHash          string   `json:"prefixHash"`
	PrefixChanged       bool     `json:"prefixChanged"`
	PrefixChangeReasons []string `json:"prefixChangeReasons,omitempty"`
	SystemHash          string   `json:"systemHash"`
	ToolsHash           string   `json:"toolsHash"`
	LogRewriteVersion   int      `json:"logRewriteVersion"`
	ToolSchemaTokens    int      `json:"toolSchemaTokens"`
	CacheMissTokens     int      `json:"cacheMissTokens"`
	CacheHitTokens      int      `json:"cacheHitTokens"`
}

// Approval is the JSON form of an event.Approval.
type Approval struct {
	ID      string        `json:"id"`
	Tool    string        `json:"tool"`
	Subject string        `json:"subject"`
	Reason  string        `json:"reason,omitempty"`
	Plan    *ApprovalPlan `json:"plan,omitempty"`
	Diff    string        `json:"diff,omitempty"`
	Added   int           `json:"added,omitempty"`
	Removed int           `json:"removed,omitempty"`
}

type ApprovalPlan struct {
	PlanID    string           `json:"planId"`
	Operation string           `json:"operation"`
	Source    string           `json:"source,omitempty"`
	Name      string           `json:"name,omitempty"`
	Kind      string           `json:"kind,omitempty"`
	Scope     string           `json:"scope,omitempty"`
	Mode      string           `json:"mode,omitempty"`
	Actions   []ApprovalAction `json:"actions"`
	Warnings  []string         `json:"warnings,omitempty"`
}

type ApprovalAction struct {
	Kind               string            `json:"kind"`
	Action             string            `json:"action"`
	RiskLevel          string            `json:"riskLevel"`
	RiskReasons        []string          `json:"riskReasons,omitempty"`
	Name               string            `json:"name,omitempty"`
	Source             string            `json:"source,omitempty"`
	Target             string            `json:"target,omitempty"`
	ConfigPath         string            `json:"configPath,omitempty"`
	Scope              string            `json:"scope,omitempty"`
	Mode               string            `json:"mode,omitempty"`
	Transport          string            `json:"transport,omitempty"`
	URL                string            `json:"url,omitempty"`
	Command            string            `json:"command,omitempty"`
	Args               []string          `json:"args,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	Permissions        []string          `json:"permissions,omitempty"`
	AddedPermissions   []string          `json:"addedPermissions,omitempty"`
	RemovedPermissions []string          `json:"removedPermissions,omitempty"`
	Version            string            `json:"version,omitempty"`
	CurrentVersion     string            `json:"currentVersion,omitempty"`
	Digest             string            `json:"digest,omitempty"`
	CurrentDigest      string            `json:"currentDigest,omitempty"`
	TrustStatus        string            `json:"trustStatus,omitempty"`
	SourceKind         string            `json:"sourceKind,omitempty"`
	SourceRevision     string            `json:"sourceRevision,omitempty"`
	WillEnable         bool              `json:"willEnable"`
}

func toWireApprovalPlan(plan *event.ApprovalPlan) *ApprovalPlan {
	if plan == nil {
		return nil
	}
	actions := make([]ApprovalAction, len(plan.Actions))
	for i, action := range plan.Actions {
		actions[i] = ApprovalAction{
			Kind: action.Kind, Action: action.Action, RiskLevel: action.RiskLevel,
			RiskReasons: append([]string(nil), action.RiskReasons...),
			Name:        action.Name, Source: action.Source, Target: action.Target, ConfigPath: action.ConfigPath,
			Scope: action.Scope, Mode: action.Mode, Transport: action.Transport, URL: action.URL, Command: action.Command,
			Args:               append([]string(nil), action.Args...),
			Env:                maps.Clone(action.Env),
			Headers:            maps.Clone(action.Headers),
			Permissions:        append([]string(nil), action.Permissions...),
			AddedPermissions:   append([]string(nil), action.AddedPermissions...),
			RemovedPermissions: append([]string(nil), action.RemovedPermissions...),
			Version:            action.Version, CurrentVersion: action.CurrentVersion,
			Digest: action.Digest, CurrentDigest: action.CurrentDigest,
			TrustStatus: action.TrustStatus, WillEnable: action.WillEnable,
			SourceKind: action.SourceKind, SourceRevision: action.SourceRevision,
		}
	}
	return &ApprovalPlan{
		PlanID: plan.PlanID, Operation: plan.Operation, Source: plan.Source, Name: plan.Name,
		Kind: plan.Kind, Scope: plan.Scope, Mode: plan.Mode, Actions: actions,
		Warnings: append([]string(nil), plan.Warnings...),
	}
}

// Guardian is the JSON form of an event.GuardianResult.
type Guardian struct {
	ID                string `json:"id"`
	Tool              string `json:"tool"`
	Subject           string `json:"subject"`
	Outcome           string `json:"outcome"`
	RiskLevel         string `json:"risk_level,omitempty"`
	UserAuthorization string `json:"user_authorization,omitempty"`
	Rationale         string `json:"rationale,omitempty"`
	DurationMs        int64  `json:"duration_ms,omitempty"`
	Usage             *Usage `json:"usage,omitempty"`
}

// ToWireGuardian converts an event.GuardianResult into its JSON wire form.
func ToWireGuardian(g event.GuardianResult) *Guardian {
	out := &Guardian{
		ID:                g.ID,
		Tool:              g.Tool,
		Subject:           g.Subject,
		Outcome:           g.Outcome,
		RiskLevel:         g.RiskLevel,
		UserAuthorization: g.UserAuthorization,
		Rationale:         g.Rationale,
		DurationMs:        g.DurationMs,
	}
	if u := g.Usage; u != nil {
		out.Usage = &Usage{
			PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
			TotalTokens: u.TotalTokens, CacheHitTokens: u.CacheHitTokens,
			CacheMissTokens: u.CacheMissTokens, ReasoningTokens: u.ReasoningTokens,
		}
		if g.Pricing != nil {
			cost := g.Pricing.Cost(u)
			out.Usage.Cost = cost
			out.Usage.Currency = g.Pricing.Symbol()
			out.Usage.CostUSD = cost
		}
	}
	return out
}

// ToWireAsk converts an event.Ask into its JSON wire form.
func ToWireAsk(a event.Ask) *Ask {
	qs := make([]AskQuestion, len(a.Questions))
	for i, q := range a.Questions {
		opts := make([]AskOption, len(q.Options))
		for j, o := range q.Options {
			opts[j] = AskOption{Label: o.Label, Description: o.Description}
		}
		qs[i] = AskQuestion{ID: q.ID, Header: q.Header, Prompt: q.Prompt, Options: opts, Multi: q.Multi}
	}
	return &Ask{ID: a.ID, Questions: qs}
}

// ToWireCacheDiagnostics converts cache diagnostics into their JSON wire form.
func ToWireCacheDiagnostics(d *event.CacheDiagnostics) *CacheDiagnostics {
	return &CacheDiagnostics{
		PrefixHash:          d.PrefixHash,
		PrefixChanged:       d.PrefixChanged,
		PrefixChangeReasons: append([]string(nil), d.PrefixChangeReasons...),
		SystemHash:          d.SystemHash,
		ToolsHash:           d.ToolsHash,
		LogRewriteVersion:   d.LogRewriteVersion,
		ToolSchemaTokens:    d.ToolSchemaTokens,
		CacheMissTokens:     d.CacheMissTokens,
		CacheHitTokens:      d.CacheHitTokens,
	}
}

var kindNames = map[event.Kind]string{
	event.TurnStarted:              "turn_started",
	event.Reasoning:                "reasoning",
	event.Text:                     "text",
	event.Message:                  "message",
	event.ToolDispatch:             "tool_dispatch",
	event.ToolResult:               "tool_result",
	event.Usage:                    "usage",
	event.Notice:                   "notice",
	event.Phase:                    "phase",
	event.ApprovalRequest:          "approval_request",
	event.AskRequest:               "ask_request",
	event.TurnDone:                 "turn_done",
	event.CompactionStarted:        "compaction_started",
	event.CompactionDone:           "compaction_done",
	event.ToolProgress:             "tool_progress",
	event.MCPSurfaceReady:          "mcp_surface_ready",
	event.Retrying:                 "retrying",
	event.Steer:                    "steer",
	event.MemoryCompilerStatsEvent: "memory_compiler_stats",
	event.SubagentStarted:          "subagent_started",
	event.SubagentCompleted:        "subagent_completed",
	event.CacheUpdated:             "cache_updated",
	event.ModeChanged:              "mode_changed",
	event.GuardianAssessment:       "guardian_assessment",
}
