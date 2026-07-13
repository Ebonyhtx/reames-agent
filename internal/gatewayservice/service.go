// Package gatewayservice renders and executes OS service lifecycle operations
// for the Reames social-channel gateway.
package gatewayservice

import (
	"context"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"reames-agent/internal/fileutil"
)

const (
	defaultServiceName = "reames-agent-gateway"
	defaultScope       = "user"
)

// Options describes one gateway service lifecycle operation.
type Options struct {
	Action     string
	Name       string
	Scope      string
	Executable string
	Home       string
	Channels   string
	Dir        string
	Model      string
	StartNow   bool
	DryRun     bool
}

// File is a service definition file that an install operation writes.
type File struct {
	Path    string
	Content string
	Mode    os.FileMode
}

// Command is an external service-manager command.
type Command struct {
	Name string
	Args []string
}

// Plan is the full set of writes and commands for a lifecycle operation.
type Plan struct {
	GOOS         string
	Action       string
	Files        []File
	Deletes      []string
	Commands     []Command
	PostCommands []Command
	Notes        []string
}

// Result reports what happened while applying a plan.
type Result struct {
	Plan    Plan
	Outputs []string
}

// NormalizeOptions fills defaults and validates a lifecycle request.
func NormalizeOptions(opts Options) (Options, error) {
	opts.Action = strings.TrimSpace(opts.Action)
	if opts.Action == "" {
		return opts, errors.New("gateway service action is required")
	}
	if opts.Name = strings.TrimSpace(opts.Name); opts.Name == "" {
		opts.Name = defaultServiceName
	}
	if strings.ContainsAny(opts.Name, `/\:*?"<>|`) {
		return opts, fmt.Errorf("invalid service name %q", opts.Name)
	}
	if opts.Scope = strings.TrimSpace(opts.Scope); opts.Scope == "" {
		opts.Scope = defaultScope
	}
	if opts.Scope != "user" && opts.Scope != "system" {
		return opts, fmt.Errorf("invalid service scope %q: use user or system", opts.Scope)
	}
	if opts.Executable = strings.TrimSpace(opts.Executable); opts.Executable == "" {
		exe, err := os.Executable()
		if err != nil {
			return opts, fmt.Errorf("resolve executable: %w", err)
		}
		opts.Executable = exe
	}
	if opts.Home = strings.TrimSpace(opts.Home); opts.Home != "" {
		opts.Home = cleanPath(opts.Home)
	}
	return opts, nil
}

// BuildPlan renders the OS-specific service operation without touching the host.
func BuildPlan(goos string, opts Options) (Plan, error) {
	opts, err := NormalizeOptions(opts)
	if err != nil {
		return Plan{}, err
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if err := validateInstallPaths(goos, opts); err != nil {
		return Plan{}, err
	}
	var plan Plan
	switch goos {
	case "linux":
		plan, err = linuxPlan(opts)
	case "darwin":
		plan, err = launchdPlan(opts)
	case "windows":
		plan, err = windowsPlan(opts)
	default:
		return Plan{}, fmt.Errorf("gateway service lifecycle is not supported on %s yet; use gateway run under your process manager", goos)
	}
	if err != nil {
		return Plan{}, err
	}
	appendCredentialNotes(&plan, opts)
	return plan, nil
}

func validateInstallPaths(goos string, opts Options) error {
	if opts.Action != "install" {
		return nil
	}
	for name, value := range map[string]string{
		"executable": opts.Executable,
		"home":       opts.Home,
		"dir":        opts.Dir,
	} {
		if value != "" && !targetPathIsAbs(goos, value) {
			return fmt.Errorf("gateway service %s must be an absolute %s path: %q", name, goos, value)
		}
	}
	return nil
}

func targetPathIsAbs(goos, value string) bool {
	if goos != "windows" {
		return pathpkg.IsAbs(value)
	}
	if strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, `//`) {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

// Apply builds and applies a lifecycle operation. With DryRun, it only returns
// the rendered plan. Actual install defaults to user-level service management.
func Apply(ctx context.Context, opts Options) (Result, error) {
	plan, err := BuildPlan(runtime.GOOS, opts)
	if err != nil {
		return Result{}, err
	}
	if opts.DryRun {
		return Result{Plan: plan}, nil
	}
	if opts.Scope == "system" {
		return Result{Plan: plan}, errors.New("system-scope gateway service changes require manual approval; re-run with --dry-run and install the rendered plan as administrator/root")
	}
	for _, f := range plan.Files {
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return Result{Plan: plan}, err
		}
		if err := fileutil.AtomicWriteFile(f.Path, []byte(f.Content), f.Mode); err != nil {
			return Result{Plan: plan}, err
		}
	}
	var outputs []string
	for _, c := range plan.Commands {
		var err error
		outputs, err = runCommand(ctx, outputs, c)
		if err != nil {
			return Result{Plan: plan, Outputs: outputs}, err
		}
	}
	for _, path := range plan.Deletes {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Result{Plan: plan, Outputs: outputs}, err
		}
	}
	for _, c := range plan.PostCommands {
		var err error
		outputs, err = runCommand(ctx, outputs, c)
		if err != nil {
			return Result{Plan: plan, Outputs: outputs}, err
		}
	}
	return Result{Plan: plan, Outputs: outputs}, nil
}

