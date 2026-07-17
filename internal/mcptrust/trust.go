// Package mcptrust owns host-local, identity-bound trust receipts for MCP
// servers and their reader tools. Receipt state never enters model prompts or
// provider-visible schemas; it only decides whether an external tool may use
// Reames Agent's read-only execution authority.
package mcptrust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/fileutil"
)

const (
	StoreVersion  = 1
	StateFilename = "mcp-security.json"
)

type Scope string

const (
	ScopeSession   Scope = "session"
	ScopeWorkspace Scope = "workspace"
)

type Source string

const (
	SourceUser         Source = "user"
	SourceLegacyImport Source = "legacy_import"
)

type TrustState string

const (
	TrustUntrusted TrustState = "untrusted"
	TrustSession   TrustState = "session"
	TrustWorkspace TrustState = "workspace"
	TrustChanged   TrustState = "changed"
)

// Identity is the secret-free canonical input to a server identity digest.
// Credential values are excluded so rotation does not invalidate a receipt;
// environment/header names remain identity-bearing.
type Identity struct {
	Server         string            `json:"server"`
	Transport      string            `json:"transport"`
	CommandPath    string            `json:"command_path,omitempty"`
	CommandSHA256  string            `json:"command_sha256,omitempty"`
	Args           []string          `json:"args,omitempty"`
	ArgFiles       []ArgFileIdentity `json:"arg_files,omitempty"`
	Dir            string            `json:"dir,omitempty"`
	URL            string            `json:"url,omitempty"`
	EnvKeys        []string          `json:"env_keys,omitempty"`
	HeaderKeys     []string          `json:"header_keys,omitempty"`
	PackageOwner   string            `json:"package_owner,omitempty"`
	PackageRoot    string            `json:"package_root,omitempty"`
	LauncherDigest string            `json:"launcher_digest,omitempty"`
	ConfigSource   string            `json:"config_source,omitempty"`
}

// ArgFileIdentity binds interpreter-style launchers such as `node server.js`
// to the actual script/archive bytes, not only to the interpreter executable.
type ArgFileIdentity struct {
	Index  int    `json:"index"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Capability is the host-observed security snapshot for one raw MCP tool.
type Capability struct {
	RawName      string          `json:"raw_name"`
	ModelName    string          `json:"model_name"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
	ReadOnly     bool            `json:"read_only"`
	Destructive  bool            `json:"destructive"`
}

type ToolReceipt struct {
	RawName       string `json:"raw_name"`
	ModelName     string `json:"model_name"`
	Fingerprint   string `json:"fingerprint"`
	ReadOnly      bool   `json:"read_only"`
	Destructive   bool   `json:"destructive"`
	TrustedReader bool   `json:"trusted_reader,omitempty"`
}

type Receipt struct {
	Scope                Scope         `json:"scope"`
	WorkspaceFingerprint string        `json:"workspace_fingerprint"`
	Server               string        `json:"server"`
	ConfigSource         string        `json:"config_source,omitempty"`
	IdentityFingerprint  string        `json:"identity_fingerprint"`
	Tools                []ToolReceipt `json:"tools"`
	Source               Source        `json:"source"`
	CreatedAt            time.Time     `json:"created_at"`
	LastVerifiedAt       time.Time     `json:"last_verified_at"`
}

