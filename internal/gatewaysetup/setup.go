// Package gatewaysetup creates and updates headless social-gateway connection
// records without accepting secret values or weakening access control.
package gatewaysetup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"reames-agent/internal/config"
)

var (
	envNamePattern      = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	connectionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// Options describes one deterministic gateway connection setup transaction.
// Secret-bearing fields deliberately accept environment variable names only.
type Options struct {
	ConfigPath       string
	Channel          string
	ConnectionID     string
	Label            string
	WorkspaceRoot    string
	Model            string
	ToolApprovalMode string
	AppID            string
	AppSecretEnv     string
	AccountID        string
	TokenEnv         string
	Pairing          bool
	AllowAll         bool
	ResetAccess      bool
	Users            []string
	Groups           []string
	Approvers        []string
	Admins           []string
	DryRun           bool
	Now              time.Time
}

// Result is a redaction-safe summary of the requested config transaction.
type Result struct {
	ConfigPath        string
	Action            string
	Channel           string
	Connection        config.BotConnectionConfig
	Changed           bool
	Applied           bool
	OtherConnections  int
	RoutesPreserved   int
	MappingsPreserved int
}

type channelTarget struct {
	Channel             string
	Provider            string
	Domain              string
	Label               string
	DefaultID           string
	DefaultAppSecretEnv string
	DefaultTokenEnv     string
}

// Apply validates, previews, and optionally persists one connection update.
// SaveToScope uses the config package's atomic sibling-temp-and-rename writer.
func Apply(opts Options) (Result, error) {
	normalized, target, err := normalizeOptions(opts)
	if err != nil {
		return Result{}, err
	}

	unlock := config.LockUserConfigEdits()
	defer unlock()
	cfg, err := config.LoadForEditStrict(normalized.ConfigPath, false)
	if err != nil {
		return Result{}, fmt.Errorf("load gateway config: %w", err)
	}
	before := config.RenderTOMLForScope(cfg, config.RenderScopeUser)

	index := -1
	for i := range cfg.Bot.Connections {
		if strings.TrimSpace(cfg.Bot.Connections[i].ID) == normalized.ConnectionID {
			index = i
			break
		}
	}
	found := index >= 0
	var next config.BotConnectionConfig
	if found {
		next = cfg.Bot.Connections[index]
		if strings.TrimSpace(next.Provider) != target.Provider || strings.TrimSpace(next.Domain) != target.Domain {
			return Result{}, fmt.Errorf("connection id %q already belongs to %s/%s", normalized.ConnectionID, next.Provider, next.Domain)
		}
	} else {
		next = config.BotConnectionConfig{
			ID:               normalized.ConnectionID,
			Provider:         target.Provider,
			Domain:           target.Domain,
			Label:            target.Label,
			Enabled:          true,
			Status:           "pending",
			ToolApprovalMode: "ask",
		}
	}

	credentialChanged := mergeConnection(&next, normalized, target)
	if err := validateConnection(next, target); err != nil {
		return Result{}, err
	}
	if credentialChanged && found {
		next.Status = "pending"
		next.LastError = ""
	}

	if found {
		cfg.Bot.Connections[index] = next
	} else {
		cfg.Bot.Connections = append(cfg.Bot.Connections, next)
		index = len(cfg.Bot.Connections) - 1
	}
	applyLegacyBotConfig(cfg, next, target)

	afterWithoutTimestamp := config.RenderTOMLForScope(cfg, config.RenderScopeUser)
	changed := before != afterWithoutTimestamp
	action := "unchanged"
	if changed {
		if found {
			action = "update"
		} else {
			action = "create"
		}
		now := normalized.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		stamp := now.UTC().Format(time.RFC3339)
		if strings.TrimSpace(next.CreatedAt) == "" {
			next.CreatedAt = stamp
		}
		next.UpdatedAt = stamp
		cfg.Bot.Connections[index] = next
		if !normalized.DryRun {
			if err := cfg.SaveToScope(normalized.ConfigPath, config.RenderScopeUser); err != nil {
				return Result{}, fmt.Errorf("save gateway config: %w", err)
			}
		}
	}

	result := Result{
		ConfigPath:        normalized.ConfigPath,
		Action:            action,
		Channel:           target.Channel,
		Connection:        next,
		Changed:           changed,
		Applied:           changed && !normalized.DryRun,
		OtherConnections:  len(cfg.Bot.Connections) - 1,
		RoutesPreserved:   len(cfg.Bot.Routes),
		MappingsPreserved: len(next.SessionMappings),
	}
	return result, nil
}

func normalizeOptions(opts Options) (Options, channelTarget, error) {
	opts.ConfigPath = strings.TrimSpace(opts.ConfigPath)
	if opts.ConfigPath == "" {
		return opts, channelTarget{}, errors.New("gateway setup config path is required")
	}
	target, err := resolveChannel(opts.Channel)
	if err != nil {
		return opts, channelTarget{}, err
	}
	opts.Channel = target.Channel
	opts.ConnectionID = strings.TrimSpace(opts.ConnectionID)
	if opts.ConnectionID == "" {
		opts.ConnectionID = target.DefaultID
	}
	if !connectionIDPattern.MatchString(opts.ConnectionID) {
		return opts, channelTarget{}, fmt.Errorf("invalid connection id %q: use letters, digits, dot, underscore, or dash", opts.ConnectionID)
	}
	opts.Label = strings.TrimSpace(opts.Label)
	opts.Model = strings.TrimSpace(opts.Model)
	opts.ToolApprovalMode = strings.ToLower(strings.TrimSpace(opts.ToolApprovalMode))
	if opts.ToolApprovalMode != "" && opts.ToolApprovalMode != "ask" && opts.ToolApprovalMode != "auto" && opts.ToolApprovalMode != "yolo" {
		return opts, channelTarget{}, fmt.Errorf("invalid tool approval mode %q: use ask, auto, or yolo", opts.ToolApprovalMode)
	}
	opts.AppID = strings.TrimSpace(opts.AppID)
	opts.AppSecretEnv = strings.TrimSpace(opts.AppSecretEnv)
	opts.AccountID = strings.TrimSpace(opts.AccountID)
	opts.TokenEnv = strings.TrimSpace(opts.TokenEnv)
	for _, field := range []struct{ name, value string }{
		{name: "app-secret-env", value: opts.AppSecretEnv},
		{name: "token-env", value: opts.TokenEnv},
	} {
		if field.value != "" && !envNamePattern.MatchString(field.value) {
			return opts, channelTarget{}, fmt.Errorf("invalid --%s %q: pass a conventional uppercase environment variable name, never a secret value", field.name, field.value)
		}
	}
	if strings.TrimSpace(opts.WorkspaceRoot) != "" {
		workspace, err := cleanWorkspace(opts.WorkspaceRoot)
		if err != nil {
			return opts, channelTarget{}, err
		}
		opts.WorkspaceRoot = workspace
	}
	opts.Users = normalizedList(opts.Users)
	opts.Groups = normalizedList(opts.Groups)
	opts.Approvers = normalizedList(opts.Approvers)
	opts.Admins = normalizedList(opts.Admins)
	return opts, target, nil
}

func resolveChannel(channel string) (channelTarget, error) {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "feishu":
		return channelTarget{Channel: "feishu", Provider: "feishu", Domain: "feishu", Label: "飞书", DefaultID: "feishu-feishu", DefaultAppSecretEnv: "FEISHU_BOT_APP_SECRET"}, nil
	case "lark":
		return channelTarget{Channel: "lark", Provider: "feishu", Domain: "lark", Label: "Lark", DefaultID: "feishu-lark", DefaultAppSecretEnv: "LARK_BOT_APP_SECRET"}, nil
	case "qq":
		return channelTarget{Channel: "qq", Provider: "qq", Domain: "qq", Label: "QQ", DefaultID: "qq-qq", DefaultAppSecretEnv: "QQ_BOT_APP_SECRET"}, nil
	case "weixin", "wechat":
		return channelTarget{Channel: "weixin", Provider: "weixin", Domain: "weixin", Label: "微信", DefaultID: "weixin-weixin", DefaultTokenEnv: "WEIXIN_BOT_TOKEN"}, nil
	case "telegram":
		return channelTarget{Channel: "telegram", Provider: "telegram", Domain: "telegram", Label: "Telegram", DefaultID: "telegram-telegram", DefaultTokenEnv: "TELEGRAM_BOT_TOKEN"}, nil
	default:
		return channelTarget{}, fmt.Errorf("invalid gateway channel %q: use feishu, lark, qq, weixin, or telegram", channel)
	}
}

