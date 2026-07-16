// Package pluginregistry resolves plugin releases from a TUF-protected
// registry. The initial root metadata is supplied out of band; downloaded
// metadata and targets are verified and cached for rollback/freeze protection.
package pluginregistry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/theupdateframework/go-tuf/v2/metadata"
	tufconfig "github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
	"golang.org/x/text/unicode/norm"

	"reames-agent/internal/config"
	"reames-agent/internal/pluginpkg"
)

const (
	// SchemaVersion is the signed plugins.json schema understood by this client.
	SchemaVersion = 1
	// DefaultIndexTarget is the TUF target containing the signed registry index.
	DefaultIndexTarget = "plugins.json"
	// GitTreeDigestPrefix identifies the cross-platform canonical Git source-tree
	// digest used by signed registry entries. It is deliberately distinct from
	// pluginpkg's installed working-tree digest, whose executable bits reflect
	// the local operating system.
	GitTreeDigestPrefix = "sha256-git-tree-v1:"
	// SourceKind and TrustStatus are persisted with registry-installed releases.
	SourceKind  = "tuf-registry"
	TrustStatus = "tuf-registry-signed"

	maxIndexBytes       = 1 << 20
	maxAttestationBytes = 1 << 20
	maxRegistryEntries  = 4096
	maxRegistryName     = 128
	maxTargetPath       = 1024
	maxVersionLength    = 128
	maxDescription      = 4096
	maxAuthor           = 256
	maxCategory         = 128
	maxDisplayURL       = 2048
)

// Options configures a TUF registry client. MetadataURL, TargetsURL and
// TrustedRootPath are explicit trust/configuration inputs; there is no built-in
// public registry until the project operates and audits one.
type Options struct {
	MetadataURL     string
	TargetsURL      string
	TrustedRootPath string
	IndexTarget     string
	CacheBaseDir    string
	HTTPClient      *http.Client
}

// Index is the authenticated registry discovery document.
type Index struct {
	SchemaVersion int       `json:"schemaVersion"`
	Registry      string    `json:"registry"`
	Updated       time.Time `json:"updated"`
	Plugins       []Entry   `json:"plugins"`
}

// Entry binds human-facing discovery metadata to one immutable source tree.
// TUF authenticates the document; install_source still clones the exact
// revision and recomputes Digest before persisting the release.
type Entry struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Version     string     `json:"version"`
	Author      string     `json:"author,omitempty"`
	Source      string     `json:"source"`
	Subpath     string     `json:"subpath,omitempty"`
	Revision    string     `json:"revision"`
	Digest      string     `json:"digest"`
	Permissions []string   `json:"permissions"`
	Homepage    string     `json:"homepage,omitempty"`
	Category    string     `json:"category,omitempty"`
	Provenance  Provenance `json:"provenance"`

	RegistryName          string `json:"-"`
	RegistryMetadataURL   string `json:"-"`
	BootstrapRootSHA256   string `json:"-"`
	RootVersion           int64  `json:"-"`
	ReleaseEvidenceSHA256 string `json:"-"`
	ProvenanceStatus      string `json:"-"`
	AttestationSHA256     string `json:"-"`
}

// Provenance is the registry signer's authenticated assertion about the source
// tree. It is deliberately not labelled SLSA: an optional attestation target is
// integrity-verified by TUF, but builder identity/predicate policy is not yet a
// general SLSA verifier.
type Provenance struct {
	Source            string `json:"source"`
	Subpath           string `json:"subpath,omitempty"`
	Revision          string `json:"revision"`
	Digest            string `json:"digest"`
	BuilderID         string `json:"builderId,omitempty"`
	AttestationTarget string `json:"attestationTarget,omitempty"`
}

// Client serializes refreshes because go-tuf Updater instances and the local
// metadata cache are not safe for concurrent use.
type Client struct {
	opts Options
	gate chan struct{}
}

