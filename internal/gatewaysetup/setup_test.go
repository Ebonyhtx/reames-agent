package gatewaysetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/config"
)

func TestApplyCreatesSupportedChannelConnections(t *testing.T) {
	tests := []struct {
		name      string
		options   Options
		wantID    string
		provider  string
		domain    string
		secretEnv string
		tokenEnv  string
	}{
		{
			name: "feishu", options: Options{Channel: "feishu", AppID: "feishu-app", Pairing: true},
			wantID: "feishu-feishu", provider: "feishu", domain: "feishu", secretEnv: "FEISHU_BOT_APP_SECRET",
		},
		{
			name: "lark", options: Options{Channel: "lark", AppID: "lark-app", Users: []string{"ou-user"}},
			wantID: "feishu-lark", provider: "feishu", domain: "lark", secretEnv: "LARK_BOT_APP_SECRET",
		},
		{
			name: "qq", options: Options{Channel: "qq", AppID: "qq-app", Groups: []string{"group-1"}},
			wantID: "qq-qq", provider: "qq", domain: "qq", secretEnv: "QQ_BOT_APP_SECRET",
		},
		{
			name: "weixin", options: Options{Channel: "weixin", AccountID: "wx-account", Admins: []string{"wx-owner"}},
			wantID: "weixin-weixin", provider: "weixin", domain: "weixin", tokenEnv: "WEIXIN_BOT_TOKEN",
		},
		{
			name: "telegram", options: Options{Channel: "telegram", Pairing: true},
			wantID: "telegram-telegram", provider: "telegram", domain: "telegram", tokenEnv: "TELEGRAM_BOT_TOKEN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := isolateGatewaySetupConfig(t)
			tt.options.ConfigPath = path
			tt.options.Now = time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
			result, err := Apply(tt.options)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !result.Applied || result.Action != "create" {
				t.Fatalf("result = %+v, want applied create", result)
			}
			conn := result.Connection
			if conn.ID != tt.wantID || conn.Provider != tt.provider || conn.Domain != tt.domain || !conn.Enabled || conn.Status != "pending" {
				t.Fatalf("connection = %+v", conn)
			}
			if conn.Credential.AppSecretEnv != tt.secretEnv || conn.Credential.TokenEnv != tt.tokenEnv {
				t.Fatalf("credential envs = %+v", conn.Credential)
			}
			if conn.CreatedAt != "2026-07-13T08:00:00Z" || conn.UpdatedAt != conn.CreatedAt {
				t.Fatalf("timestamps = created %q updated %q", conn.CreatedAt, conn.UpdatedAt)
			}

			cfg, err := config.LoadForEditStrict(path, false)
			if err != nil {
				t.Fatalf("LoadForEditStrict: %v", err)
			}
			if !cfg.Bot.Enabled || len(cfg.Bot.Connections) != 1 {
				t.Fatalf("bot config = %+v", cfg.Bot)
			}
			switch tt.provider {
			case "feishu":
				if !cfg.Bot.Feishu.Enabled || cfg.Bot.Feishu.Mode != "websocket" || cfg.Bot.Feishu.Domain != tt.domain {
					t.Fatalf("legacy feishu config = %+v", cfg.Bot.Feishu)
				}
			case "qq":
				if !cfg.Bot.QQ.Enabled || cfg.Bot.QQ.AppID != "qq-app" {
					t.Fatalf("legacy QQ config = %+v", cfg.Bot.QQ)
				}
			case "weixin":
				if !cfg.Bot.Weixin.Enabled || cfg.Bot.Weixin.AccountID != "wx-account" {
					t.Fatalf("legacy Weixin config = %+v", cfg.Bot.Weixin)
				}
			case "telegram":
				if !cfg.Bot.Telegram.Enabled || cfg.Bot.Telegram.TokenEnv != "TELEGRAM_BOT_TOKEN" {
					t.Fatalf("legacy Telegram config = %+v", cfg.Bot.Telegram)
				}
			}
		})
	}
}

func TestApplyUpdatePreservesOtherStateAndIsIdempotent(t *testing.T) {
	path := isolateGatewaySetupConfig(t)
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg := config.Default()
	cfg.Bot.Routes = []config.BotRouteConfig{{ConnectionID: "feishu-lark", ChatID: "oc-route", Model: "route-model"}}
	cfg.Bot.Connections = []config.BotConnectionConfig{
		{
			ID: "feishu-lark", Provider: "feishu", Domain: "lark", Label: "Existing Lark", Enabled: true, Status: "connected",
			ToolApprovalMode: "ask",
			Access:           config.BotAccessConfig{Enabled: true, PairingEnabled: true, Users: []string{"ou-existing"}},
			Credential:       config.BotConnectionCredential{AppID: "existing-app", AppSecretEnv: "CUSTOM_LARK_SECRET"},
			SessionMappings:  []config.BotConnectionSessionMapping{{RemoteID: "oc-chat", SessionID: "path:/sessions/lark.jsonl", Scope: "project", WorkspaceRoot: workspace}},
			CreatedAt:        "2026-07-01T00:00:00Z", UpdatedAt: "2026-07-02T00:00:00Z",
		},
		{ID: "weixin-weixin", Provider: "weixin", Domain: "weixin", Enabled: true, Credential: config.BotConnectionCredential{AccountID: "wx"}},
	}
	if err := cfg.SaveToScope(path, config.RenderScopeUser); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	opts := Options{
		ConfigPath: path, Channel: "lark", WorkspaceRoot: workspace, Model: "deepseek-pro",
		Users: []string{"ou-existing,ou-new"}, Approvers: []string{"ou-approver"},
		Now: time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC),
	}
	first, err := Apply(opts)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if !first.Applied || first.Action != "update" || first.OtherConnections != 1 || first.RoutesPreserved != 1 || first.MappingsPreserved != 1 {
		t.Fatalf("first result = %+v", first)
	}
	conn := first.Connection
	if conn.CreatedAt != "2026-07-01T00:00:00Z" || conn.UpdatedAt != "2026-07-13T09:00:00Z" || conn.Status != "connected" {
		t.Fatalf("preserved metadata = %+v", conn)
	}
	if conn.Credential.AppID != "existing-app" || conn.Credential.AppSecretEnv != "CUSTOM_LARK_SECRET" {
		t.Fatalf("credentials not preserved: %+v", conn.Credential)
	}
	if len(conn.SessionMappings) != 1 || len(conn.Access.Users) != 2 || len(conn.Access.Approvers) != 1 || !conn.Access.PairingEnabled {
		t.Fatalf("connection state not merged: %+v", conn)
	}

	beforeRepeat, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	opts.Now = time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	second, err := Apply(opts)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if second.Changed || second.Applied || second.Action != "unchanged" || second.Connection.UpdatedAt != "2026-07-13T09:00:00Z" {
		t.Fatalf("second result = %+v, want unchanged", second)
	}
	afterRepeat, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeRepeat) != string(afterRepeat) {
		t.Fatal("idempotent setup rewrote config bytes")
	}
}