func mergeConnection(next *config.BotConnectionConfig, opts Options, target channelTarget) bool {
	credentialChanged := false
	next.ID = opts.ConnectionID
	next.Provider = target.Provider
	next.Domain = target.Domain
	next.Enabled = true
	if opts.Label != "" {
		next.Label = opts.Label
	} else if strings.TrimSpace(next.Label) == "" {
		next.Label = target.Label
	}
	if opts.WorkspaceRoot != "" {
		next.WorkspaceRoot = opts.WorkspaceRoot
	}
	if opts.Model != "" {
		next.Model = opts.Model
	}
	if opts.ToolApprovalMode != "" {
		next.ToolApprovalMode = opts.ToolApprovalMode
	} else if strings.TrimSpace(next.ToolApprovalMode) == "" {
		next.ToolApprovalMode = "ask"
	}
	if strings.TrimSpace(next.Status) == "" {
		next.Status = "pending"
	}

	if opts.AppID != "" && opts.AppID != next.Credential.AppID {
		next.Credential.AppID = opts.AppID
		credentialChanged = true
	}
	if opts.AppSecretEnv != "" && opts.AppSecretEnv != next.Credential.AppSecretEnv {
		next.Credential.AppSecretEnv = opts.AppSecretEnv
		credentialChanged = true
	}
	if opts.AccountID != "" && opts.AccountID != next.Credential.AccountID {
		next.Credential.AccountID = opts.AccountID
		credentialChanged = true
	}
	if opts.TokenEnv != "" && opts.TokenEnv != next.Credential.TokenEnv {
		next.Credential.TokenEnv = opts.TokenEnv
		credentialChanged = true
	}
	if target.DefaultAppSecretEnv != "" && strings.TrimSpace(next.Credential.AppSecretEnv) == "" {
		next.Credential.AppSecretEnv = target.DefaultAppSecretEnv
		credentialChanged = true
	}
	if target.DefaultTokenEnv != "" && strings.TrimSpace(next.Credential.TokenEnv) == "" {
		next.Credential.TokenEnv = target.DefaultTokenEnv
		credentialChanged = true
	}

	accessSpecified := opts.Pairing || opts.AllowAll || opts.ResetAccess || len(opts.Users)+len(opts.Groups)+len(opts.Approvers)+len(opts.Admins) > 0
	if opts.ResetAccess {
		next.Access = config.BotAccessConfig{}
	}
	if accessSpecified {
		next.Access.Enabled = true
		next.Access.PairingEnabled = next.Access.PairingEnabled || opts.Pairing
		next.Access.AllowAll = next.Access.AllowAll || opts.AllowAll
		next.Access.Users = appendUnique(next.Access.Users, opts.Users...)
		next.Access.Groups = appendUnique(next.Access.Groups, opts.Groups...)
		next.Access.Approvers = appendUnique(next.Access.Approvers, opts.Approvers...)
		next.Access.Admins = appendUnique(next.Access.Admins, opts.Admins...)
	}
	return credentialChanged
}

