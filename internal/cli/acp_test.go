package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/acp"
	"reames-agent/internal/config"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

const acpTestProviderKind = "acp-test-provider"

func init() {
	provider.Register(acpTestProviderKind, func(cfg provider.Config) (provider.Provider, error) {
		return &acpTestProvider{cfg: cfg}, nil
	})
}

func TestACPInitializesWithoutAPIKey(t *testing.T) {
	isolateCLIConfigHome(t)
	t.Setenv("DEEPSEEK_API_KEY", "")
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}` + "\n")
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})

	out := captureStdout(t, func() {
		if rc := Run([]string{"--acp"}, "test-version"); rc != 0 {
			t.Fatalf("Run --acp initialize rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(out, `"protocolVersion":1`) || !strings.Contains(out, `"name":"reames-agent"`) {
		t.Fatalf("initialize output = %s", out)
	}
}

func TestACPFactoryLoadsSessionCwdProjectConfig(t *testing.T) {
	home := isolateCLIConfigHome(t)
	if _, err := config.SetCredential("REAMES_AGENT_TEST_KEY", "test-key"); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "reames-agent.toml"), []byte(`
default_model = "local"

[[providers]]
name = "local"
kind = "acp-test-provider"
base_url = "http://example.invalid"
model = "fake-model"
api_key_env = "REAMES_AGENT_TEST_KEY"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmdDir := filepath.Join(project, ".reames-agent", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "acp-only.md"), []byte("ACP project command"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}

	ctrl, err := (&acpFactory{}).NewSession(context.Background(), acp.SessionParams{Cwd: project, Sink: event.Discard})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer ctrl.Close()

	for _, cmd := range ctrl.Commands() {
		if cmd.Name == "acp-only" {
			return
		}
	}
	t.Fatalf("ACP session did not load project command from cwd; commands=%v", ctrl.Commands())
}

func TestACPFactoryClearsEffortOverrideForUnsupportedModel(t *testing.T) {
	isolateCLIConfigHome(t)
	if _, err := config.SetCredential("REAMES_AGENT_TEST_KEY", "test-key"); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "reames-agent.toml"), []byte(`
default_model = "reasoner/reasoning-model"

[[providers]]
name = "reasoner"
kind = "acp-test-provider"
base_url = "http://example.invalid"
model = "reasoning-model"
api_key_env = "REAMES_AGENT_TEST_KEY"
supported_efforts = ["low", "high"]

[[providers]]
name = "plain"
kind = "acp-test-provider"
base_url = "http://example.invalid"
model = "plain-model"
api_key_env = "REAMES_AGENT_TEST_KEY"
effort = "high"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	high := "high"
	state, err := (&acpFactory{}).SessionConfigState(context.Background(), acp.SessionConfigStateParams{
		Cwd:            project,
		Model:          "reasoner/reasoning-model",
		EffortOverride: &high,
	})
	if err != nil {
		t.Fatalf("reasoning SessionConfigState: %v", err)
	}
	if state.EffortOverride == nil || *state.EffortOverride != "high" {
		t.Fatalf("reasoning effort override = %v, want high", state.EffortOverride)
	}
	workMode, ok := findACPConfigOption(state.ConfigOptions, "work_mode")
	if !ok || workMode.CurrentValue != "balanced" || len(workMode.Options) != 3 {
		t.Fatalf("work-mode option = %+v, want balanced with three choices", workMode)
	}

	state, err = (&acpFactory{}).SessionConfigState(context.Background(), acp.SessionConfigStateParams{
		Cwd:      project,
		Model:    "reasoner/reasoning-model",
		WorkMode: "delivery",
	})
	if err != nil {
		t.Fatalf("delivery SessionConfigState: %v", err)
	}
	if state.WorkMode != "delivery" {
		t.Fatalf("delivery work mode = %q, want delivery", state.WorkMode)
	}

	state, err = (&acpFactory{}).SessionConfigState(context.Background(), acp.SessionConfigStateParams{
		Cwd:            project,
		Model:          "plain/plain-model",
		EffortOverride: &high,
	})
	if err != nil {
		t.Fatalf("plain SessionConfigState: %v", err)
	}
	if _, ok := findACPConfigOption(state.ConfigOptions, "effort"); ok {
		t.Fatalf("plain model should not advertise effort option: %+v", state.ConfigOptions)
	}
	if state.EffortOverride == nil || *state.EffortOverride != "" {
		t.Fatalf("plain effort override = %v, want explicit empty override", state.EffortOverride)
	}
}

func findACPConfigOption(options []acp.SessionConfigOption, id string) (acp.SessionConfigOption, bool) {
	for _, opt := range options {
		if opt.ID == id {
			return opt, true
		}
	}
	return acp.SessionConfigOption{}, false
}

type acpTestProvider struct {
	cfg provider.Config
}

func (p *acpTestProvider) Name() string { return p.cfg.Name }

func (p *acpTestProvider) Stream(context.Context, provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk, 1)
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}
