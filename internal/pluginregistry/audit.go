package pluginregistry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
	"github.com/theupdateframework/go-tuf/v2/metadata/trustedmetadata"
)

const productionAuditPolicyName = "reames-production-v1"

// AuditOptions identifies an already-published local TUF repository and the
// bootstrap root obtained out of band. AuditRepository never creates or reads
// private keys and never mutates the repository.
type AuditOptions struct {
	RepositoryDir   string
	TrustedRootPath string
	IndexTarget     string
	ReferenceTime   time.Time
	Policy          AuditPolicy
}

// AuditPolicy defines the public-metadata invariants expected from a
// production plugin registry. The zero value is normalized to the current
// Reames production baseline.
type AuditPolicy struct {
	Name                  string
	RootThreshold         int
	TargetsThreshold      int
	SnapshotThreshold     int
	TimestampThreshold    int
	MaxRootRotations      int
	MinRootRemaining      time.Duration
	MinTargetsRemaining   time.Duration
	MinSnapshotRemaining  time.Duration
	MinTimestampRemaining time.Duration
	MaxRootRemaining      time.Duration
	MaxTargetsRemaining   time.Duration
	MaxSnapshotRemaining  time.Duration
	MaxTimestampRemaining time.Duration
	MaxIndexAge           time.Duration
}

// ProductionAuditPolicy returns the fail-closed public registry policy. It is
// intentionally separate from the runtime TUF client: a client can consume a
// valid 1-of-1 private registry, while an operator claiming the public Reames
// production baseline must satisfy stronger role separation and expiry rules.
func ProductionAuditPolicy() AuditPolicy {
	return AuditPolicy{
		Name:                  productionAuditPolicyName,
		RootThreshold:         2,
		TargetsThreshold:      2,
		SnapshotThreshold:     1,
		TimestampThreshold:    1,
		MaxRootRotations:      32,
		MinRootRemaining:      30 * 24 * time.Hour,
		MinTargetsRemaining:   72 * time.Hour,
		MinSnapshotRemaining:  24 * time.Hour,
		MinTimestampRemaining: time.Hour,
		MaxRootRemaining:      400 * 24 * time.Hour,
		MaxTargetsRemaining:   45 * 24 * time.Hour,
		MaxSnapshotRemaining:  14 * 24 * time.Hour,
		MaxTimestampRemaining: 48 * time.Hour,
		MaxIndexAge:           45 * 24 * time.Hour,
	}
}

// AuditReport is stable, machine-readable evidence from a local repository
// audit. ExternalRequired is deliberately part of successful output so this
// evidence cannot be mistaken for an HSM, personnel-quorum, endpoint, or
// monitoring attestation.
type AuditReport struct {
	SchemaVersion       int             `json:"schemaVersion"`
	Policy              string          `json:"policy"`
	ReferenceTime       time.Time       `json:"referenceTime"`
	BootstrapRootSHA256 string          `json:"bootstrapRootSHA256"`
	RootVersions        []int64         `json:"rootVersions"`
	Roles               []AuditRole     `json:"roles"`
	Metadata            []AuditMetadata `json:"metadata"`
	Index               AuditIndex      `json:"index"`
	Attestations        []AuditTarget   `json:"attestations,omitempty"`
	Checks              []string        `json:"checks"`
	ExternalRequired    []string        `json:"externalRequired"`
}

// AuditRole records the final root policy for one top-level role.
type AuditRole struct {
	Name      string   `json:"name"`
	Threshold int      `json:"threshold"`
	KeyCount  int      `json:"keyCount"`
	KeyIDs    []string `json:"keyIds"`
}

// AuditMetadata records the authenticated top-level metadata generation.
type AuditMetadata struct {
	Role           string    `json:"role"`
	Version        int64     `json:"version"`
	Expires        time.Time `json:"expires"`
	SignatureCount int       `json:"signatureCount"`
	SHA256         string    `json:"sha256"`
}