type LauncherLock struct {
	Server          string    `json:"server"`
	Workspace       string    `json:"workspace_fingerprint"`
	Locator         string    `json:"locator"`
	ResolvedVersion string    `json:"resolved_version"`
	ContentSHA256   string    `json:"content_sha256"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type State struct {
	Version       int            `json:"version"`
	Receipts      []Receipt      `json:"receipts,omitempty"`
	LauncherLocks []LauncherLock `json:"launcher_locks,omitempty"`
	LegacyImports []string       `json:"legacy_imports,omitempty"`
}

type ToolChange struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Evaluation struct {
	State           TrustState
	Source          Source
	Scope           Scope
	IdentityChanged bool
	TrustedReaders  map[string]bool
	ChangedTools    []string
	ToolChanges     []ToolChange
}

type Manager struct {
	path                 string
	workspaceFingerprint string

	mu      sync.Mutex
	session []Receipt
}

var managerRegistry struct {
	sync.Mutex
	items map[string]*Manager
}

func StatePath(reamesHome string) string {
	if strings.TrimSpace(reamesHome) == "" {
		return ""
	}
	return filepath.Join(reamesHome, StateFilename)
}

func NewManager(path, workspace string) *Manager {
	return &Manager{path: path, workspaceFingerprint: WorkspaceFingerprint(workspace)}
}

// ForWorkspace shares session receipts across controller tabs using the same
// Reames home and workspace.
func ForWorkspace(reamesHome, workspace string) *Manager {
	path := StatePath(reamesHome)
	workspaceFP := WorkspaceFingerprint(workspace)
	key := path + "\x00" + workspaceFP
	managerRegistry.Lock()
	defer managerRegistry.Unlock()
	if managerRegistry.items == nil {
		managerRegistry.items = map[string]*Manager{}
	}
	if manager := managerRegistry.items[key]; manager != nil {
		return manager
	}
	manager := &Manager{path: path, workspaceFingerprint: workspaceFP}
	managerRegistry.items[key] = manager
	return manager
}

func WorkspaceFingerprint(workspace string) string {
	workspace = canonicalPath(workspace)
	if workspace == "" {
		return ""
	}
	return digestBytes([]byte(workspace))
}

func IdentityFingerprint(identity Identity) (string, error) {
	identity.Server = strings.TrimSpace(identity.Server)
	identity.Transport = normalizeTransport(identity.Transport)
	identity.CommandPath = canonicalPath(identity.CommandPath)
	identity.Dir = canonicalPath(identity.Dir)
	identity.PackageRoot = canonicalPath(identity.PackageRoot)
	identity.URL = strings.TrimSpace(identity.URL)
	identity.ConfigSource = strings.TrimSpace(identity.ConfigSource)
	identity.Args = append([]string(nil), identity.Args...)
	identity.ArgFiles = append([]ArgFileIdentity(nil), identity.ArgFiles...)
	for i := range identity.ArgFiles {
		identity.ArgFiles[i].Path = canonicalPath(identity.ArgFiles[i].Path)
	}
	identity.EnvKeys = cleanStrings(identity.EnvKeys, runtime.GOOS == "windows")
	identity.HeaderKeys = cleanStrings(identity.HeaderKeys, true)
	body, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	return digestBytes(body), nil
}

func CapabilityFingerprint(cap Capability) (string, error) {
	input, err := canonicalSecuritySchema(cap.InputSchema)
	if err != nil {
		return "", fmt.Errorf("input schema: %w", err)
	}
	output, err := canonicalSecuritySchema(cap.OutputSchema)
	if err != nil {
		return "", fmt.Errorf("output schema: %w", err)
	}
	payload := struct {
		RawName     string          `json:"raw_name"`
		ModelName   string          `json:"model_name"`
		Input       json.RawMessage `json:"input,omitempty"`
		Output      json.RawMessage `json:"output,omitempty"`
		ReadOnly    bool            `json:"read_only"`
		Destructive bool            `json:"destructive"`
	}{
		RawName: strings.TrimSpace(cap.RawName), ModelName: strings.TrimSpace(cap.ModelName),
		Input: input, Output: output, ReadOnly: cap.ReadOnly, Destructive: cap.Destructive,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return digestBytes(body), nil
}

func FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (m *Manager) WorkspaceFingerprint() string { return m.workspaceFingerprint }

func (m *Manager) Load() (State, error) {
	if strings.TrimSpace(m.path) == "" {
		return State{Version: StoreVersion}, nil
	}
	body, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Version: StoreVersion}, nil
		}
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, fmt.Errorf("parse MCP trust state: %w", err)
	}
	if state.Version == 0 {
		state.Version = StoreVersion
	}
	if state.Version != StoreVersion {
		return State{}, fmt.Errorf("unsupported MCP trust state version %d", state.Version)
	}
	normalizeState(&state)
	return state, nil
}

// TrustReaders records every observed capability for drift detection while
// granting reader authority only to selected raw tool names.
func (m *Manager) TrustReaders(scope Scope, source Source, server, configSource, identityFingerprint string, capabilities []Capability, selected []string) error {
	if scope != ScopeSession && scope != ScopeWorkspace {
		return fmt.Errorf("invalid MCP trust scope %q", scope)
	}
	if strings.TrimSpace(server) == "" || strings.TrimSpace(identityFingerprint) == "" {
		return fmt.Errorf("MCP trust requires server and identity fingerprint")
	}
	selectedReaders := map[string]bool{}
	for _, name := range selected {
		if name = strings.TrimSpace(name); name != "" {
			selectedReaders[name] = true
		}
	}
	receipt, err := buildReceipt(scope, source, m.workspaceFingerprint, server, configSource, identityFingerprint, capabilities, selectedReaders, time.Now().UTC())
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if scope == ScopeSession {
		m.session = upsertReceipt(m.session, receipt)
		return nil
	}
	return m.updatePersistent(func(state *State) {
		state.Receipts = upsertReceipt(state.Receipts, receipt)
	})
}

func (m *Manager) Revoke(server string) error {
	server = strings.TrimSpace(server)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = removeReceipts(m.session, server, m.workspaceFingerprint)
	return m.updatePersistent(func(state *State) {
		state.Receipts = removeReceipts(state.Receipts, server, m.workspaceFingerprint)
	})
}

func (m *Manager) HasReceipt(server, configSource string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.Load()
	if err != nil {
		return false, err
	}
	receipts := append(append([]Receipt(nil), m.session...), state.Receipts...)
	_, ok := selectReceipt(receipts, strings.TrimSpace(server), strings.TrimSpace(configSource), m.workspaceFingerprint)
	return ok, nil
}

// SelectedReaders returns the receipt's previously selected raw names without
// granting authority. It only seeds an explicit identity reverify decision.
func (m *Manager) SelectedReaders(server, configSource string) ([]string, error) {
	selected, _, err := m.SelectedReadersWithScope(server, configSource)
	return selected, err
}

// SelectedReadersWithScope returns the previously selected raw reader names and
// the exact receipt scope. Identity/capability re-verification must preserve a
// session-only decision instead of silently upgrading it to workspace trust.
func (m *Manager) SelectedReadersWithScope(server, configSource string) ([]string, Scope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.Load()
	if err != nil {
		return nil, "", err
	}
	receipts := append(append([]Receipt(nil), m.session...), state.Receipts...)
	receipt, ok := selectReceipt(receipts, strings.TrimSpace(server), strings.TrimSpace(configSource), m.workspaceFingerprint)
	if !ok {
		return nil, "", nil
	}
	var out []string
	for _, tool := range receipt.Tools {
		if tool.TrustedReader {
			out = append(out, tool.RawName)
		}
	}
	sort.Strings(out)
	return out, receipt.Scope, nil
}

// ClearSessionReceipt removes an older session-scoped receipt after an
// explicit workspace identity re-verification. Session scope otherwise has
// higher precedence and would keep projecting the stale identity.
func (m *Manager) ClearSessionReceipt(server, configSource string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	server = strings.TrimSpace(server)
	configSource = strings.TrimSpace(configSource)
	out := m.session[:0]
	for _, receipt := range m.session {
		if receipt.Server == server && receipt.ConfigSource == configSource && receipt.WorkspaceFingerprint == m.workspaceFingerprint {
			continue
		}
		out = append(out, receipt)
	}
	m.session = out
}

func (m *Manager) IdentityChanged(server, configSource, identityFingerprint string) (hasReceipt, changed bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.Load()
	if err != nil {
		return false, false, err
	}
	receipts := append(append([]Receipt(nil), m.session...), state.Receipts...)
	receipt, ok := selectReceipt(receipts, strings.TrimSpace(server), strings.TrimSpace(configSource), m.workspaceFingerprint)
	if !ok {
		return false, false, nil
	}
	return true, receipt.IdentityFingerprint != identityFingerprint, nil
}

func (m *Manager) Evaluate(server, configSource, identityFingerprint string, capabilities []Capability) (Evaluation, error) {
	eval := Evaluation{State: TrustUntrusted, TrustedReaders: map[string]bool{}}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.Load()
	if err != nil {
		return eval, err
	}
	receipts := append(append([]Receipt(nil), m.session...), state.Receipts...)
	receipt, ok := selectReceipt(receipts, strings.TrimSpace(server), strings.TrimSpace(configSource), m.workspaceFingerprint)
	if !ok {
		return eval, nil
	}
	eval.Source, eval.Scope = receipt.Source, receipt.Scope
	if receipt.IdentityFingerprint != identityFingerprint {
		eval.State = TrustChanged
		eval.IdentityChanged = true
		return eval, nil
	}
	if receipt.Scope == ScopeSession {
		eval.State = TrustSession
	} else {
		eval.State = TrustWorkspace
	}
	live := make(map[string]Capability, len(capabilities))
	for _, cap := range capabilities {
		live[cap.RawName] = cap
	}
	for _, saved := range receipt.Tools {
		cap, exists := live[saved.RawName]
		if !exists {
			eval.ChangedTools = append(eval.ChangedTools, saved.RawName)
			eval.ToolChanges = append(eval.ToolChanges, ToolChange{Name: saved.RawName, Kind: "removed"})
			continue
		}
		fingerprint, fpErr := CapabilityFingerprint(cap)
		if fpErr != nil {
			return eval, fpErr
		}
		if saved.Fingerprint != fingerprint || saved.ReadOnly != cap.ReadOnly || saved.Destructive != cap.Destructive {
			eval.ChangedTools = append(eval.ChangedTools, saved.RawName)
			eval.ToolChanges = append(eval.ToolChanges, ToolChange{Name: saved.RawName, Kind: toolChangeKind(saved, cap)})
			continue
		}
		if saved.TrustedReader && cap.ReadOnly && !cap.Destructive {
			eval.TrustedReaders[saved.RawName] = true
		}
	}
	for _, cap := range capabilities {
		if _, ok := findToolReceipt(receipt.Tools, cap.RawName); !ok {
			eval.ChangedTools = append(eval.ChangedTools, cap.RawName)
			eval.ToolChanges = append(eval.ToolChanges, ToolChange{Name: cap.RawName, Kind: "added"})
		}
	}
	if len(eval.ChangedTools) > 0 {
		sort.Strings(eval.ChangedTools)
		eval.ChangedTools = compactStrings(eval.ChangedTools)
		sort.Slice(eval.ToolChanges, func(i, j int) bool {
			if eval.ToolChanges[i].Name != eval.ToolChanges[j].Name {
				return eval.ToolChanges[i].Name < eval.ToolChanges[j].Name
			}
			return eval.ToolChanges[i].Kind < eval.ToolChanges[j].Kind
		})
		eval.State = TrustChanged
	}
	return eval, nil
}

func (m *Manager) LegacyImported(server, configSource string) (bool, error) {
	key := m.legacyImportKey(server, configSource)
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.Load()
	if err != nil {
		return false, err
	}
	for _, imported := range state.LegacyImports {
		if imported == key {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) MarkLegacyImported(server, configSource string) error {
	key := m.legacyImportKey(server, configSource)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updatePersistent(func(state *State) {
		state.LegacyImports = append(state.LegacyImports, key)
	})
}

// ImportLegacyReaders atomically records the one-time legacy import marker and,
// when at least one eligible reader was selected, its workspace receipt. A
// crash or write failure therefore cannot leave a receipt without the marker
// and later re-authorize a revoked legacy raw-name list.
func (m *Manager) ImportLegacyReaders(server, configSource, identityFingerprint string, capabilities []Capability, selected []string) error {
	selectedReaders := map[string]bool{}
	for _, name := range selected {
		if name = strings.TrimSpace(name); name != "" {
			selectedReaders[name] = true
		}
	}
	var receipt Receipt
	var err error
	if len(selectedReaders) > 0 {
		receipt, err = buildReceipt(
			ScopeWorkspace,
			SourceLegacyImport,
			m.workspaceFingerprint,
			server,
			configSource,
			identityFingerprint,
			capabilities,
			selectedReaders,
			time.Now().UTC(),
		)
		if err != nil {
			return err
		}
	}
	key := m.legacyImportKey(server, configSource)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updatePersistent(func(state *State) {
		if len(selectedReaders) > 0 {
			state.Receipts = upsertReceipt(state.Receipts, receipt)
		}
		state.LegacyImports = append(state.LegacyImports, key)
	})
}

func (m *Manager) legacyImportKey(server, configSource string) string {
	payload := strings.Join([]string{m.workspaceFingerprint, strings.TrimSpace(server), strings.TrimSpace(configSource)}, "\x00")
	return digestBytes([]byte(payload))
}

func (m *Manager) GetLauncherLock(server, locator string) (LauncherLock, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.Load()
	if err != nil {
		return LauncherLock{}, false, err
	}
	for _, lock := range state.LauncherLocks {
		if lock.Server == strings.TrimSpace(server) && lock.Locator == strings.TrimSpace(locator) && lock.Workspace == m.workspaceFingerprint {
			return lock, true, nil
		}
	}
	return LauncherLock{}, false, nil
}

func (m *Manager) PutLauncherLock(lock LauncherLock) error {
	lock.Server = strings.TrimSpace(lock.Server)
	lock.Locator = strings.TrimSpace(lock.Locator)
	lock.ResolvedVersion = strings.TrimSpace(lock.ResolvedVersion)
	lock.ContentSHA256 = strings.TrimSpace(lock.ContentSHA256)
	if lock.Server == "" || lock.Locator == "" || lock.ResolvedVersion == "" || lock.ContentSHA256 == "" {
		return fmt.Errorf("incomplete MCP launcher lock")
	}
	lock.Workspace = m.workspaceFingerprint
	lock.UpdatedAt = time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updatePersistent(func(state *State) {
		for i := range state.LauncherLocks {
			if state.LauncherLocks[i].Server == lock.Server && state.LauncherLocks[i].Locator == lock.Locator && state.LauncherLocks[i].Workspace == lock.Workspace {
				state.LauncherLocks[i] = lock
				return
			}
		}
		state.LauncherLocks = append(state.LauncherLocks, lock)
	})
}

func LauncherLockFingerprint(lock LauncherLock) string {
	payload := struct{ Server, Workspace, Locator, ResolvedVersion, ContentSHA256 string }{
		lock.Server, lock.Workspace, lock.Locator, lock.ResolvedVersion, lock.ContentSHA256,
	}
	body, _ := json.Marshal(payload)
	return digestBytes(body)
}

func (m *Manager) updatePersistent(update func(*State)) error {
	if strings.TrimSpace(m.path) == "" {
		return fmt.Errorf("MCP trust state path is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	unlock, err := acquireStateFileLock(m.path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	state, err := m.Load()
	if err != nil {
		return err
	}
	update(&state)
	state.Version = StoreVersion
	normalizeState(&state)
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return fileutil.AtomicWriteFile(m.path, body, 0o600)
}

func buildReceipt(scope Scope, source Source, workspaceFP, server, configSource, identityFP string, capabilities []Capability, selected map[string]bool, now time.Time) (Receipt, error) {
	receipt := Receipt{
		Scope: scope, WorkspaceFingerprint: workspaceFP, Server: strings.TrimSpace(server),
		ConfigSource: strings.TrimSpace(configSource), IdentityFingerprint: strings.TrimSpace(identityFP),
		Source: source, CreatedAt: now, LastVerifiedAt: now,
	}
	for _, cap := range capabilities {
		rawName := strings.TrimSpace(cap.RawName)
		fingerprint, err := CapabilityFingerprint(cap)
		if err != nil {
			return Receipt{}, fmt.Errorf("fingerprint MCP tool %q: %w", cap.RawName, err)
		}
		receipt.Tools = append(receipt.Tools, ToolReceipt{
			RawName: rawName, ModelName: strings.TrimSpace(cap.ModelName), Fingerprint: fingerprint,
			ReadOnly: cap.ReadOnly, Destructive: cap.Destructive,
			TrustedReader: selected[rawName] && cap.ReadOnly && !cap.Destructive,
		})
	}
	sort.Slice(receipt.Tools, func(i, j int) bool { return receipt.Tools[i].RawName < receipt.Tools[j].RawName })
	return receipt, nil
}

func canonicalSecuritySchema(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	stripDisplayFields(value)
	body, err := json.Marshal(value)
	return json.RawMessage(body), err
}

func stripDisplayFields(value any) {
	switch current := value.(type) {
	case map[string]any:
		for _, key := range []string{"description", "title", "examples", "$comment"} {
			delete(current, key)
		}
		for key, child := range current {
			switch key {
			case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas", "dependentRequired", "dependencies":
				if named, ok := child.(map[string]any); ok {
					for _, schema := range named {
						stripDisplayFields(schema)
					}
					continue
				}
			}
			stripDisplayFields(child)
		}
	case []any:
		for _, child := range current {
			stripDisplayFields(child)
		}
	}
}

func normalizeState(state *State) {
	if state.Version == 0 {
		state.Version = StoreVersion
	}
	state.Receipts = dedupeReceipts(state.Receipts)
	for i := range state.Receipts {
		sort.Slice(state.Receipts[i].Tools, func(a, b int) bool {
			return state.Receipts[i].Tools[a].RawName < state.Receipts[i].Tools[b].RawName
		})
	}
	sort.Slice(state.Receipts, func(i, j int) bool {
		a, b := state.Receipts[i], state.Receipts[j]
		if a.Server != b.Server {
			return a.Server < b.Server
		}
		if a.WorkspaceFingerprint != b.WorkspaceFingerprint {
			return a.WorkspaceFingerprint < b.WorkspaceFingerprint
		}
		if a.ConfigSource != b.ConfigSource {
			return a.ConfigSource < b.ConfigSource
		}
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		return a.Source < b.Source
	})
	sort.Slice(state.LauncherLocks, func(i, j int) bool {
		a, b := state.LauncherLocks[i], state.LauncherLocks[j]
		if a.Server != b.Server {
			return a.Server < b.Server
		}
		if a.Workspace != b.Workspace {
			return a.Workspace < b.Workspace
		}
		return a.Locator < b.Locator
	})
	state.LegacyImports = cleanStrings(state.LegacyImports, false)
}

func upsertReceipt(receipts []Receipt, receipt Receipt) []Receipt {
	for i := range receipts {
		if sameReceiptKey(receipts[i], receipt) {
			if receipts[i].Source == SourceUser && receipt.Source == SourceLegacyImport {
				return receipts
			}
			receipt.CreatedAt = receipts[i].CreatedAt
			receipts[i] = receipt
			return receipts
		}
	}
	return append(receipts, receipt)
}

func dedupeReceipts(receipts []Receipt) []Receipt {
	var out []Receipt
	for _, receipt := range receipts {
		out = upsertReceipt(out, receipt)
	}
	return out
}

func removeReceipts(receipts []Receipt, server, workspaceFP string) []Receipt {
	out := receipts[:0]
	for _, receipt := range receipts {
		if receipt.Server == server && receipt.WorkspaceFingerprint == workspaceFP {
			continue
		}
		out = append(out, receipt)
	}
	return out
}

func selectReceipt(receipts []Receipt, server, configSource, workspaceFP string) (Receipt, bool) {
	var selected Receipt
	found := false
	for _, receipt := range receipts {
		if receipt.Server != server || receipt.ConfigSource != configSource || receipt.WorkspaceFingerprint != workspaceFP {
			continue
		}
		if !found || receiptRank(receipt) > receiptRank(selected) {
			selected, found = receipt, true
		}
	}
	return selected, found
}

func receiptRank(receipt Receipt) int {
	rank := 0
	if receipt.Scope == ScopeSession {
		rank += 10
	}
	if receipt.Source == SourceUser {
		rank++
	}
	return rank
}

func sameReceiptKey(a, b Receipt) bool {
	return a.Scope == b.Scope && a.WorkspaceFingerprint == b.WorkspaceFingerprint && a.Server == b.Server && a.ConfigSource == b.ConfigSource
}

func findToolReceipt(receipts []ToolReceipt, rawName string) (ToolReceipt, bool) {
	for _, receipt := range receipts {
		if receipt.RawName == rawName {
			return receipt, true
		}
	}
	return ToolReceipt{}, false
}

func toolChangeKind(saved ToolReceipt, live Capability) string {
	switch {
	case saved.ReadOnly && !saved.Destructive && live.Destructive:
		return "reader_to_destructive"
	case saved.ReadOnly && !saved.Destructive && !live.ReadOnly:
		return "reader_to_writer"
	case !saved.ReadOnly && live.ReadOnly && !live.Destructive:
		return "writer_to_reader"
	case saved.ReadOnly != live.ReadOnly || saved.Destructive != live.Destructive:
		return "safety_changed"
	case saved.ModelName != strings.TrimSpace(live.ModelName):
		return "name_changed"
	default:
		return "schema_changed"
	}
}

func normalizeTransport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "stdio":
		return "stdio"
	case "http", "streamable-http", "streamable_http":
		return "http"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func canonicalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func cleanStrings(values []string, fold bool) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := value
		if fold {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if fold {
			return strings.ToLower(out[i]) < strings.ToLower(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