func runCommand(ctx context.Context, outputs []string, c Command) ([]string, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		outputs = append(outputs, strings.TrimSpace(string(out)))
	}
	if err != nil {
		return outputs, fmt.Errorf("%s %s: %w", c.Name, strings.Join(c.Args, " "), err)
	}
	return outputs, nil
}

// FormatPlan returns a stable human-readable representation for dry-runs and logs.
func FormatPlan(plan Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "gateway service plan: os=%s action=%s\n", plan.GOOS, plan.Action)
	for _, note := range plan.Notes {
		fmt.Fprintf(&b, "note: %s\n", note)
	}
	for _, f := range plan.Files {
		fmt.Fprintf(&b, "write %s mode=%#o\n", f.Path, f.Mode)
		b.WriteString(f.Content)
		if !strings.HasSuffix(f.Content, "\n") {
			b.WriteByte('\n')
		}
	}
	for _, c := range plan.Commands {
		fmt.Fprintf(&b, "run: %s\n", shellLine(c))
	}
	for _, path := range plan.Deletes {
		fmt.Fprintf(&b, "delete %s\n", path)
	}
	for _, c := range plan.PostCommands {
		fmt.Fprintf(&b, "run after delete: %s\n", shellLine(c))
	}
	return b.String()
}

func linuxPlan(opts Options) (Plan, error) {
	unitPath := linuxUnitPath(opts)
	serviceName := opts.Name + ".service"
	plan := Plan{GOOS: "linux", Action: opts.Action}
	switch opts.Action {
	case "install":
		plan.Files = append(plan.Files, File{Path: unitPath, Mode: 0o644, Content: systemdUnit(opts)})
		plan.Commands = append(plan.Commands, Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "daemon-reload")})
		if opts.StartNow {
			plan.Commands = append(plan.Commands,
				Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "enable", serviceName)},
				Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "restart", serviceName)},
				Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "is-active", "--quiet", serviceName)},
			)
		}
	case "uninstall":
		plan.Commands = append(plan.Commands, Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "disable", "--now", serviceName)})
		plan.Deletes = append(plan.Deletes, unitPath)
		plan.PostCommands = append(plan.PostCommands, Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "daemon-reload")})
	case "start", "stop", "restart", "status":
		plan.Commands = append(plan.Commands, Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, opts.Action, serviceName)})
	default:
		return Plan{}, fmt.Errorf("unknown gateway service action %q", opts.Action)
	}
	if opts.Scope == "user" {
		plan.Notes = append(plan.Notes, "user services may require: loginctl enable-linger $USER")
	}
	return plan, nil
}

