package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"reames-agent/internal/acp"
	"reames-agent/internal/appserver"
	"reames-agent/internal/config"
	"reames-agent/internal/i18n"
)

// appServerCommand exposes the Codex-class local integration protocol over
// stdio JSONL. stdout is reserved exclusively for protocol frames.
func appServerCommand(args []string, version string) int {
	fs := flag.NewFlagSet("app-server", flag.ContinueOnError)
	model := fs.String("model", "", "provider/model reference (default: config default_model)")
	profile := fs.String("profile", "", "default work mode: economy, balanced, or delivery")
	listen := fs.String("listen", "stdio://", "transport endpoint; currently stdio:// only")
	stdio := fs.Bool("stdio", false, "serve over stdin/stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "app-server does not accept positional arguments")
		return 2
	}
	if *stdio {
		*listen = "stdio://"
	}
	if strings.TrimSpace(*listen) != "stdio://" && strings.TrimSpace(*listen) != "stdio" {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "app-server currently supports only --listen stdio://")
		return 2
	}
	workMode, _, err := parseCLIWorkMode(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	factory := &appServerFactory{acp: acpFactory{model: *model, workMode: workMode}}
	info := appserver.ServerInfo{Name: "reames-agent", Version: version, Home: config.ReamesAgentHomeDir()}
	if err := appserver.Serve(ctx, os.Stdin, os.Stdout, factory, info); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	return 0
}

type appServerFactory struct{ acp acpFactory }

func (f *appServerFactory) SessionDir() string { return f.acp.SessionDir() }

func (f *appServerFactory) NewThread(ctx context.Context, p appserver.ThreadParams) (appserver.ThreadRuntime, error) {
	state, err := f.acp.SessionConfigState(ctx, acp.SessionConfigStateParams{Cwd: p.Cwd, Model: p.Model, WorkMode: f.acp.workMode})
	if err != nil {
		return appserver.ThreadRuntime{}, err
	}
	ctrl, err := f.acp.NewSession(ctx, acp.SessionParams{
		Cwd: p.Cwd, Sink: p.Sink, Model: state.Model, EffortOverride: state.EffortOverride,
		WorkMode: state.WorkMode, OnSessionRecovered: p.OnSessionRecovered,
	})
	if err != nil {
		return appserver.ThreadRuntime{}, err
	}
	root := ctrl.WorkspaceRoot()
	cfg, err := config.LoadForRoot(root)
	if err != nil {
		ctrl.Close()
		return appserver.ThreadRuntime{}, err
	}
	provider := ""
	if prefix, _, ok := strings.Cut(state.Model, "/"); ok {
		provider = prefix
	}
	return appserver.ThreadRuntime{
		Controller: ctrl, Model: state.Model, ModelProvider: provider, Cwd: root,
		Sandbox: appServerSandbox(cfg, root),
	}, nil
}

func appServerSandbox(cfg *config.Config, root string) appserver.SandboxPolicy {
	if cfg == nil || cfg.BashMode() == "off" {
		return appserver.SandboxPolicy{Type: "dangerFullAccess"}
	}
	seen := make(map[string]struct{})
	roots := make([]string, 0)
	for _, candidate := range cfg.WriteRootsForRoot(root) {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if _, ok := seen[absolute]; ok {
			continue
		}
		seen[absolute] = struct{}{}
		roots = append(roots, absolute)
	}
	if len(roots) == 0 {
		return appserver.SandboxPolicy{Type: "dangerFullAccess"}
	}
	return appserver.SandboxPolicy{Type: "workspaceWrite", WritableRoots: roots, NetworkAccess: cfg.Sandbox.Network}
}