// New validates immutable client configuration. Root bytes are read during
// refresh so an explicit out-of-band root replacement takes effect immediately
// and selects a separate cache namespace.
func New(opts Options) (*Client, error) {
	opts.MetadataURL = strings.TrimRight(strings.TrimSpace(opts.MetadataURL), "/")
	opts.TargetsURL = strings.TrimRight(strings.TrimSpace(opts.TargetsURL), "/")
	opts.TrustedRootPath = strings.TrimSpace(opts.TrustedRootPath)
	opts.CacheBaseDir = strings.TrimSpace(opts.CacheBaseDir)
	opts.IndexTarget = strings.TrimSpace(opts.IndexTarget)
	if opts.IndexTarget == "" {
		opts.IndexTarget = DefaultIndexTarget
	}
	if err := validateRemoteBaseURL(opts.MetadataURL, "metadata"); err != nil {
		return nil, err
	}
	if opts.TargetsURL == "" {
		opts.TargetsURL = defaultTargetsURL(opts.MetadataURL)
	}
	if err := validateRemoteBaseURL(opts.TargetsURL, "targets"); err != nil {
		return nil, err
	}
	if opts.TrustedRootPath == "" {
		return nil, fmt.Errorf("plugin registry trusted_root is not configured")
	}
	if opts.CacheBaseDir == "" {
		return nil, fmt.Errorf("plugin registry cache directory is not configured")
	}
	if err := validateTargetPath(opts.IndexTarget); err != nil {
		return nil, fmt.Errorf("plugin registry index target: %w", err)
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &Client{opts: opts, gate: gate}, nil
}

// NewConfigured constructs the shared registry client used by CLI, Desktop,
// and install_source from the already merged runtime configuration.
func NewConfigured(cfg *config.Config, client *http.Client) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("plugin registry configuration is unavailable")
	}
	return New(Options{
		MetadataURL:     cfg.PluginRegistryMetadataURL(),
		TargetsURL:      cfg.PluginRegistryTargetsURL(),
		TrustedRootPath: cfg.PluginRegistryTrustedRootPath(),
		IndexTarget:     cfg.PluginRegistry.IndexTarget,
		CacheBaseDir:    config.PluginRegistryCacheDir(),
		HTTPClient:      client,
	})
}

// Refresh verifies current TUF metadata and returns the authenticated index.
func (c *Client) Refresh(ctx context.Context) (*Index, error) {
	index, _, err := c.load(ctx, "")
	return index, err
}

// Resolve verifies current metadata, returns one named immutable release, and
// verifies its optional TUF-targeted attestation payload.
func (c *Client) Resolve(ctx context.Context, name string) (Entry, error) {
	name = strings.TrimSpace(name)
	if !pluginpkg.IsValidName(name) {
		return Entry{}, fmt.Errorf("invalid registry plugin name %q", name)
	}
	_, entry, err := c.load(ctx, name)
	if err != nil {
		return Entry{}, err
	}
	if entry == nil {
		return Entry{}, fmt.Errorf("plugin %q is not present in the configured registry", name)
	}
	return *entry, nil
}

