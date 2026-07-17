package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"reames-agent/internal/mcptrust"
	"reames-agent/internal/processpolicy"
)

func specIdentityFingerprint(ctx context.Context, spec Spec) (string, error) {
	identity, err := buildSpecIdentity(ctx, spec)
	if err != nil {
		return "", err
	}
	return mcptrust.IdentityFingerprint(identity)
}

func buildSpecIdentity(ctx context.Context, spec Spec) (mcptrust.Identity, error) {
	transport := strings.ToLower(strings.TrimSpace(spec.Type))
	if transport == "" {
		transport = "stdio"
	}
	identity := mcptrust.Identity{
		Server: spec.Name, Transport: transport, Dir: spec.Dir,
		Args:    identityLaunchArgs(spec),
		EnvKeys: sortedMapKeys(spec.Env), HeaderKeys: sortedMapKeys(spec.Headers),
		PackageOwner: spec.PackagePolicy.Owner, PackageRoot: spec.PackagePolicy.PackageRoot,
		LauncherDigest: spec.LauncherDigest, ConfigSource: trustConfigSource(spec),
	}
	var err error
	identity.ArgFiles, err = identityArgumentFiles(spec, identity.Args)
	if err != nil {
		return mcptrust.Identity{}, err
	}
	switch transport {
	case "stdio":
		if strings.TrimSpace(spec.Command) == "" {
			return mcptrust.Identity{}, fmt.Errorf("stdio plugin %q: command is required", spec.Name)
		}
		env := mergeEnv(processpolicy.ProcessEnvironment(), spec.Env)
		if spec.PackagePolicy.Enabled() {
			env = spec.PackagePolicy.ChildEnvironment(spec.Env)
		}
		executable, _, err := resolveStdioExecutableWithShellPath(ctx, spec, env, !spec.PackagePolicy.Enabled())
		if err != nil {
			return mcptrust.Identity{}, err
		}
		if abs, err := filepath.Abs(executable); err == nil {
			executable = abs
		}
		identity.CommandPath = executable
		identity.CommandSHA256, err = mcptrust.FileSHA256(executable)
		if err != nil {
			return mcptrust.Identity{}, fmt.Errorf("hash MCP executable %q: %w", executable, err)
		}
	case "http", "streamable-http", "streamable_http":
		identity.Transport = "http"
		identity.URL = normalizeIdentityURL(spec.URL)
	default:
		identity.URL = normalizeIdentityURL(spec.URL)
	}
	return identity, nil
}

func identityArgumentFiles(spec Spec, args []string) ([]mcptrust.ArgFileIdentity, error) {
	base := strings.TrimSpace(spec.Dir)
	if base == "" && spec.PackagePolicy.Enabled() {
		base = spec.PackagePolicy.PackageRoot
	}
	if base == "" {
		base = "."
	}
	var out []mcptrust.ArgFileIdentity
	for index, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		candidate := arg
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(base, candidate)
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		digest, err := mcptrust.FileSHA256(candidate)
		if err != nil {
			return nil, fmt.Errorf("hash MCP argument file %q: %w", candidate, err)
		}
		out = append(out, mcptrust.ArgFileIdentity{Index: index, Path: candidate, SHA256: digest})
	}
	return out, nil
}

const identityURLRedacted = "__redacted__"

var credentialURLQueryKeys = map[string]bool{
	"auth": true, "authorization": true, "bearer": true, "credential": true, "credentials": true, "sig": true,
	"key": true, "accesskey": true, "secretkey": true, "privatekey": true, "authkey": true,
	"appkey": true, "clientkey": true, "subscriptionkey": true, "sharedkey": true,
}

var credentialURLQuerySuffixes = []string{"token", "secret", "password", "passwd", "apikey", "signature"}

func credentialURLQueryKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	if credentialURLQueryKeys[normalized] {
		return true
	}
	for _, suffix := range credentialURLQuerySuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

// normalizeIdentityURL canonicalizes endpoint structure while replacing
// credentials in userinfo and credential-like query values with a fixed token.
func normalizeIdentityURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSpace(raw)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host = net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
	parsed.Host = host
	parsed.Fragment = ""
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(identityURLRedacted, identityURLRedacted)
		} else {
			parsed.User = url.User(identityURLRedacted)
		}
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key, values := range query {
			if credentialURLQueryKey(key) {
				for i := range values {
					values[i] = identityURLRedacted
				}
			} else {
				sort.Strings(values)
			}
			query[key] = values
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func validatePersistentTransportTrust(spec Spec) error {
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "http", "streamable-http", "streamable_http", "sse":
		parsed, err := url.Parse(strings.TrimSpace(spec.URL))
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
			return fmt.Errorf("persistent trust for remote MCP server %q requires an HTTPS URL; use session trust for this connection", spec.Name)
		}
	}
	return nil
}

func sortedMapKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		if key = strings.TrimSpace(key); key != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func capabilityOf(spec Spec, raw mcpTool, schema json.RawMessage) mcptrust.Capability {
	visible := raw.Name
	if spec.StripRawPrefix != "" {
		visible = strings.TrimPrefix(visible, spec.StripRawPrefix)
	}
	hinted := raw.Annotations != nil && raw.Annotations.ReadOnlyHint
	destructive := raw.Annotations != nil && raw.Annotations.DestructiveHint
	return mcptrust.Capability{
		RawName: raw.Name, ModelName: toolName(spec.Name, visible),
		InputSchema: schema, OutputSchema: raw.OutputSchema,
		ReadOnly: hinted || spec.toolReadOnlyOverride(raw.Name, visible), Destructive: destructive,
	}
}

func cachedCapabilityOf(spec Spec, cached CachedTool) mcptrust.Capability {
	visible := cached.Name
	if spec.StripRawPrefix != "" {
		visible = strings.TrimPrefix(visible, spec.StripRawPrefix)
	}
	return mcptrust.Capability{
		RawName: cached.Name, ModelName: toolName(spec.Name, visible),
		InputSchema: cached.Schema, OutputSchema: cached.OutputSchema,
		ReadOnly: cached.ReadOnly || spec.toolReadOnlyOverride(cached.Name, visible), Destructive: cached.Destructive,
	}
}

func evaluateCachedTrust(ctx context.Context, spec Spec, cached *CachedSchema) (Spec, mcptrust.Evaluation) {
	eval := mcptrust.Evaluation{State: mcptrust.TrustUntrusted, TrustedReaders: map[string]bool{}}
	if cached == nil {
		return spec, eval
	}
	if spec.TrustManager == nil {
		for _, tool := range cached.Tools {
			capability := cachedCapabilityOf(spec, tool)
			if capability.ReadOnly && !capability.Destructive {
				eval.TrustedReaders[tool.Name] = true
			}
		}
		return spec, eval
	}
	locked, err := applyStoredLauncherLock(spec)
	if err != nil {
		return spec, eval
	}
	identity, err := specIdentityFingerprint(ctx, locked)
	if err != nil {
		return locked, eval
	}
	capabilities := make([]mcptrust.Capability, 0, len(cached.Tools))
	for _, tool := range cached.Tools {
		capabilities = append(capabilities, cachedCapabilityOf(locked, tool))
	}
	eval, err = locked.TrustManager.Evaluate(locked.Name, trustConfigSource(locked), identity, capabilities)
	if err != nil || eval.TrustedReaders == nil {
		eval = mcptrust.Evaluation{State: mcptrust.TrustUntrusted, TrustedReaders: map[string]bool{}}
	}
	return locked, eval
}

func trustConfigSource(spec Spec) string {
	if value := strings.TrimSpace(spec.ConfigSource); value != "" {
		return value
	}
	if owner := strings.TrimSpace(spec.PackagePolicy.Owner); owner != "" {
		return "plugin_package:" + owner
	}
	return "configured"
}

// HasStoredReaderSelection reports whether plan-mode/on-demand resolution may
// attempt this server. It does not grant execution authority; the live or cached
// capability evaluation still has to match the receipt before any tool is a
// reader.
func HasStoredReaderSelection(spec Spec) bool {
	manager := spec.TrustManager
	configSource := trustConfigSource(spec)
	if manager != nil {
		if hasReceipt, err := manager.HasReceipt(spec.Name, configSource); err == nil && hasReceipt {
			selected, err := manager.SelectedReaders(spec.Name, configSource)
			return err == nil && len(selected) > 0
		}
		if imported, err := manager.LegacyImported(spec.Name, configSource); err == nil && imported {
			return false
		}
	}
	return len(spec.ReadOnlyToolNames) > 0 || len(spec.ReadOnlyModelToolNames) > 0
}

func evaluateSpecTrust(spec Spec, identity string, capabilities []mcptrust.Capability) (mcptrust.Evaluation, error) {
	manager := spec.TrustManager
	if manager == nil {
		trusted := map[string]bool{}
		for _, capability := range capabilities {
			if capability.ReadOnly && !capability.Destructive {
				trusted[capability.RawName] = true
			}
		}
		return mcptrust.Evaluation{State: mcptrust.TrustUntrusted, TrustedReaders: trusted}, nil
	}
	configSource := trustConfigSource(spec)
	hasReceipt, err := manager.HasReceipt(spec.Name, configSource)
	if err != nil {
		return mcptrust.Evaluation{}, err
	}
	if !hasReceipt {
		legacyImported, err := manager.LegacyImported(spec.Name, configSource)
		if err != nil {
			return mcptrust.Evaluation{}, err
		}
		if !legacyImported {
			selected := make([]string, 0)
			for _, capability := range capabilities {
				visible := strings.TrimPrefix(capability.ModelName, ToolPrefix(spec.Name))
				if spec.toolReadOnlyOverride(capability.RawName, visible) && capability.ReadOnly && !capability.Destructive {
					selected = append(selected, capability.RawName)
				}
			}
			if err := manager.ImportLegacyReaders(spec.Name, configSource, identity, capabilities, selected); err != nil {
				return mcptrust.Evaluation{}, err
			}
		}
	}
	return manager.Evaluate(spec.Name, configSource, identity, capabilities)
}