// AuditIndex records the TUF-authenticated registry index.
type AuditIndex struct {
	Target      string    `json:"target"`
	SHA256      string    `json:"sha256"`
	Registry    string    `json:"registry"`
	Updated     time.Time `json:"updated"`
	PluginCount int       `json:"pluginCount"`
}

// AuditTarget records an authenticated optional target referenced by the
// registry index. This is integrity evidence, not DSSE/SLSA policy evidence.
type AuditTarget struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Length int64  `json:"length"`
}

// AuditRepository verifies a published repository through the same TUF trust
// transitions as the runtime client, then applies the stronger public operator
// policy to final root roles, expiration windows, index schema, and every
// referenced attestation target.
func AuditRepository(ctx context.Context, opts AuditOptions) (*AuditReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts.RepositoryDir = strings.TrimSpace(opts.RepositoryDir)
	opts.TrustedRootPath = strings.TrimSpace(opts.TrustedRootPath)
	opts.IndexTarget = strings.TrimSpace(opts.IndexTarget)
	if opts.RepositoryDir == "" {
		return nil, fmt.Errorf("plugin registry audit repository directory is required")
	}
	if opts.TrustedRootPath == "" {
		return nil, fmt.Errorf("plugin registry audit requires an out-of-band trusted root")
	}
	if err := requireOutOfBandRootPath(opts.RepositoryDir, opts.TrustedRootPath); err != nil {
		return nil, err
	}
	if opts.IndexTarget == "" {
		opts.IndexTarget = DefaultIndexTarget
	}
	if err := validateTargetPath(opts.IndexTarget); err != nil {
		return nil, fmt.Errorf("plugin registry audit index target: %w", err)
	}
	policy := normalizeAuditPolicy(opts.Policy)
	if err := validateAuditPolicy(policy); err != nil {
		return nil, err
	}
	ref := opts.ReferenceTime.UTC()
	if ref.IsZero() {
		ref = time.Now().UTC()
	}

	repository, err := os.OpenRoot(opts.RepositoryDir)
	if err != nil {
		return nil, fmt.Errorf("open plugin registry audit repository: %w", err)
	}
	defer repository.Close()
	bootstrap, trustedRootInfo, err := readAuditTrustedRoot(opts.TrustedRootPath, 512000)
	if err != nil {
		return nil, fmt.Errorf("read plugin registry audit trusted root: %w", err)
	}
	if err := rejectTrustedRootFileAlias(repository, trustedRootInfo); err != nil {
		return nil, err
	}
	if err := rejectDuplicateJSONKeys(bootstrap); err != nil {
		return nil, fmt.Errorf("decode plugin registry audit trusted root: %w", err)
	}
	bootstrapRoot, err := metadata.Root().FromBytes(bootstrap)
	if err != nil {
		return nil, fmt.Errorf("decode plugin registry audit trusted root: %w", err)
	}
	trusted, err := trustedmetadata.New(bootstrap)
	if err != nil {
		return nil, fmt.Errorf("verify plugin registry audit trusted root: %w", err)
	}
	trusted.RefTime = ref
	rootVersions := []int64{bootstrapRoot.Signed.Version}
	finalRootBody := bootstrap
	availableRoots, err := numberedRootVersions(repository)
	if err != nil {
		return nil, err
	}
	expected := bootstrapRoot.Signed.Version + 1
	rotations := 0
	for _, version := range availableRoots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if version < bootstrapRoot.Signed.Version {
			continue
		}
		if version == bootstrapRoot.Signed.Version {
			published, err := readAuditFile(repository, fmt.Sprintf("metadata/%d.root.json", version), 512000)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(published, bootstrap) {
				return nil, fmt.Errorf("plugin registry published root %d does not match the out-of-band bootstrap bytes", version)
			}
			continue
		}
		if version != expected {
			return nil, fmt.Errorf("plugin registry root chain has a gap: expected version %d, found %d", expected, version)
		}
		rotations++
		if rotations > policy.MaxRootRotations {
			return nil, fmt.Errorf("plugin registry root chain exceeds %d rotations", policy.MaxRootRotations)
		}
		body, err := readAuditFile(repository, fmt.Sprintf("metadata/%d.root.json", version), 512000)
		if err != nil {
			return nil, err
		}
		if err := rejectDuplicateJSONKeys(body); err != nil {
			return nil, fmt.Errorf("decode plugin registry root %d: %w", version, err)
		}
		if _, err := trusted.UpdateRoot(body); err != nil {
			return nil, fmt.Errorf("verify plugin registry root %d: %w", version, err)
		}
		rootVersions = append(rootVersions, version)
		finalRootBody = body
		expected++
	}

	roles, err := auditRootPolicy(trusted.Root, policy)
	if err != nil {
		return nil, err
	}
	if err := auditExpiry("root", trusted.Root.Signed.Expires, ref, policy.MinRootRemaining, policy.MaxRootRemaining); err != nil {
		return nil, err
	}

	timestampBody, err := readAuditFile(repository, "metadata/timestamp.json", 16384)
	if err != nil {
		return nil, err
	}
	timestampPreview, err := parseAuditMetadata(timestampBody, metadata.Timestamp())
	if err != nil {
		return nil, fmt.Errorf("decode plugin registry timestamp: %w", err)
	}
	snapshotInfo := timestampPreview.Signed.Meta[metadata.SNAPSHOT+".json"]
	if snapshotInfo == nil || snapshotInfo.Version < 1 {
		return nil, fmt.Errorf("plugin registry timestamp is missing valid snapshot.json metadata")
	}
	timestamp, err := trusted.UpdateTimestamp(timestampBody)
	if err != nil {
		return nil, fmt.Errorf("verify plugin registry timestamp: %w", err)
	}
	if err := auditExpiry("timestamp", timestamp.Signed.Expires, ref, policy.MinTimestampRemaining, policy.MaxTimestampRemaining); err != nil {
		return nil, err
	}

	snapshotBody, err := readAuditFile(repository, fmt.Sprintf("metadata/%d.snapshot.json", snapshotInfo.Version), 1<<20)
	if err != nil {
		return nil, err
	}
	snapshotPreview, err := parseAuditMetadata(snapshotBody, metadata.Snapshot())
	if err != nil {
		return nil, fmt.Errorf("decode plugin registry snapshot: %w", err)
	}
	targetsInfo := snapshotPreview.Signed.Meta[metadata.TARGETS+".json"]
	if targetsInfo == nil || targetsInfo.Version < 1 {
		return nil, fmt.Errorf("plugin registry snapshot is missing valid targets.json metadata")
	}
	snapshot, err := trusted.UpdateSnapshot(snapshotBody, false)
	if err != nil {
		return nil, fmt.Errorf("verify plugin registry snapshot: %w", err)
	}
	if err := auditExpiry("snapshot", snapshot.Signed.Expires, ref, policy.MinSnapshotRemaining, policy.MaxSnapshotRemaining); err != nil {
		return nil, err
	}

	targetsBody, err := readAuditFile(repository, fmt.Sprintf("metadata/%d.targets.json", targetsInfo.Version), 2<<20)
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicateJSONKeys(targetsBody); err != nil {
		return nil, fmt.Errorf("decode plugin registry targets: %w", err)
	}
	targets, err := trusted.UpdateTargets(targetsBody)
	if err != nil {
		return nil, fmt.Errorf("verify plugin registry targets: %w", err)
	}
	if err := auditExpiry("targets", targets.Signed.Expires, ref, policy.MinTargetsRemaining, policy.MaxTargetsRemaining); err != nil {
		return nil, err
	}

	indexInfo := targets.Signed.Targets[opts.IndexTarget]
	if indexInfo == nil {
		return nil, fmt.Errorf("plugin registry targets does not authenticate %q", opts.IndexTarget)
	}
	indexBody, indexDigest, err := readAndVerifyAuditTarget(repository, opts.IndexTarget, indexInfo, maxIndexBytes)
	if err != nil {
		return nil, fmt.Errorf("verify plugin registry index target: %w", err)
	}
	index, err := decodeAndValidateIndex(indexBody)
	if err != nil {
		return nil, err
	}
	if index.Updated.After(ref.Add(5 * time.Minute)) {
		return nil, fmt.Errorf("plugin registry index updated timestamp is in the future")
	}
	if ref.Sub(index.Updated) > policy.MaxIndexAge {
		return nil, fmt.Errorf("plugin registry index is older than %s", policy.MaxIndexAge)
	}

	attestationNames := make(map[string]bool)
	for _, entry := range index.Plugins {
		if entry.Provenance.AttestationTarget != "" {
			attestationNames[entry.Provenance.AttestationTarget] = true
		}
	}
	names := make([]string, 0, len(attestationNames))
	for name := range attestationNames {
		names = append(names, name)
	}
	sort.Strings(names)
	attestations := make([]AuditTarget, 0, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info := targets.Signed.Targets[name]
		if info == nil {
			return nil, fmt.Errorf("plugin registry attestation target %q is not authenticated by targets metadata", name)
		}
		body, digest, err := readAndVerifyAuditTarget(repository, name, info, maxAttestationBytes)
		if err != nil {
			return nil, fmt.Errorf("verify plugin registry attestation target %q: %w", name, err)
		}
		attestations = append(attestations, AuditTarget{Path: name, SHA256: digest, Length: int64(len(body))})
	}

	bootstrapDigest := sha256.Sum256(bootstrap)
	report := &AuditReport{
		SchemaVersion:       1,
		Policy:              policy.Name,
		ReferenceTime:       ref,
		BootstrapRootSHA256: "sha256:" + hex.EncodeToString(bootstrapDigest[:]),
		RootVersions:        rootVersions,
		Roles:               roles,
		Metadata: []AuditMetadata{
			metadataAudit("root", trusted.Root.Signed.Version, trusted.Root.Signed.Expires, trusted.Root.Signatures, finalRootBody),
			metadataAudit("targets", targets.Signed.Version, targets.Signed.Expires, targets.Signatures, targetsBody),
			metadataAudit("snapshot", snapshot.Signed.Version, snapshot.Signed.Expires, snapshot.Signatures, snapshotBody),
			metadataAudit("timestamp", timestamp.Signed.Version, timestamp.Signed.Expires, timestamp.Signatures, timestampBody),
		},
		Index:        AuditIndex{Target: opts.IndexTarget, SHA256: indexDigest, Registry: index.Registry, Updated: index.Updated, PluginCount: len(index.Plugins)},
		Attestations: attestations,
		Checks: []string{
			"bootstrap root self-signature and every sequential root rotation verified",
			"final root roles use independent canonical keys and production thresholds",
			"timestamp, snapshot, targets, index, and referenced attestation bytes verified",
			"metadata expiry windows and signed registry index freshness satisfy policy",
		},
		ExternalRequired: []string{
			"independently witnessed offline root/targets key ceremony and personnel quorum",
			"production HSM or equivalent private-key custody evidence",
			"atomic public HTTPS publication, freshness monitoring, and alert evidence",
			"documented live key-rotation and compromise-response exercise",
			"independent DSSE/SLSA identity and predicate verification when such claims are made",
		},
	}
	return report, nil
}