func (c *Client) load(ctx context.Context, resolveName string) (*Index, *Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-c.gate:
	}
	defer func() { c.gate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	rootBytes, err := readLimitedRegularFile(c.opts.TrustedRootPath, 512000)
	if err != nil {
		return nil, nil, fmt.Errorf("read plugin registry trusted root: %w", err)
	}
	cacheBase, err := ensurePrivateDirectory(c.opts.CacheBaseDir)
	if err != nil {
		return nil, nil, fmt.Errorf("secure plugin registry cache base: %w", err)
	}
	cacheDir, err := ensurePrivateDirectory(cacheNamespace(cacheBase, c.opts.MetadataURL, rootBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("create plugin registry cache: %w", err)
	}
	lock, err := takeCacheLock(ctx, filepath.Join(cacheDir, "registry.lock"))
	if err != nil {
		return nil, nil, fmt.Errorf("lock plugin registry cache: %w", err)
	}
	defer lock.release()
	defer func() { _ = hardenCacheTree(cacheDir) }()
	if err := hardenCacheTree(cacheDir); err != nil {
		return nil, nil, fmt.Errorf("validate plugin registry cache: %w", err)
	}

	metadataDir, err := ensurePrivateDirectory(filepath.Join(cacheDir, "metadata"))
	if err != nil {
		return nil, nil, fmt.Errorf("secure plugin registry metadata cache: %w", err)
	}
	targetsDir, err := ensurePrivateDirectory(filepath.Join(cacheDir, "targets"))
	if err != nil {
		return nil, nil, fmt.Errorf("secure plugin registry target cache: %w", err)
	}
	trustedRoot := rootBytes
	if cached, readErr := readLimitedRegularFile(filepath.Join(metadataDir, "root.json"), 512000); readErr == nil {
		trustedRoot = cached
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read cached plugin registry root: %w", readErr)
	}

	cfg, err := tufconfig.New(c.opts.MetadataURL, trustedRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("configure plugin registry TUF client: %w", err)
	}
	cfg.LocalMetadataDir = metadataDir
	cfg.LocalTargetsDir = targetsDir
	cfg.RemoteTargetsURL = c.opts.TargetsURL
	cfg.PrefixTargetsWithHash = true
	cfg.MaxRootRotations = 32
	cfg.MaxDelegations = 8
	cfg.RootMaxLength = 512000
	cfg.TimestampMaxLength = 16384
	cfg.SnapshotMaxLength = 1 << 20
	cfg.TargetsMaxLength = 2 << 20
	cfg.Fetcher = &contextFetcher{ctx: ctx, client: scopedHTTPClient(c.opts.HTTPClient)}

	up, err := updater.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize plugin registry trust: %w", err)
	}
	if err := up.Refresh(); err != nil {
		return nil, nil, fmt.Errorf("refresh plugin registry trust metadata: %w", err)
	}
	if err := hardenCacheTree(cacheDir); err != nil {
		return nil, nil, fmt.Errorf("harden refreshed plugin registry cache: %w", err)
	}
	info, err := up.GetTargetInfo(c.opts.IndexTarget)
	if err != nil {
		return nil, nil, fmt.Errorf("locate authenticated registry index %q: %w", c.opts.IndexTarget, err)
	}
	if info.Length <= 0 || info.Length > maxIndexBytes {
		return nil, nil, fmt.Errorf("authenticated registry index length %d is outside 1..%d bytes", info.Length, maxIndexBytes)
	}
	indexDestination, err := secureCacheDestination(cfg.LocalTargetsDir, c.opts.IndexTarget)
	if err != nil {
		return nil, nil, fmt.Errorf("secure authenticated registry index cache: %w", err)
	}
	_, body, err := up.DownloadTarget(info, indexDestination, "")
	if err != nil {
		return nil, nil, fmt.Errorf("download authenticated registry index: %w", err)
	}
	if err := hardenCacheTree(cacheDir); err != nil {
		return nil, nil, fmt.Errorf("harden downloaded plugin registry index: %w", err)
	}
	index, err := decodeAndValidateIndex(body)
	if err != nil {
		return nil, nil, err
	}
	bootstrapHash := sha256.Sum256(rootBytes)
	trusted := up.GetTrustedMetadataSet()
	for i := range index.Plugins {
		releaseEvidenceSHA256, err := registryEntryDigest(index.SchemaVersion, index.Registry, c.opts.IndexTarget, index.Plugins[i])
		if err != nil {
			return nil, nil, fmt.Errorf("digest authenticated registry entry %q: %w", index.Plugins[i].Name, err)
		}
		index.Plugins[i].RegistryName = index.Registry
		index.Plugins[i].RegistryMetadataURL = c.opts.MetadataURL
		index.Plugins[i].BootstrapRootSHA256 = "sha256:" + hex.EncodeToString(bootstrapHash[:])
		index.Plugins[i].RootVersion = trusted.Root.Signed.Version
		index.Plugins[i].ReleaseEvidenceSHA256 = releaseEvidenceSHA256
		index.Plugins[i].ProvenanceStatus = "registry-assertion-tuf-authenticated"
	}

	var resolved *Entry
	if resolveName != "" {
		for i := range index.Plugins {
			if index.Plugins[i].Name == resolveName {
				resolved = &index.Plugins[i]
				break
			}
		}
		if resolved != nil && resolved.Provenance.AttestationTarget != "" {
			digest, err := verifyAttestationTarget(up, cfg.LocalTargetsDir, resolved.Provenance.AttestationTarget)
			if err != nil {
				return nil, nil, fmt.Errorf("verify provenance attestation for %s: %w", resolveName, err)
			}
			resolved.AttestationSHA256 = digest
			resolved.ProvenanceStatus = "tuf-attestation-target-integrity-verified"
		}
	}
	if err := hardenCacheTree(cacheDir); err != nil {
		return nil, nil, fmt.Errorf("harden plugin registry cache: %w", err)
	}
	return index, resolved, nil
}