func validateConnection(conn config.BotConnectionConfig, target channelTarget) error {
	if !hasExplicitAccess(conn.Access) {
		return errors.New("gateway setup refuses an open-ended connection: choose --pairing, --users/--groups/--approvers/--admins, or --allow-all intentionally")
	}
	switch target.Provider {
	case "feishu", "qq":
		if strings.TrimSpace(conn.Credential.AppID) == "" {
			return fmt.Errorf("%s setup requires --app-id", target.Channel)
		}
		if !envNamePattern.MatchString(strings.TrimSpace(conn.Credential.AppSecretEnv)) {
			return fmt.Errorf("%s setup requires a valid --app-secret-env name", target.Channel)
		}
	case "weixin":
		if strings.TrimSpace(conn.Credential.AccountID) == "" {
			return errors.New("weixin setup requires --account-id")
		}
		if !envNamePattern.MatchString(strings.TrimSpace(conn.Credential.TokenEnv)) {
			return errors.New("weixin setup requires a valid --token-env name")
		}
	case "telegram":
		if !envNamePattern.MatchString(strings.TrimSpace(conn.Credential.TokenEnv)) {
			return errors.New("telegram setup requires a valid --token-env name")
		}
	}
	return nil
}

func applyLegacyBotConfig(cfg *config.Config, conn config.BotConnectionConfig, target channelTarget) {
	cfg.Bot.Enabled = true
	switch target.Provider {
	case "qq":
		cfg.Bot.QQ.Enabled = true
		cfg.Bot.QQ.AppID = conn.Credential.AppID
		cfg.Bot.QQ.AppSecretEnv = conn.Credential.AppSecretEnv
	case "feishu":
		cfg.Bot.Feishu.Enabled = true
		cfg.Bot.Feishu.Domain = target.Domain
		cfg.Bot.Feishu.AppID = conn.Credential.AppID
		cfg.Bot.Feishu.AppSecretEnv = conn.Credential.AppSecretEnv
		cfg.Bot.Feishu.Mode = "websocket"
		cfg.Bot.Feishu.RequireMention = true
	case "weixin":
		cfg.Bot.Weixin.Enabled = true
		cfg.Bot.Weixin.AccountID = conn.Credential.AccountID
		cfg.Bot.Weixin.TokenEnv = conn.Credential.TokenEnv
	case "telegram":
		cfg.Bot.Telegram.Enabled = true
		cfg.Bot.Telegram.TokenEnv = conn.Credential.TokenEnv
	}
}

