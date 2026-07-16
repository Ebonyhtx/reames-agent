package pluginregistry

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

type auditKeys struct {
	root      []ed25519.PrivateKey
	targets   []ed25519.PrivateKey
	snapshot  ed25519.PrivateKey
	timestamp ed25519.PrivateKey
}

func TestAuditRepositoryProductionPolicyAndSequentialRotation(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	oldKeys := newAuditKeys(t)
	newKeys := newAuditKeys(t)
	rootV1 := makeAuditRoot(t, oldKeys, 1, ref.Add(365*24*time.Hour), nil, oldKeys.root[:2])
	rootV2 := makeAuditRoot(t, newKeys, 2, ref.Add(365*24*time.Hour), oldKeys.root[:2], newKeys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, newKeys, rootV1, map[int64][]byte{1: rootV1, 2: rootV2}, ref)

	report, err := AuditRepository(context.Background(), AuditOptions{
		RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref,
	})
	if err != nil {
		t.Fatalf("AuditRepository: %v", err)
	}
	if report.Policy != productionAuditPolicyName || report.BootstrapRootSHA256 == "" {
		t.Fatalf("report identity = %+v", report)
	}
	if got := report.RootVersions; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("root versions = %v, want [1 2]", got)
	}
	if len(report.Roles) != 4 || report.Roles[0].Name != metadata.ROOT || report.Roles[0].Threshold != 2 || report.Roles[1].Name != metadata.TARGETS || report.Roles[1].Threshold != 2 {
		t.Fatalf("roles = %+v", report.Roles)
	}
	if len(report.Roles[0].KeyIDs) != 3 || len(report.Metadata) != 4 || report.Metadata[0].SHA256 == "" {
		t.Fatalf("ceremony identifiers missing: roles=%+v metadata=%+v", report.Roles, report.Metadata)
	}
	if report.Index.Registry != "test-registry" || report.Index.PluginCount != 1 || report.Index.SHA256 == "" {
		t.Fatalf("index = %+v", report.Index)
	}
	if len(report.Attestations) != 1 || report.Attestations[0].Path != "attestations/demo.dsse.json" {
		t.Fatalf("attestations = %+v", report.Attestations)
	}
	if len(report.ExternalRequired) == 0 {
		t.Fatal("successful local audit omitted external evidence boundary")
	}
}

func TestAuditRepositoryRejectsRootRotationWithoutOldQuorum(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	oldKeys := newAuditKeys(t)
	newKeys := newAuditKeys(t)
	rootV1 := makeAuditRoot(t, oldKeys, 1, ref.Add(365*24*time.Hour), nil, oldKeys.root[:2])
	rootV2 := makeAuditRoot(t, newKeys, 2, ref.Add(365*24*time.Hour), oldKeys.root[:1], newKeys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, newKeys, rootV1, map[int64][]byte{1: rootV1, 2: rootV2}, ref)

	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "signature") {
		t.Fatalf("rotation without old quorum err = %v, want signature failure", err)
	}
}

func TestAuditRepositoryRejectsRootGap(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	rootV1 := makeAuditRoot(t, keys, 1, ref.Add(365*24*time.Hour), nil, keys.root[:2])
	rootV3 := makeAuditRoot(t, keys, 3, ref.Add(365*24*time.Hour), keys.root[:2], keys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, keys, rootV1, map[int64][]byte{1: rootV1, 3: rootV3}, ref)

	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
	if err == nil || !strings.Contains(err.Error(), "root chain has a gap") {
		t.Fatalf("root gap err = %v", err)
	}
}

func TestAuditRepositoryRejectsNonCanonicalNumberedRootName(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	root := makeAuditRoot(t, keys, 1, ref.Add(365*24*time.Hour), nil, keys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, keys, root, map[int64][]byte{1: root}, ref)
	if err := os.WriteFile(filepath.Join(repository, "metadata", "01.root.json"), root, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
	if err == nil || !strings.Contains(err.Error(), "invalid numbered root") {
		t.Fatalf("non-canonical root name err = %v", err)
	}
}

func TestAuditRepositoryRejectsPublishedBootstrapByteMismatch(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	trustedRoot := makeAuditRoot(t, keys, 1, ref.Add(365*24*time.Hour), nil, keys.root[:2])
	publishedRoot := append([]byte(" \n"), trustedRoot...)
	repository, trustedRootPath := writeAuditRepository(t, keys, trustedRoot, map[int64][]byte{1: publishedRoot}, ref)
	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRootPath, ReferenceTime: ref})
	if err == nil || !strings.Contains(err.Error(), "does not match the out-of-band bootstrap bytes") {
		t.Fatalf("published bootstrap mismatch err = %v", err)
	}
}

