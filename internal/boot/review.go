package boot

import (
	"context"

	"reames-agent/internal/agent"
	"reames-agent/internal/config"
	"reames-agent/internal/event"
	"reames-agent/internal/sandbox"
	"reames-agent/internal/skill"
	"reames-agent/internal/tool"
	"reames-agent/internal/tool/builtin"
)

// RunReviewSubagent assembles and runs the built-in review skill without
// exposing provider, agent, or tool registries to the CLI transport.
func RunReviewSubagent(ctx context.Context, reviewSkill skill.Skill, cfg *config.Config, entry *config.ProviderEntry, task string) (string, error) {
	prov, err := NewProviderWithProxy(entry, cfg.NetworkProxySpec())
	if err != nil {
		return "", err
	}
	runCtx, ledger, cleanup := agent.EnsureDelegationLedger(ctx, subagentDelegationLimits(cfg))
	defer cleanup()
	return agent.RunSubAgentWithSession(runCtx, prov, reviewSubagentRegistry(reviewSkill, cfg), agent.NewSession(reviewSkill.Body), task, agent.Options{
		MaxSteps:         12,
		Temperature:      cfg.Agent.Temperature,
		Pricing:          entry.Price,
		ContextWindow:    entry.ContextWindow,
		DelegationLedger: ledger,
	}, event.Discard)
}

func reviewSubagentRegistry(reviewSkill skill.Skill, cfg *config.Config) *tool.Registry {
	parent := tool.NewRegistry()
	for _, name := range reviewSkill.AllowedTools {
		if builtinTool, ok := tool.LookupBuiltin(name); ok {
			parent.Add(builtinTool)
		}
	}
	if _, ok := parent.Get("bash"); ok {
		guard := builtin.NewSessionDataGuard(config.MemoryUserDir(), cfg.AllowWriteRoots())
		parent.Add(builtin.ConfineBash(sandbox.Spec{}, guard))
	}
	return agent.SubagentToolRegistry(parent, reviewSkill.AllowedTools)
}