func hasExplicitAccess(access config.BotAccessConfig) bool {
	return access.AllowAll || access.PairingEnabled || len(access.Users)+len(access.Groups)+len(access.Approvers)+len(access.Admins) > 0
}

func cleanWorkspace(raw string) (string, error) {
	path := os.ExpandEnv(strings.TrimSpace(raw))
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve workspace home: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q: %w", raw, err)
	}
	return filepath.Clean(abs), nil
}

func normalizedList(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = appendUnique(out, part)
			}
		}
	}
	return out
}

func appendUnique(values []string, additions ...string) []string {
	for _, addition := range additions {
		addition = strings.TrimSpace(addition)
		if addition == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if strings.TrimSpace(existing) == addition {
				found = true
				break
			}
		}
		if !found {
			values = append(values, addition)
		}
	}
	return values
}

// FormatResult returns stable, redaction-safe output for logs and dry-runs.
func FormatResult(result Result, dryRun bool) string {
	conn := result.Connection
	workspace := strings.TrimSpace(conn.WorkspaceRoot)
	if workspace == "" {
		workspace = "<default>"
	}
	model := strings.TrimSpace(conn.Model)
	if model == "" {
		model = "<default>"
	}
	writeState := "unchanged"
	if result.Changed && dryRun {
		writeState = "skipped (dry-run)"
	} else if result.Applied {
		writeState = "applied atomically"
	}
	return fmt.Sprintf(
		"gateway setup plan:\n  config: %s\n  action: %s\n  channel: %s\n  connection_id: %s\n  provider: %s\n  domain: %s\n  workspace: %s\n  model: %s\n  tool_approval: %s\n  credentials: app_id=%s app_secret_env=%s account_id=%s token_env=%s\n  access: allow_all=%t pairing=%t users=%d groups=%d approvers=%d admins=%d\n  preserved: other_connections=%d routes=%d session_mappings=%d\n  write: %s\n",
		result.ConfigPath,
		result.Action,
		result.Channel,
		conn.ID,
		conn.Provider,
		conn.Domain,
		workspace,
		model,
		conn.ToolApprovalMode,
		setState(conn.Credential.AppID),
		emptyState(conn.Credential.AppSecretEnv),
		setState(conn.Credential.AccountID),
		emptyState(conn.Credential.TokenEnv),
		conn.Access.AllowAll,
		conn.Access.PairingEnabled,
		len(conn.Access.Users),
		len(conn.Access.Groups),
		len(conn.Access.Approvers),
		len(conn.Access.Admins),
		result.OtherConnections,
		result.RoutesPreserved,
		result.MappingsPreserved,
		writeState,
	)
}

func setState(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unset"
	}
	return "set"
}

func emptyState(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unset>"
	}
	return strings.TrimSpace(value)
}