func readAuditTrustedRoot(name string, limit int64) ([]byte, os.FileInfo, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if unsafe, reason := unsafePathEntry(name, before); unsafe {
		return nil, nil, fmt.Errorf("%s is unsafe: %s", name, reason)
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > limit {
		return nil, nil, fmt.Errorf("%s is not a regular file between 1 and %d bytes", name, limit)
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || opened.Size() <= 0 || opened.Size() > limit || !os.SameFile(before, opened) {
		return nil, nil, fmt.Errorf("%s changed while opening the trusted root", name)
	}
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(body)) > limit {
		return nil, nil, fmt.Errorf("%s exceeds %d bytes", name, limit)
	}
	return body, opened, nil
}

func rejectTrustedRootFileAlias(repository *os.Root, trustedRoot os.FileInfo) error {
	dir, err := repository.Open("metadata")
	if err != nil {
		return fmt.Errorf("open plugin registry metadata directory: %w", err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("list plugin registry metadata directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := path.Join("metadata", entry.Name())
		info, err := repository.Stat(name)
		if err != nil {
			return fmt.Errorf("inspect plugin registry metadata file %q: %w", entry.Name(), err)
		}
		if os.SameFile(trustedRoot, info) {
			return fmt.Errorf("plugin registry trusted root must not be the same file as repository metadata %q", entry.Name())
		}
	}
	return nil
}

func requireOutOfBandRootPath(repositoryDir, trustedRootPath string) error {
	repositoryPath, err := filepath.EvalSymlinks(repositoryDir)
	if err != nil {
		return fmt.Errorf("resolve plugin registry audit repository: %w", err)
	}
	repositoryPath, err = filepath.Abs(repositoryPath)
	if err != nil {
		return fmt.Errorf("resolve plugin registry audit repository: %w", err)
	}
	rootPath, err := filepath.EvalSymlinks(trustedRootPath)
	if err != nil {
		return fmt.Errorf("resolve plugin registry audit trusted root: %w", err)
	}
	rootPath, err = filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("resolve plugin registry audit trusted root: %w", err)
	}
	relative, err := filepath.Rel(repositoryPath, rootPath)
	if err != nil {
		if !strings.EqualFold(filepath.VolumeName(repositoryPath), filepath.VolumeName(rootPath)) {
			// Paths on different Windows volumes cannot be relative and are
			// necessarily outside one another.
			return nil
		}
		return fmt.Errorf("compare plugin registry audit repository and trusted root paths: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return fmt.Errorf("plugin registry audit trusted root must be outside the repository directory")
	}
	return nil
}

func normalizeAuditPolicy(policy AuditPolicy) AuditPolicy {
	defaults := ProductionAuditPolicy()
	policy.Name = strings.TrimSpace(policy.Name)
	if policy.Name == "" {
		policy.Name = defaults.Name
	}
	if policy.RootThreshold == 0 {
		policy.RootThreshold = defaults.RootThreshold
	}
	if policy.TargetsThreshold == 0 {
		policy.TargetsThreshold = defaults.TargetsThreshold
	}
	if policy.SnapshotThreshold == 0 {
		policy.SnapshotThreshold = defaults.SnapshotThreshold
	}
	if policy.TimestampThreshold == 0 {
		policy.TimestampThreshold = defaults.TimestampThreshold
	}
	if policy.MaxRootRotations == 0 {
		policy.MaxRootRotations = defaults.MaxRootRotations
	}
	if policy.MinRootRemaining == 0 {
		policy.MinRootRemaining = defaults.MinRootRemaining
	}
	if policy.MinTargetsRemaining == 0 {
		policy.MinTargetsRemaining = defaults.MinTargetsRemaining
	}
	if policy.MinSnapshotRemaining == 0 {
		policy.MinSnapshotRemaining = defaults.MinSnapshotRemaining
	}
	if policy.MinTimestampRemaining == 0 {
		policy.MinTimestampRemaining = defaults.MinTimestampRemaining
	}
	if policy.MaxRootRemaining == 0 {
		policy.MaxRootRemaining = defaults.MaxRootRemaining
	}
	if policy.MaxTargetsRemaining == 0 {
		policy.MaxTargetsRemaining = defaults.MaxTargetsRemaining
	}
	if policy.MaxSnapshotRemaining == 0 {
		policy.MaxSnapshotRemaining = defaults.MaxSnapshotRemaining
	}
	if policy.MaxTimestampRemaining == 0 {
		policy.MaxTimestampRemaining = defaults.MaxTimestampRemaining
	}
	if policy.MaxIndexAge == 0 {
		policy.MaxIndexAge = defaults.MaxIndexAge
	}
	return policy
}

func validateAuditPolicy(policy AuditPolicy) error {
	if err := validateDisplayText("plugin registry audit policy name", policy.Name, 128, true); err != nil {
		return err
	}
	for role, threshold := range map[string]int{
		metadata.ROOT: policy.RootThreshold, metadata.TARGETS: policy.TargetsThreshold,
		metadata.SNAPSHOT: policy.SnapshotThreshold, metadata.TIMESTAMP: policy.TimestampThreshold,
	} {
		if threshold < 1 {
			return fmt.Errorf("plugin registry audit %s threshold must be positive", role)
		}
	}
	if policy.MaxRootRotations < 1 || policy.MaxRootRotations > 32 {
		return fmt.Errorf("plugin registry audit maximum root rotations must be between 1 and 32")
	}
	for role, window := range map[string]struct{ minimum, maximum time.Duration }{
		metadata.ROOT:      {policy.MinRootRemaining, policy.MaxRootRemaining},
		metadata.TARGETS:   {policy.MinTargetsRemaining, policy.MaxTargetsRemaining},
		metadata.SNAPSHOT:  {policy.MinSnapshotRemaining, policy.MaxSnapshotRemaining},
		metadata.TIMESTAMP: {policy.MinTimestampRemaining, policy.MaxTimestampRemaining},
	} {
		if window.minimum < 0 || window.maximum <= window.minimum {
			return fmt.Errorf("plugin registry audit %s expiry window is invalid", role)
		}
	}
	if policy.MaxIndexAge <= 0 {
		return fmt.Errorf("plugin registry audit maximum index age must be positive")
	}
	return nil
}

func auditRootPolicy(root *metadata.Metadata[metadata.RootType], policy AuditPolicy) ([]AuditRole, error) {
	if root == nil || !root.Signed.ConsistentSnapshot {
		return nil, fmt.Errorf("plugin registry root must enable consistent snapshots")
	}
	minimums := map[string]int{
		metadata.ROOT: policy.RootThreshold, metadata.TARGETS: policy.TargetsThreshold,
		metadata.SNAPSHOT: policy.SnapshotThreshold, metadata.TIMESTAMP: policy.TimestampThreshold,
	}
	owner := make(map[string]string)
	roles := make([]AuditRole, 0, len(metadata.TOP_LEVEL_ROLE_NAMES))
	for _, name := range []string{metadata.ROOT, metadata.TARGETS, metadata.SNAPSHOT, metadata.TIMESTAMP} {
		role := root.Signed.Roles[name]
		if role == nil {
			return nil, fmt.Errorf("plugin registry root is missing %s role", name)
		}
		if role.Threshold < minimums[name] {
			return nil, fmt.Errorf("plugin registry %s threshold %d is below production minimum %d", name, role.Threshold, minimums[name])
		}
		seen := make(map[string]bool)
		for _, keyID := range role.KeyIDs {
			if seen[keyID] {
				return nil, fmt.Errorf("plugin registry %s role repeats key %s", name, keyID)
			}
			seen[keyID] = true
			key := root.Signed.Keys[keyID]
			if key == nil {
				return nil, fmt.Errorf("plugin registry %s role references missing key %s", name, keyID)
			}
			canonical, err := key.ID()
			if err != nil {
				return nil, fmt.Errorf("compute plugin registry %s key ID: %w", name, err)
			}
			if canonical != keyID {
				return nil, fmt.Errorf("plugin registry %s key ID does not match canonical public key", name)
			}
			if previous, ok := owner[keyID]; ok && previous != name {
				return nil, fmt.Errorf("plugin registry roles %s and %s reuse key %s", previous, name, keyID)
			}
			owner[keyID] = name
		}
		if len(seen) < role.Threshold {
			return nil, fmt.Errorf("plugin registry %s role has %d keys for threshold %d", name, len(seen), role.Threshold)
		}
		keyIDs := append([]string(nil), role.KeyIDs...)
		sort.Strings(keyIDs)
		roles = append(roles, AuditRole{Name: name, Threshold: role.Threshold, KeyCount: len(seen), KeyIDs: keyIDs})
	}
	return roles, nil
}

func auditExpiry(role string, expires, ref time.Time, minimum, maximum time.Duration) error {
	remaining := expires.Sub(ref)
	if remaining < minimum {
		return fmt.Errorf("plugin registry %s expires too soon: %s remaining, require at least %s", role, remaining.Round(time.Second), minimum)
	}
	if remaining > maximum {
		return fmt.Errorf("plugin registry %s expiry is too far in the future: %s remaining, maximum %s", role, remaining.Round(time.Second), maximum)
	}
	return nil
}

func metadataAudit(role string, version int64, expires time.Time, signatures []metadata.Signature, body []byte) AuditMetadata {
	seen := make(map[string]bool)
	for _, signature := range signatures {
		seen[signature.KeyID] = true
	}
	digest := sha256.Sum256(body)
	return AuditMetadata{Role: role, Version: version, Expires: expires.UTC(), SignatureCount: len(seen), SHA256: "sha256:" + hex.EncodeToString(digest[:])}
}

func numberedRootVersions(repository *os.Root) ([]int64, error) {
	dir, err := repository.Open("metadata")
	if err != nil {
		return nil, fmt.Errorf("open plugin registry metadata directory: %w", err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("list plugin registry metadata directory: %w", err)
	}
	versions := make([]int64, 0)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".root.json") {
			continue
		}
		prefix := strings.TrimSuffix(name, ".root.json")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || version < 1 || prefix != strconv.FormatInt(version, 10) {
			return nil, fmt.Errorf("plugin registry metadata contains invalid numbered root %q", name)
		}
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("plugin registry numbered root %q is not a regular file", name)
		}
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions, nil
}

func parseAuditMetadata[T metadata.Roles](body []byte, document *metadata.Metadata[T]) (*metadata.Metadata[T], error) {
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return nil, err
	}
	return document.FromBytes(body)
}

func readAndVerifyAuditTarget(repository *os.Root, target string, info *metadata.TargetFiles, limit int64) ([]byte, string, error) {
	if err := validateTargetPath(target); err != nil {
		return nil, "", err
	}
	if info.Length <= 0 || info.Length > limit {
		return nil, "", fmt.Errorf("target length %d is outside 1..%d bytes", info.Length, limit)
	}
	hashBytes, ok := info.Hashes["sha256"]
	if !ok || len(hashBytes) != sha256.Size {
		return nil, "", fmt.Errorf("target is missing a canonical SHA-256 hash")
	}
	digest := hex.EncodeToString(hashBytes)
	dir, base := path.Split(target)
	relative := path.Join("targets", dir, digest+"."+base)
	body, err := readAuditFile(repository, relative, limit)
	if err != nil {
		return nil, "", err
	}
	if err := info.VerifyLengthHashes(body); err != nil {
		return nil, "", err
	}
	return body, "sha256:" + digest, nil
}

func readAuditFile(repository *os.Root, name string, limit int64) ([]byte, error) {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("plugin registry audit path %q is not a clean relative path", name)
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		info, err := repository.Lstat(prefix)
		if err != nil {
			return nil, fmt.Errorf("inspect plugin registry audit file %q: %w", name, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("plugin registry audit file %q crosses a symbolic link", name)
		}
	}
	file, err := repository.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open plugin registry audit file %q: %w", name, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat plugin registry audit file %q: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("plugin registry audit file %q is not a regular file between 1 and %d bytes", name, limit)
	}
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read plugin registry audit file %q: %w", name, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("plugin registry audit file %q exceeds %d bytes", name, limit)
	}
	return body, nil
}