func launchdPlan(opts Options) (Plan, error) {
	plistPath := launchdPlistPath(opts)
	label := launchdLabel(opts)
	plan := Plan{GOOS: "darwin", Action: opts.Action}
	switch opts.Action {
	case "install":
		plan.Files = append(plan.Files, File{Path: plistPath, Mode: 0o644, Content: launchdPlist(opts)})
		if opts.StartNow {
			plan.Commands = append(plan.Commands, Command{Name: "launchctl", Args: []string{"bootstrap", launchdDomain(opts.Scope), plistPath}})
			plan.Commands = append(plan.Commands, Command{Name: "launchctl", Args: []string{"kickstart", "-k", launchdDomain(opts.Scope) + "/" + label}})
		}
	case "uninstall":
		plan.Commands = append(plan.Commands, Command{Name: "launchctl", Args: []string{"bootout", launchdDomain(opts.Scope), plistPath}})
		plan.Deletes = append(plan.Deletes, plistPath)
	case "start":
		plan.Commands = append(plan.Commands, Command{Name: "launchctl", Args: []string{"kickstart", "-k", launchdDomain(opts.Scope) + "/" + label}})
	case "stop":
		plan.Commands = append(plan.Commands, Command{Name: "launchctl", Args: []string{"kill", "TERM", launchdDomain(opts.Scope) + "/" + label}})
	case "restart":
		plan.Commands = append(plan.Commands, Command{Name: "launchctl", Args: []string{"kickstart", "-k", launchdDomain(opts.Scope) + "/" + label}})
	case "status":
		plan.Commands = append(plan.Commands, Command{Name: "launchctl", Args: []string{"print", launchdDomain(opts.Scope) + "/" + label}})
	default:
		return Plan{}, fmt.Errorf("unknown gateway service action %q", opts.Action)
	}
	return plan, nil
}

