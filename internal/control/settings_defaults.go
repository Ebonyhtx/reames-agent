package control

import (
	"reames-agent/internal/agent"
	"reames-agent/internal/provider"
)

// DefaultMaxSubagentDepth is the runtime default exposed to settings surfaces.
const DefaultMaxSubagentDepth = agent.DefaultMaxSubagentDepth

// RegisteredProviderKinds is the stable settings view of available provider
// factories. Callers receive a copy so UI sorting cannot mutate the registry.
func RegisteredProviderKinds() []string {
	return append([]string(nil), provider.Kinds()...)
}
