package control

import (
	"context"
	"errors"
	"strings"

	"reames-agent/internal/agent"
)

var ErrSubagentDeliveryUnavailable = errors.New("subagent delivery is unavailable")

// SubagentDeliveryView is the public control-plane DTO. The alias keeps the
// persistence owner in internal/agent while transport packages depend only on
// control.
type SubagentDeliveryView = agent.SubagentDeliveryView

// SubagentDeliveries returns the live Git-derived delivery projection owned by
// the active parent session.
func (c *Controller) SubagentDeliveries() ([]SubagentDeliveryView, error) {
	if c == nil || c.subagents == nil {
		return nil, ErrSubagentDeliveryUnavailable
	}
	path := strings.TrimSpace(c.SessionPath())
	if path == "" {
		return nil, nil
	}
	return c.subagents.ListDeliveries(agent.BranchID(path), c.workspaceRoot)
}

func (c *Controller) SubagentDelivery(ref string) (SubagentDeliveryView, error) {
	if c == nil || c.subagents == nil {
		return agent.SubagentDeliveryView{}, ErrSubagentDeliveryUnavailable
	}
	return c.subagents.Delivery(ref, c.workspaceRoot)
}

// MutateSubagentDelivery applies, merges, rolls back, or rejects one delivery
// under the same runtime reservation and cross-process workspace lease used by
// every frontend.
func (c *Controller) MutateSubagentDelivery(ctx context.Context, ref, op string) (SubagentDeliveryView, error) {
	if c == nil || c.subagents == nil || c.workspaceLease == nil {
		return agent.SubagentDeliveryView{}, ErrSubagentDeliveryUnavailable
	}
	releaseRuntime, err := c.BeginRuntimeMutation()
	if err != nil {
		return agent.SubagentDeliveryView{}, err
	}
	defer releaseRuntime()
	c.workspaceLease.BeginRun()
	defer c.workspaceLease.EndRun()
	if err := c.workspaceLease.AcquireWrite(ctx); err != nil {
		return agent.SubagentDeliveryView{}, err
	}
	return c.subagents.MutateDelivery(ctx, ref, c.workspaceRoot, op)
}
