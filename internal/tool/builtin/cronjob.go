package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"reames-agent/internal/config"
	"reames-agent/internal/cron"
	"reames-agent/internal/tool"
)

func init() { tool.RegisterBuiltin(cronJob{}) }

type cronJob struct{}

func (cronJob) Name() string        { return "cronjob" }
func (cronJob) ReadOnly() bool      { return false }

func (cronJob) Description() string {
	return "Create, list, or delete scheduled tasks that survive restarts. Supports interval (every 30m, every 2h) and once (30m, 2h) schedules. Use to set up recurring automation like daily reports or periodic checks."
}

func (cronJob) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "action":{"type":"string","description":"create, list, or delete"},
  "id":{"type":"string","description":"Job id (required for create and delete)"},
  "name":{"type":"string","description":"Human-readable name (for create)"},
  "prompt":{"type":"string","description":"The task prompt (for create)"},
  "schedule":{"type":"string","description":"Schedule: 30m, 2h, every 30m, every 2h (for create)"}
},
"required":["action"]
}`)
}

func (cronJob) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Action   string `json:"action"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		Prompt   string `json:"prompt"`
		Schedule string `json:"schedule"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("cronjob: %w", err)
	}

	dir := config.ReamesAgentHomeDir()
	if dir == "" {
		return "", fmt.Errorf("cronjob: config directory not available")
	}
	store, err := cron.Open(dir)
	if err != nil {
		return "", fmt.Errorf("cronjob: %w", err)
	}

	switch p.Action {
	case "create":
		if p.ID == "" || p.Prompt == "" {
			return "", fmt.Errorf("cronjob create: id and prompt required")
		}
		sched, err := cron.ParseSchedule(p.Schedule)
		if err != nil {
			return "", fmt.Errorf("cronjob: %w", err)
		}
		job := cron.Job{ID: p.ID, Name: p.Name, Prompt: p.Prompt, Enabled: true, Schedule: sched}
		if err := store.Add(job); err != nil {
			return "", fmt.Errorf("cronjob: %w", err)
		}
		return fmt.Sprintf("Created cron job %q with schedule %s.", p.ID, p.Schedule), nil

	case "list":
		jobs := store.List()
		if len(jobs) == 0 {
			return "No cron jobs.", nil
		}
		var b strings.Builder
		for _, j := range jobs {
			status := "enabled"
			if !j.Enabled {
				status = "disabled"
			}
			next := ""
			if !j.NextRunAt.IsZero() {
				next = fmt.Sprintf("next: %s", j.NextRunAt.Format("15:04"))
			}
			fmt.Fprintf(&b, "[%s] %s (%s) %s — %s\n", j.ID, j.Name, status, j.Schedule.Kind, next)
		}
		return strings.TrimSpace(b.String()), nil

	case "delete":
		if p.ID == "" {
			return "", fmt.Errorf("cronjob delete: id required")
		}
		if err := store.Remove(p.ID); err != nil {
			return "", fmt.Errorf("cronjob: %w", err)
		}
		return fmt.Sprintf("Deleted cron job %q.", p.ID), nil

	default:
		return "", fmt.Errorf("cronjob: unknown action %q (use create, list, delete)", p.Action)
	}
}