func windowsPlan(opts Options) (Plan, error) {
	taskName := `\ReamesAgent\` + opts.Name
	plan := Plan{GOOS: "windows", Action: opts.Action}
	switch opts.Action {
	case "install":
		args := []string{"/Create", "/F", "/SC", "ONLOGON", "/TN", taskName, "/TR", windowsTaskCommand(opts)}
		if opts.Scope == "system" {
			args = append(args, "/RU", "SYSTEM")
		}
		plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: args})
		if opts.StartNow {
			plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: []string{"/Run", "/TN", taskName}})
		}
	case "uninstall":
		plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: []string{"/Delete", "/F", "/TN", taskName}})
	case "start":
		plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: []string{"/Run", "/TN", taskName}})
	case "stop":
		plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: []string{"/End", "/TN", taskName}})
	case "restart":
		plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: []string{"/End", "/TN", taskName}})
		plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: []string{"/Run", "/TN", taskName}})
	case "status":
		plan.Commands = append(plan.Commands, Command{Name: "schtasks.exe", Args: []string{"/Query", "/TN", taskName, "/FO", "LIST", "/V"}})
	default:
		return Plan{}, fmt.Errorf("unknown gateway service action %q", opts.Action)
	}
	plan.Notes = append(plan.Notes, "Windows uses Scheduled Task for the gateway service")
	return plan, nil
}

func appendCredentialNotes(plan *Plan, opts Options) {
	if plan == nil {
		return
	}
	if opts.Home != "" {
		plan.Notes = append(plan.Notes,
			"gateway service pins REAMES_AGENT_HOME="+opts.Home,
			"provider and bot secrets stay in "+credentialEnvPath(opts.Home)+"; service definitions do not embed secret values",
		)
		return
	}
	plan.Notes = append(plan.Notes, "no --home supplied; gateway service will use the platform default Reames Agent home for config and credentials")
}

func credentialEnvPath(home string) string {
	home = strings.TrimSpace(home)
	if home == "" {
		return ".env"
	}
	if strings.Contains(home, `\`) && !strings.Contains(home, `/`) {
		return strings.TrimRight(home, `\`) + `\.env`
	}
	return strings.TrimRight(home, `/`) + "/.env"
}

func gatewayArgs(opts Options) []string {
	args := []string{opts.Executable, "gateway", "run"}
	if strings.TrimSpace(opts.Channels) != "" {
		args = append(args, "--channels", strings.TrimSpace(opts.Channels))
	}
	if strings.TrimSpace(opts.Dir) != "" {
		args = append(args, "--dir", strings.TrimSpace(opts.Dir))
	}
	if strings.TrimSpace(opts.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(opts.Model))
	}
	return args
}

func systemdUnit(opts Options) string {
	return `[Unit]
Description=Reames Agent social gateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=` + joinSystemdExecArgs(gatewayArgs(opts)) + `
` + systemdEnvironment(opts) + `Restart=always
RestartSec=5
WorkingDirectory=` + systemdWorkingDirectory(opts) + `

[Install]
WantedBy=default.target
`
}

func launchdPlist(opts Options) string {
	var args strings.Builder
	for _, a := range gatewayArgs(opts) {
		fmt.Fprintf(&args, "    <string>%s</string>\n", html.EscapeString(a))
	}
	env := launchdEnvironment(opts)
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + html.EscapeString(launchdLabel(opts)) + `</string>
  <key>ProgramArguments</key>
  <array>
` + args.String() + `  </array>
` + env + `  <key>KeepAlive</key>
  <true/>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`
}

func linuxUnitPath(opts Options) string {
	if opts.Scope == "system" {
		return filepath.Join(string(filepath.Separator), "etc", "systemd", "system", opts.Name+".service")
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			base = filepath.Join(home, ".config")
		} else {
			base = ".config"
		}
	}
	return filepath.Join(base, "systemd", "user", opts.Name+".service")
}

func systemdEnvironment(opts Options) string {
	if opts.Home == "" {
		return ""
	}
	return "Environment=" + quoteSystemdWord("REAMES_AGENT_HOME="+opts.Home) + "\n"
}

func launchdEnvironment(opts Options) string {
	if opts.Home == "" {
		return ""
	}
	return `  <key>EnvironmentVariables</key>
  <dict>
    <key>REAMES_AGENT_HOME</key>
    <string>` + html.EscapeString(opts.Home) + `</string>
  </dict>
`
}

func launchdPlistPath(opts Options) string {
	file := launchdLabel(opts) + ".plist"
	if opts.Scope == "system" {
		return filepath.Join(string(filepath.Separator), "Library", "LaunchDaemons", file)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Library", "LaunchAgents", file)
	}
	return filepath.Join("Library", "LaunchAgents", file)
}

func launchdLabel(opts Options) string {
	return "com.reames-agent." + opts.Name
}

func launchdDomain(scope string) string {
	if scope == "system" {
		return "system"
	}
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func systemctlArgs(scope string, args ...string) []string {
	if scope == "user" {
		return append([]string{"--user"}, args...)
	}
	return args
}

func systemdWorkingDirectory(opts Options) string {
	if opts.Dir != "" {
		return escapeSystemdPath(opts.Dir)
	}
	return "~"
}

func windowsTaskCommand(opts Options) string {
	if opts.Home == "" {
		return joinWindowsQuoted(gatewayArgs(opts))
	}
	script := `set "REAMES_AGENT_HOME=` + strings.ReplaceAll(opts.Home, `"`, `\"`) + `" && ` + joinWindowsQuoted(gatewayArgs(opts))
	return joinWindowsQuoted([]string{"cmd.exe", "/C", script})
}

func shellLine(c Command) string {
	return joinQuoted(append([]string{c.Name}, c.Args...))
}

func joinQuoted(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, strconv.Quote(a))
	}
	return strings.Join(out, " ")
}

func joinSystemdExecArgs(args []string) string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, quoteSystemdExecArg(arg))
	}
	return strings.Join(out, " ")
}

func quoteSystemdExecArg(value string) string {
	return quoteSystemdWord(strings.ReplaceAll(value, `$`, `$$`))
}

func quoteSystemdWord(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, `%`, `%%`)
	return `"` + value + `"`
}

func escapeSystemdPath(path string) string {
	return strings.ReplaceAll(path, "%", "%%")
}

func joinWindowsQuoted(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, windowsQuote(a))
	}
	return strings.Join(out, " ")
}

func windowsQuote(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func cleanPath(path string) string {
	path = os.ExpandEnv(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = home
		}
	} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, path[2:])
		}
	}
	return path
}