func TestAuditRepositoryRejectsBootstrapPathInsideRepository(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	root := makeAuditRoot(t, keys, 1, ref.Add(365*24*time.Hour), nil, keys.root[:2])
	repository, _ := writeAuditRepository(t, keys, root, map[int64][]byte{1: root}, ref)
	inside := filepath.Join(repository, "metadata", "1.root.json")
	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: inside, ReferenceTime: ref})
	if err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("repository-local bootstrap err = %v", err)
	}
}

func TestAuditRepositoryRejectsBootstrapHardlinkToPublishedRoot(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	root := makeAuditRoot(t, keys, 1, ref.Add(365*24*time.Hour), nil, keys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, keys, root, map[int64][]byte{1: root}, ref)
	linked := filepath.Join(repository, "metadata", "root.json")
	if err := os.Link(trustedRoot, linked); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
	if err == nil || !strings.Contains(err.Error(), "must not be the same file") {
		t.Fatalf("hard-linked bootstrap err = %v", err)
	}
}

func TestAuditRepositoryRejectsProductionPolicyViolations(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		mutate  func(*testing.T, auditKeys, *metadata.Metadata[metadata.RootType])
		wantErr string
	}{
		{
			name: "root threshold",
			mutate: func(_ *testing.T, _ auditKeys, root *metadata.Metadata[metadata.RootType]) {
				root.Signed.Roles[metadata.ROOT].Threshold = 1
			},
			wantErr: "root threshold",
		},
		{
			name: "role key reuse",
			mutate: func(t *testing.T, keys auditKeys, root *metadata.Metadata[metadata.RootType]) {
				shared, err := metadata.KeyFromPublicKey(keys.root[0].Public())
				if err != nil {
					t.Fatal(err)
				}
				if err := root.Signed.AddKey(shared, metadata.SNAPSHOT); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "reuse key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := newAuditKeys(t)
			root := makeAuditRootDocument(t, keys, 1, ref.Add(365*24*time.Hour))
			tt.mutate(t, keys, root)
			for _, signer := range keys.root[:2] {
				signTestMetadata(t, root, signer)
			}
			rootBytes := metadataBytes(t, root)
			repository, trustedRoot := writeAuditRepository(t, keys, rootBytes, map[int64][]byte{1: rootBytes}, ref)
			_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("policy violation err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestAuditRepositoryRejectsTamperedIndexAndUnsafeTargetPath(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	root := makeAuditRoot(t, keys, 1, ref.Add(365*24*time.Hour), nil, keys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, keys, root, map[int64][]byte{1: root}, ref)
	indexFile := findAuditTarget(t, repository, ".plugins.json")
	if err := os.WriteFile(indexFile, []byte(`{"schemaVersion":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hash") {
		t.Fatalf("tampered index err = %v, want hash failure", err)
	}

	_, err = AuditRepository(context.Background(), AuditOptions{
		RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref, IndexTarget: "../plugins.json",
	})
	if err == nil || !strings.Contains(err.Error(), "clean relative") {
		t.Fatalf("unsafe index path err = %v", err)
	}
}

func TestAuditRepositoryRejectsSymlinkedAuthenticatedTarget(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	root := makeAuditRoot(t, keys, 1, ref.Add(365*24*time.Hour), nil, keys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, keys, root, map[int64][]byte{1: root}, ref)
	indexFile := findAuditTarget(t, repository, ".plugins.json")
	body, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "plugins.json")
	if err := os.WriteFile(outside, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(indexFile); err != nil {
		t.Fatal(err)
	}
	createTestSymlinkOrSkip(t, outside, indexFile)
	_, err = AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symbolic") {
		t.Fatalf("symlinked index err = %v", err)
	}
}

func TestAuditRepositoryHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AuditRepository(ctx, AuditOptions{RepositoryDir: t.TempDir(), TrustedRootPath: "root.json"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled audit err = %v, want context.Canceled", err)
	}
}

func TestAuditRepositoryRejectsExpiryOutsideProductionWindow(t *testing.T) {
	ref := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	keys := newAuditKeys(t)
	root := makeAuditRoot(t, keys, 1, ref.Add(500*24*time.Hour), nil, keys.root[:2])
	repository, trustedRoot := writeAuditRepository(t, keys, root, map[int64][]byte{1: root}, ref)
	_, err := AuditRepository(context.Background(), AuditOptions{RepositoryDir: repository, TrustedRootPath: trustedRoot, ReferenceTime: ref})
	if err == nil || !strings.Contains(err.Error(), "too far in the future") {
		t.Fatalf("long-lived root err = %v", err)
	}
}

func newAuditKeys(t *testing.T) auditKeys {
	t.Helper()
	generate := func() ed25519.PrivateKey {
		_, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return private
	}
	keys := auditKeys{snapshot: generate(), timestamp: generate()}
	for range 3 {
		keys.root = append(keys.root, generate())
		keys.targets = append(keys.targets, generate())
	}
	return keys
}

func makeAuditRoot(t *testing.T, keys auditKeys, version int64, expires time.Time, oldSigners, newSigners []ed25519.PrivateKey) []byte {
	t.Helper()
	root := makeAuditRootDocument(t, keys, version, expires)
	for _, signer := range append(append([]ed25519.PrivateKey(nil), oldSigners...), newSigners...) {
		signTestMetadata(t, root, signer)
	}
	return metadataBytes(t, root)
}

func makeAuditRootDocument(t *testing.T, keys auditKeys, version int64, expires time.Time) *metadata.Metadata[metadata.RootType] {
	t.Helper()
	root := metadata.Root(expires.UTC())
	root.Signed.Version = version
	root.Signed.ConsistentSnapshot = true
	add := func(role string, private ed25519.PrivateKey) {
		key, err := metadata.KeyFromPublicKey(private.Public())
		if err != nil {
			t.Fatal(err)
		}
		if err := root.Signed.AddKey(key, role); err != nil {
			t.Fatal(err)
		}
	}
	for _, private := range keys.root {
		add(metadata.ROOT, private)
	}
	for _, private := range keys.targets {
		add(metadata.TARGETS, private)
	}
	add(metadata.SNAPSHOT, keys.snapshot)
	add(metadata.TIMESTAMP, keys.timestamp)
	root.Signed.Roles[metadata.ROOT].Threshold = 2
	root.Signed.Roles[metadata.TARGETS].Threshold = 2
	return root
}

func writeAuditRepository(t *testing.T, keys auditKeys, trustedRoot []byte, roots map[int64][]byte, ref time.Time) (string, string) {
	t.Helper()
	repository := t.TempDir()
	metadataDir := filepath.Join(repository, "metadata")
	targetsDir := filepath.Join(repository, "targets")
	if err := os.MkdirAll(filepath.Join(targetsDir, "attestations"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(metadataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	trustedRootPath := filepath.Join(t.TempDir(), "bootstrap-root.json")
	if err := os.WriteFile(trustedRootPath, trustedRoot, 0o600); err != nil {
		t.Fatal(err)
	}
	for version, body := range roots {
		if err := os.WriteFile(filepath.Join(metadataDir, strconv.FormatInt(version, 10)+".root.json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	index := testIndex(testRegistryEntry())
	index.Updated = ref
	indexBytes, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	attestation := []byte(`{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`)
	targets := metadata.Targets(ref.Add(30 * 24 * time.Hour))
	indexInfo, err := metadata.TargetFile().FromBytes(DefaultIndexTarget, indexBytes, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	attestationInfo, err := metadata.TargetFile().FromBytes("attestations/demo.dsse.json", attestation, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	targets.Signed.Targets[DefaultIndexTarget] = indexInfo
	targets.Signed.Targets["attestations/demo.dsse.json"] = attestationInfo
	for _, signer := range keys.targets[:2] {
		signTestMetadata(t, targets, signer)
	}
	targetsBytes := metadataBytes(t, targets)

	snapshot := metadata.Snapshot(ref.Add(7 * 24 * time.Hour))
	snapshot.Signed.Meta["targets.json"] = testMetaFile(1, targetsBytes)
	signTestMetadata(t, snapshot, keys.snapshot)
	snapshotBytes := metadataBytes(t, snapshot)
	timestamp := metadata.Timestamp(ref.Add(24 * time.Hour))
	timestamp.Signed.Meta["snapshot.json"] = testMetaFile(1, snapshotBytes)
	signTestMetadata(t, timestamp, keys.timestamp)

	write := func(name string, body []byte) {
		if err := os.WriteFile(name, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(metadataDir, "timestamp.json"), metadataBytes(t, timestamp))
	write(filepath.Join(metadataDir, "1.snapshot.json"), snapshotBytes)
	write(filepath.Join(metadataDir, "1.targets.json"), targetsBytes)
	write(filepath.Join(targetsDir, targetHash(indexInfo)+".plugins.json"), indexBytes)
	write(filepath.Join(targetsDir, "attestations", targetHash(attestationInfo)+".demo.dsse.json"), attestation)
	return repository, trustedRootPath
}

func findAuditTarget(t *testing.T, repository, suffix string) string {
	t.Helper()
	var found string
	err := filepath.WalkDir(filepath.Join(repository, "targets"), func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			found = name
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatalf("find target suffix %q: path=%q err=%v", suffix, found, err)
	}
	return found
}
