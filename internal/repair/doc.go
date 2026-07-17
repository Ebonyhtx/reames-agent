// Package repair owns credential-free startup recovery, configuration repair,
// and verified update rollback. It must remain independent of providers, MCP,
// plugins, hooks, the Agent loop, and frontend runtimes so the Guard executable
// stays usable when normal boot cannot complete.
package repair