// registryEntryDigest binds approval and lifecycle state to the complete,
// normalized signed release entry and to the registry schema/index context in
// which that entry was authenticated. Runtime-only TUF evidence fields use
// json:"-" and therefore cannot make the digest self-referential.
func registryEntryDigest(schemaVersion int, registry, indexTarget string, entry Entry) (string, error) {
	payload := struct {
		SchemaVersion int    `json:"schemaVersion"`
		Registry      string `json:"registry"`
		IndexTarget   string `json:"indexTarget"`
		Entry         Entry  `json:"entry"`
	}{
		SchemaVersion: schemaVersion,
		Registry:      registry,
		IndexTarget:   indexTarget,
		Entry:         entry,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func verifyAttestationTarget(up *updater.Updater, targetDir, target string) (string, error) {
	info, err := up.GetTargetInfo(target)
	if err != nil {
		return "", fmt.Errorf("locate authenticated target %q: %w", target, err)
	}
	if info.Length <= 0 || info.Length > maxAttestationBytes {
		return "", fmt.Errorf("attestation target length %d is outside 1..%d bytes", info.Length, maxAttestationBytes)
	}
	destination, err := secureCacheDestination(targetDir, target)
	if err != nil {
		return "", fmt.Errorf("secure attestation cache destination: %w", err)
	}
	_, body, err := up.DownloadTarget(info, destination, "")
	if err != nil {
		return "", fmt.Errorf("download authenticated target %q: %w", target, err)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func decodeAndValidateIndex(body []byte) (*Index, error) {
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return nil, fmt.Errorf("decode authenticated plugin registry index: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var index Index
	if err := dec.Decode(&index); err != nil {
		return nil, fmt.Errorf("decode authenticated plugin registry index: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, fmt.Errorf("decode authenticated plugin registry index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("plugin registry schemaVersion %d is unsupported (want %d)", index.SchemaVersion, SchemaVersion)
	}
	index.Registry = strings.TrimSpace(index.Registry)
	if err := validateDisplayText("plugin registry name", index.Registry, maxRegistryName, true); err != nil {
		return nil, err
	}
	if index.Updated.IsZero() {
		return nil, fmt.Errorf("plugin registry updated timestamp is missing")
	}
	if len(index.Plugins) > maxRegistryEntries {
		return nil, fmt.Errorf("plugin registry contains %d entries (maximum %d)", len(index.Plugins), maxRegistryEntries)
	}
	seen := make(map[string]bool, len(index.Plugins))
	for i := range index.Plugins {
		if err := validateEntry(&index.Plugins[i]); err != nil {
			return nil, fmt.Errorf("plugin registry entry %d: %w", i, err)
		}
		key, _ := pluginpkg.CanonicalNameKey(index.Plugins[i].Name)
		if seen[key] {
			return nil, fmt.Errorf("plugin registry contains cross-platform duplicate plugin %q", index.Plugins[i].Name)
		}
		seen[key] = true
	}
	sort.Slice(index.Plugins, func(i, j int) bool { return index.Plugins[i].Name < index.Plugins[j].Name })
	return &index, nil
}

func validateEntry(entry *Entry) error {
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Description = strings.TrimSpace(entry.Description)
	entry.Version = strings.TrimSpace(entry.Version)
	entry.Author = strings.TrimSpace(entry.Author)
	entry.Source = strings.TrimSpace(entry.Source)
	entry.Subpath = strings.TrimSpace(entry.Subpath)
	entry.Revision = strings.ToLower(strings.TrimSpace(entry.Revision))
	entry.Digest = strings.ToLower(strings.TrimSpace(entry.Digest))
	entry.Homepage = strings.TrimSpace(entry.Homepage)
	entry.Category = strings.TrimSpace(entry.Category)
	if !pluginpkg.IsValidName(entry.Name) {
		return fmt.Errorf("invalid plugin name %q", entry.Name)
	}
	if err := validateDisplayText("version", entry.Version, maxVersionLength, true); err != nil {
		return fmt.Errorf("plugin %s %w", entry.Name, err)
	}
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "description", value: entry.Description, limit: maxDescription},
		{name: "author", value: entry.Author, limit: maxAuthor},
		{name: "category", value: entry.Category, limit: maxCategory},
	} {
		if err := validateDisplayText(field.name, field.value, field.limit, false); err != nil {
			return fmt.Errorf("plugin %s %w", entry.Name, err)
		}
	}
	if err := validateGitHubRepository(entry.Source); err != nil {
		return fmt.Errorf("plugin %s source: %w", entry.Name, err)
	}
	if entry.Subpath != "" {
		if err := validateTargetPath(entry.Subpath); err != nil {
			return fmt.Errorf("plugin %s subpath: %w", entry.Name, err)
		}
	}
	if !isHexDigest(entry.Revision, 40) {
		return fmt.Errorf("plugin %s revision must be a full 40-character Git commit", entry.Name)
	}
	if !strings.HasPrefix(entry.Digest, GitTreeDigestPrefix) || !isHexDigest(strings.TrimPrefix(entry.Digest, GitTreeDigestPrefix), 64) {
		return fmt.Errorf("plugin %s digest must use sha256-git-tree-v1 with 64 lowercase hex characters", entry.Name)
	}
	permissions, err := pluginpkg.NormalizePermissions(entry.Permissions)
	if err != nil {
		return fmt.Errorf("plugin %s permissions: %w", entry.Name, err)
	}
	entry.Permissions = permissions
	entry.Provenance.Source = strings.TrimSpace(entry.Provenance.Source)
	entry.Provenance.Subpath = strings.TrimSpace(entry.Provenance.Subpath)
	entry.Provenance.Revision = strings.ToLower(strings.TrimSpace(entry.Provenance.Revision))
	entry.Provenance.Digest = strings.ToLower(strings.TrimSpace(entry.Provenance.Digest))
	entry.Provenance.BuilderID = strings.TrimSpace(entry.Provenance.BuilderID)
	entry.Provenance.AttestationTarget = strings.TrimSpace(entry.Provenance.AttestationTarget)
	if entry.Provenance.Source != entry.Source || entry.Provenance.Subpath != entry.Subpath || entry.Provenance.Revision != entry.Revision || entry.Provenance.Digest != entry.Digest {
		return fmt.Errorf("plugin %s provenance does not bind the exact source, subpath, revision, and digest", entry.Name)
	}
	if entry.Provenance.AttestationTarget != "" {
		if err := validateTargetPath(entry.Provenance.AttestationTarget); err != nil {
			return fmt.Errorf("plugin %s attestation target: %w", entry.Name, err)
		}
	}
	if entry.Homepage != "" {
		if err := validateDisplayText("homepage", entry.Homepage, maxDisplayURL, false); err != nil {
			return fmt.Errorf("plugin %s %w", entry.Name, err)
		}
		if u, err := url.Parse(entry.Homepage); err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("plugin %s homepage must be an absolute HTTPS URL", entry.Name)
		}
	}
	if err := validateDisplayText("provenance builderId", entry.Provenance.BuilderID, maxDisplayURL, false); err != nil {
		return fmt.Errorf("plugin %s %w", entry.Name, err)
	}
	return nil
}

// Search filters an authenticated index by name, description, author, or
// category. Results retain the stable name order established during validation.
func Search(index *Index, query string) []Entry {
	if index == nil {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]Entry(nil), index.Plugins...)
	}
	result := make([]Entry, 0)
	for _, entry := range index.Plugins {
		if strings.Contains(strings.ToLower(entry.Name), query) ||
			strings.Contains(strings.ToLower(entry.Description), query) ||
			strings.Contains(strings.ToLower(entry.Author), query) ||
			strings.Contains(strings.ToLower(entry.Category), query) {
			result = append(result, entry)
		}
	}
	return result
}

func validateGitHubRepository(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return fmt.Errorf("must be a canonical HTTPS github.com repository URL")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || !isGitHubPathComponent(parts[0]) || !isGitHubPathComponent(parts[1]) || strings.HasSuffix(strings.ToLower(parts[1]), ".git") {
		return fmt.Errorf("must identify exactly github.com/<owner>/<repository>")
	}
	if u.Path != "/"+parts[0]+"/"+parts[1] {
		return fmt.Errorf("must be a canonical HTTPS github.com repository URL")
	}
	return nil
}

func isGitHubPathComponent(value string) bool {
	if value == "" || len(value) > 100 || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validateDisplayText(name, value string, limit int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if len(value) > limit {
		return fmt.Errorf("%s exceeds %d bytes", name, limit)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return fmt.Errorf("%s contains control or formatting characters", name)
		}
	}
	return nil
}

func validateRemoteBaseURL(raw, label string) error {
	if raw == "" {
		return fmt.Errorf("plugin registry %s URL is not configured", label)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("plugin registry %s URL must be an absolute URL without credentials, query, or fragment", label)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := strings.Trim(u.Hostname(), "[]")
		if strings.EqualFold(host, "localhost") {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("plugin registry %s URL must use HTTPS (HTTP is allowed only for loopback tests)", label)
}

func defaultTargetsURL(metadataURL string) string {
	u, err := url.Parse(metadataURL)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimRight(u.Path, "/")
	if path.Base(trimmed) == "metadata" {
		u.Path = path.Join(path.Dir(trimmed), "targets")
	} else {
		u.Path = path.Join(trimmed, "targets")
	}
	return strings.TrimRight(u.String(), "/")
}

func validateTargetPath(value string) error {
	if err := validateDisplayText("target path", value, maxTargetPath, true); err != nil {
		return err
	}
	if strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || value == "." || value == ".." || path.Clean(value) != value || strings.HasPrefix(value, "../") {
		return fmt.Errorf("%q is not a clean relative target path", value)
	}
	if !norm.NFC.IsNormalString(value) {
		return fmt.Errorf("%q must use Unicode NFC normalization", value)
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || len(component) > 255 || strings.TrimRight(component, ". ") != component || strings.ContainsAny(component, `<>:"|?*`) || isWindowsReservedTargetComponent(component) {
			return fmt.Errorf("%q is not portable across supported platforms", value)
		}
	}
	return nil
}

func isWindowsReservedTargetComponent(component string) bool {
	base, _, _ := strings.Cut(component, ".")
	base = strings.ToUpper(base)
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
}

func isHexDigest(value string, length int) bool {
	if len(value) != length || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func cacheNamespace(base, metadataURL string, root []byte) string {
	rootHash := sha256.Sum256(root)
	identity := sha256.Sum256([]byte(metadataURL + "\x00" + hex.EncodeToString(rootHash[:])))
	return filepath.Join(base, hex.EncodeToString(identity[:16]))
}

func readLimitedRegularFile(name string, limit int64) ([]byte, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if unsafe, reason := unsafePathEntry(name, before); unsafe {
		return nil, fmt.Errorf("%s is unsafe: %s", name, reason)
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > limit {
		return nil, fmt.Errorf("%s is not a regular file between 1 and %d bytes", name, limit)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit || !os.SameFile(before, info) {
		return nil, fmt.Errorf("%s is not a regular file between 1 and %d bytes", name, limit)
	}
	return io.ReadAll(io.LimitReader(f, limit+1))
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("unexpected trailing JSON value")
}

type contextFetcher struct {
	ctx    context.Context
	client *http.Client
}

func (f *contextFetcher) DownloadFile(raw string, maxLength int64, _ time.Duration) ([]byte, error) {
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "reames-agent-plugin-registry/1")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &metadata.ErrDownloadHTTP{StatusCode: resp.StatusCode, URL: req.URL.Redacted()}
	}
	if resp.ContentLength > maxLength {
		return nil, fmt.Errorf("GET %s advertises %d bytes (maximum %d)", req.URL.Redacted(), resp.ContentLength, maxLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLength+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxLength {
		return nil, fmt.Errorf("GET %s exceeded %d bytes", req.URL.Redacted(), maxLength)
	}
	return body, nil
}

func scopedHTTPClient(source *http.Client) *http.Client {
	if source == nil {
		source = &http.Client{}
	}
	clone := *source
	prior := clone.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(req.URL, via[0].URL) {
			return fmt.Errorf("plugin registry redirect changed origin from %s to %s", via[0].URL.Redacted(), req.URL.Redacted())
		}
		if prior != nil {
			return prior(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	if clone.Timeout == 0 {
		clone.Timeout = 20 * time.Second
	}
	return &clone
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