func TestApplyDryRunDoesNotCreateOrRewriteConfig(t *testing.T) {
	path := isolateGatewaySetupConfig(t)
	result, err := Apply(Options{ConfigPath: path, Channel: "feishu", AppID: "private-app-id", Pairing: true, DryRun: true})
	if err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	if !result.Changed || result.Applied || result.Action != "create" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry-run created config: %v", err)
	}
	out := FormatResult(result, true)
	if strings.Contains(out, "private-app-id") || !strings.Contains(out, "app_id=set") || !strings.Contains(out, "write: skipped (dry-run)") {
		t.Fatalf("dry-run output is not redaction-safe:\n%s", out)
	}

	preset, ok := config.CuratedProviderPreset("stepfun")
	if !ok || len(preset.Entries) != 1 {
		t.Fatal("missing stepfun preset fixture")
	}
	cfg := config.Default()
	entry := preset.Entries[0]
	entry.BaseURL = "https://api.stepfun.ai/step_plan/v1"
	cfg.Providers = append(cfg.Providers, entry)
	if err := cfg.SaveToScope(path, config.RenderScopeUser); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err = Apply(Options{ConfigPath: path, Channel: "feishu", AppID: "private-app-id", Pairing: true, DryRun: true})
	if err != nil {
		t.Fatalf("Apply dry-run to existing config: %v", err)
	}
	if !result.Changed || result.Applied || result.Action != "create" {
		t.Fatalf("existing config result = %+v", result)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("dry-run rewrote existing config during normalization")
	}
}

func TestApplyRejectsUnsafeInputsWithoutTouchingConfig(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{name: "missing access", opts: Options{Channel: "feishu", AppID: "app"}, want: "choose --pairing"},
		{name: "secret instead of env", opts: Options{Channel: "qq", AppID: "app", AppSecretEnv: "sk_live_actualsecret", Pairing: true}, want: "environment variable name"},
		{name: "missing weixin account", opts: Options{Channel: "weixin", Pairing: true}, want: "--account-id"},
		{name: "invalid channel", opts: Options{Channel: "discord", Pairing: true}, want: "invalid gateway channel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := isolateGatewaySetupConfig(t)
			tt.opts.ConfigPath = path
			_, err := Apply(tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Apply error = %v, want %q", err, tt.want)
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("rejected setup touched config: %v", statErr)
			}
		})
	}
}

func TestApplyResetAccessCanNarrowExistingAllowAll(t *testing.T) {
	path := isolateGatewaySetupConfig(t)
	cfg := config.Default()
	cfg.Bot.Enabled = true
	cfg.Bot.Connections = []config.BotConnectionConfig{{
		ID: "qq-qq", Provider: "qq", Domain: "qq", Enabled: true, Status: "connected",
		Access:     config.BotAccessConfig{Enabled: true, AllowAll: true, PairingEnabled: true, Users: []string{"old-user"}},
		Credential: config.BotConnectionCredential{AppID: "qq-app", AppSecretEnv: "QQ_BOT_APP_SECRET"},
		CreatedAt:  "2026-07-01T00:00:00Z", UpdatedAt: "2026-07-02T00:00:00Z",
	}}
	if err := cfg.SaveToScope(path, config.RenderScopeUser); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(Options{
		ConfigPath: path, Channel: "qq", ResetAccess: true, Users: []string{"owner"},
		Now: time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	access := result.Connection.Access
	if access.AllowAll || access.PairingEnabled || len(access.Users) != 1 || access.Users[0] != "owner" || !access.Enabled {
		t.Fatalf("reset access = %+v, want only owner", access)
	}
}

func TestApplyMalformedConfigFailsClosed(t *testing.T) {
	path := isolateGatewaySetupConfig(t)
	raw := []byte("[bot\nenabled = true\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(Options{ConfigPath: path, Channel: "feishu", AppID: "app", Pairing: true})
	if err == nil || !strings.Contains(err.Error(), "load gateway config") {
		t.Fatalf("Apply error = %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(raw) {
		t.Fatalf("malformed config was overwritten: %q", got)
	}
}

func isolateGatewaySetupConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	return config.UserConfigPath()
}
