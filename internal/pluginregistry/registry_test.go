package pluginregistry

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func TestClientRefreshResolveAndVerifyAttestation(t *testing.T) {
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	entry := testRegistryEntry()
	index := testIndex(entry)
	files := makeTestRepository(t, keys, 1, index, []byte(`{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`), time.Now().Add(time.Hour))
	server := newMutableRegistryServer(files)
	defer server.Close()

	client := newTestClient(t, server.URL, root)
	gotIndex, err := client.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(gotIndex.Plugins) != 1 || gotIndex.Plugins[0].Name != entry.Name {
		t.Fatalf("index plugins = %+v", gotIndex.Plugins)
	}
	resolved, err := client.Resolve(context.Background(), entry.Name)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Revision != entry.Revision || resolved.Digest != entry.Digest || resolved.ReleaseEvidenceSHA256 == "" || resolved.AttestationSHA256 == "" {
		t.Fatalf("resolved entry = %+v", resolved)
	}
	if resolved.ProvenanceStatus != "tuf-attestation-target-integrity-verified" {
		t.Fatalf("resolved provenance status = %q", resolved.ProvenanceStatus)
	}
	_ = findCachedRoot(t, client.opts.CacheBaseDir)
}

func TestClientRejectsTamperedAuthenticatedIndex(t *testing.T) {
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	files := makeTestRepository(t, keys, 1, testIndex(testRegistryEntry()), nil, time.Now().Add(time.Hour))
	for name := range files {
		if strings.HasPrefix(name, "/targets/") && strings.HasSuffix(name, ".plugins.json") {
			files[name] = append([]byte(nil), []byte(`{"schemaVersion":1,"registry":"tampered"}`)...)
		}
	}
	server := newMutableRegistryServer(files)
	defer server.Close()

	_, err := newTestClient(t, server.URL, root).Refresh(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hash") {
		t.Fatalf("Refresh tampered index err = %v, want hash failure", err)
	}
}

func TestClientPersistentMetadataRejectsRollback(t *testing.T) {
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	newFiles := makeTestRepository(t, keys, 2, testIndex(testRegistryEntry()), nil, time.Now().Add(time.Hour))
	oldFiles := makeTestRepository(t, keys, 1, testIndex(testRegistryEntry()), nil, time.Now().Add(time.Hour))
	server := newMutableRegistryServer(newFiles)
	defer server.Close()

	client := newTestClient(t, server.URL, root)
	if _, err := client.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	server.Replace(oldFiles)
	if _, err := client.Refresh(context.Background()); err == nil || !strings.Contains(strings.ToLower(err.Error()), "version") {
		t.Fatalf("rollback Refresh err = %v, want version rollback failure", err)
	}
}

func TestClientAcceptsSequentialRootRotationAndPersistsIt(t *testing.T) {
	keys := newTestKeys(t)
	rootV1 := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	rootV2 := makeTestRoot(t, keys, 2, true, time.Now().Add(time.Hour))
	files := makeTestRepository(t, keys, 1, testIndex(testRegistryEntry()), nil, time.Now().Add(time.Hour))
	files["/metadata/2.root.json"] = rootV2
	server := newMutableRegistryServer(files)
	defer server.Close()

	client := newTestClient(t, server.URL, rootV1)
	if _, err := client.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh after root rotation: %v", err)
	}
	cachedRootPath := findCachedRoot(t, client.opts.CacheBaseDir)
	cachedRoot, err := os.ReadFile(cachedRootPath)
	if err != nil {
		t.Fatalf("read cached root: %v", err)
	}
	parsed, err := metadata.Root().FromBytes(cachedRoot)
	if err != nil {
		t.Fatalf("parse cached root: %v", err)
	}
	if parsed.Signed.Version != 2 {
		t.Fatalf("cached root version = %d, want 2", parsed.Signed.Version)
	}
	if _, err := client.Refresh(context.Background()); err != nil {
		t.Fatalf("second Refresh using persisted rotated root: %v", err)
	}
}

func TestClientRejectsExpiredTimestamp(t *testing.T) {
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	files := makeTestRepository(t, keys, 1, testIndex(testRegistryEntry()), nil, time.Now().Add(-time.Minute))
	server := newMutableRegistryServer(files)
	defer server.Close()

	_, err := newTestClient(t, server.URL, root).Refresh(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "expired") {
		t.Fatalf("Refresh expired metadata err = %v, want expired failure", err)
	}
}

func TestDecodeAndValidateIndexRejectsUnboundProvenanceAndUnknownFields(t *testing.T) {
	entry := testRegistryEntry()
	entry.Provenance.Revision = strings.Repeat("b", 40)
	body, _ := json.Marshal(testIndex(entry))
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("unbound provenance err = %v", err)
	}
	body = []byte(`{"schemaVersion":1,"registry":"test","updated":"2026-01-01T00:00:00Z","plugins":[],"unsignedPolicy":"accept"}`)
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field err = %v", err)
	}
}

func TestRegistryEntryDigestBindsCompleteNormalizedSignedEntry(t *testing.T) {
	base := testRegistryEntry()
	digest := func(t *testing.T, entry Entry) string {
		t.Helper()
		body, err := json.Marshal(testIndex(entry))
		if err != nil {
			t.Fatal(err)
		}
		index, err := decodeAndValidateIndex(body)
		if err != nil {
			t.Fatal(err)
		}
		got, err := registryEntryDigest(index.SchemaVersion, index.Registry, DefaultIndexTarget, index.Plugins[0])
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	baseDigest := digest(t, base)
	changedSource := base
	changedSource.Source = "https://github.com/example/other-demo"
	changedSource.Provenance.Source = changedSource.Source
	if got := digest(t, changedSource); got == baseDigest {
		t.Fatal("source change did not change release evidence digest")
	}

	changedBuilder := base
	changedBuilder.Provenance.BuilderID = "https://builder.example/other"
	if got := digest(t, changedBuilder); got == baseDigest {
		t.Fatal("builderId change did not change release evidence digest")
	}

	if got, err := registryEntryDigest(SchemaVersion, "other-registry", DefaultIndexTarget, base); err != nil || got == baseDigest {
		t.Fatalf("registry context digest = %q err=%v, want distinct digest", got, err)
	}
	if got, err := registryEntryDigest(SchemaVersion, "fixture-registry", "nested/plugins.json", base); err != nil || got == baseDigest {
		t.Fatalf("index target context digest = %q err=%v, want distinct digest", got, err)
	}
}

func TestDecodeAndValidateIndexRejectsDisplayControlCharacters(t *testing.T) {
	entry := testRegistryEntry()
	entry.Description = "trusted\x1b[31mspoofed"
	body, _ := json.Marshal(testIndex(entry))
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "control or formatting characters") {
		t.Fatalf("display control character err = %v", err)
	}

	index := testIndex(testRegistryEntry())
	index.Registry = "trusted\nforged"
	body, _ = json.Marshal(index)
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "control or formatting characters") {
		t.Fatalf("registry control character err = %v", err)
	}

	entry = testRegistryEntry()
	entry.Author = "trusted\u202eforged"
	body, _ = json.Marshal(testIndex(entry))
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "control or formatting characters") {
		t.Fatalf("display formatting character err = %v", err)
	}

	entry = testRegistryEntry()
	entry.Subpath = "plugin\u202e"
	entry.Provenance.Subpath = entry.Subpath
	body, _ = json.Marshal(testIndex(entry))
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "control or formatting characters") {
		t.Fatalf("subpath formatting character err = %v", err)
	}

	entry = testRegistryEntry()
	entry.Provenance.AttestationTarget = "attestations/bad\nname.json"
	body, _ = json.Marshal(testIndex(entry))
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "control or formatting characters") {
		t.Fatalf("attestation control character err = %v", err)
	}
}

func TestDecodeAndValidateIndexRejectsNonPortableSubpath(t *testing.T) {
	for _, subpath := range []string{"aux/plugin", "cafe\u0301/plugin"} {
		entry := testRegistryEntry()
		entry.Subpath = subpath
		entry.Provenance.Subpath = entry.Subpath
		body, _ := json.Marshal(testIndex(entry))
		if _, err := decodeAndValidateIndex(body); err == nil || (!strings.Contains(err.Error(), "portable") && !strings.Contains(err.Error(), "NFC")) {
			t.Fatalf("non-portable subpath %q err = %v", subpath, err)
		}
	}
}

func TestDecodeAndValidateIndexRejectsPortableNameAliases(t *testing.T) {
	upper := testRegistryEntry()
	upper.Name = "Foo"
	lower := testRegistryEntry()
	lower.Name = "foo"
	body, err := json.Marshal(Index{
		SchemaVersion: SchemaVersion,
		Registry:      "test-registry",
		Updated:       time.Now().UTC(),
		Plugins:       []Entry{upper, lower},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "cross-platform duplicate") {
		t.Fatalf("case-alias registry err = %v", err)
	}

	for _, name := range []string{"foo.", "CON", "aux.txt"} {
		entry := testRegistryEntry()
		entry.Name = name
		body, _ := json.Marshal(testIndex(entry))
		if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "invalid plugin name") {
			t.Fatalf("non-portable registry name %q err = %v", name, err)
		}
	}
}

func TestValidateGitHubRepositoryRequiresCanonicalURL(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/example/demo",
		"https://github.com/Example/demo-repo_1",
	} {
		if err := validateGitHubRepository(raw); err != nil {
			t.Errorf("validateGitHubRepository(%q): %v", raw, err)
		}
	}
	for _, raw := range []string{
		"https://github.com:443/example/demo",
		"https://github.com/example/demo.git",
		"https://github.com/%65xample/demo",
		"https://github.com/example/demo/",
		"https://github.com/example/demo;branch=main",
	} {
		if err := validateGitHubRepository(raw); err == nil {
			t.Errorf("validateGitHubRepository(%q) succeeded", raw)
		}
	}
}

func TestDecodeAndValidateIndexRejectsTrailingSlashRepository(t *testing.T) {
	entry := testRegistryEntry()
	entry.Source += "/"
	entry.Provenance.Source += "/"
	body, err := json.Marshal(testIndex(entry))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAndValidateIndex(body); err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatalf("trailing-slash repository err = %v, want canonical source failure", err)
	}
}

func TestNewRequiresHTTPSExceptLoopback(t *testing.T) {
	base := Options{TargetsURL: "https://registry.example/targets", TrustedRootPath: "root.json", CacheBaseDir: "cache"}
	for _, raw := range []string{"", "http://registry.example/metadata", "https://user:pass@registry.example/metadata?x=1"} {
		opts := base
		opts.MetadataURL = raw
		if _, err := New(opts); err == nil {
			t.Errorf("New MetadataURL=%q succeeded", raw)
		}
	}
	opts := base
	opts.MetadataURL = "http://127.0.0.1:8080/metadata"
	if _, err := New(opts); err != nil {
		t.Fatalf("loopback HTTP should be available to deterministic tests: %v", err)
	}
}

func TestClientGateWaitHonorsContextCancellation(t *testing.T) {
	client, err := New(Options{
		MetadataURL:     "https://registry.example/metadata",
		TargetsURL:      "https://registry.example/targets",
		TrustedRootPath: "unused-root.json",
		CacheBaseDir:    "unused-cache",
	})
	if err != nil {
		t.Fatal(err)
	}
	<-client.gate
	defer func() { client.gate <- struct{}{} }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := client.Refresh(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Refresh while client gate is held err = %v, want context deadline", err)
	}
}

func TestDistinctClientsFileLockWaitHonorsContextCancellation(t *testing.T) {
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	files := makeTestRepository(t, keys, 1, testIndex(testRegistryEntry()), nil, time.Now().Add(time.Hour))
	entered := make(chan struct{})
	release := make(chan struct{})
	var blockOnce sync.Once
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		blockOnce.Do(func() {
			close(entered)
			<-release
		})
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	dir := t.TempDir()
	rootPath := filepath.Join(dir, "root.json")
	if err := os.WriteFile(rootPath, root, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		MetadataURL: server.URL + "/metadata", TargetsURL: server.URL + "/targets",
		TrustedRootPath: rootPath, CacheBaseDir: filepath.Join(dir, "cache"), HTTPClient: server.Client(),
	}
	first, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	firstResult := make(chan error, 1)
	go func() {
		_, refreshErr := first.Refresh(context.Background())
		firstResult <- refreshErr
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first client did not reach registry while holding the cache lock")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if _, err := second.Refresh(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Refresh while file lock is held err = %v, want context deadline", err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-firstResult; err != nil {
		t.Fatalf("first Refresh after releasing server: %v", err)
	}
}

func TestClientRejectsSymlinkTrustedRoot(t *testing.T) {
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	dir := t.TempDir()
	realRoot := filepath.Join(dir, "real-root.json")
	linkedRoot := filepath.Join(dir, "root.json")
	if err := os.WriteFile(realRoot, root, 0o600); err != nil {
		t.Fatal(err)
	}
	createTestSymlinkOrSkip(t, realRoot, linkedRoot)
	client, err := New(Options{
		MetadataURL: "https://registry.example/metadata", TargetsURL: "https://registry.example/targets",
		TrustedRootPath: linkedRoot, CacheBaseDir: filepath.Join(dir, "cache"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertUnsafeRefresh(t, client)
}

func TestEnsurePrivateDirectoryAllowsSymlinkOutsideCacheBoundary(t *testing.T) {
	dir := t.TempDir()
	realParent := filepath.Join(dir, "real-parent")
	linkedParent := filepath.Join(dir, "linked-parent")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatal(err)
	}
	createTestSymlinkOrSkip(t, realParent, linkedParent)

	cacheBase := filepath.Join(linkedParent, "cache")
	got, err := ensurePrivateDirectory(cacheBase)
	if err != nil {
		t.Fatalf("ensure cache below symlinked external ancestor: %v", err)
	}
	if got != cacheBase {
		t.Fatalf("cache path = %q, want configured path %q", got, cacheBase)
	}
	if _, err := ensurePrivateSubdirectory(got, filepath.Join("namespace", "metadata")); err != nil {
		t.Fatalf("ensure cache-owned descendants: %v", err)
	}
}

func TestClientRejectsSymlinksInExistingCacheTree(t *testing.T) {
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	server := newMutableRegistryServer(makeTestRepository(t, keys, 1, testIndex(testRegistryEntry()), nil, time.Now().Add(time.Hour)))
	defer server.Close()

	for _, test := range []struct {
		name  string
		setup func(t *testing.T, base, namespace, outside string)
	}{
		{
			name: "cache base",
			setup: func(t *testing.T, base, _, outside string) {
				if err := os.MkdirAll(outside, 0o700); err != nil {
					t.Fatal(err)
				}
				createTestSymlinkOrSkip(t, outside, base)
			},
		},
		{
			name: "registry namespace",
			setup: func(t *testing.T, base, namespace, outside string) {
				if err := os.MkdirAll(base, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(outside, 0o700); err != nil {
					t.Fatal(err)
				}
				createTestSymlinkOrSkip(t, outside, namespace)
			},
		},
		{
			name: "metadata directory",
			setup: func(t *testing.T, _, namespace, outside string) {
				if err := os.MkdirAll(namespace, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(outside, 0o700); err != nil {
					t.Fatal(err)
				}
				createTestSymlinkOrSkip(t, outside, filepath.Join(namespace, "metadata"))
			},
		},
		{
			name: "targets directory",
			setup: func(t *testing.T, _, namespace, outside string) {
				if err := os.MkdirAll(namespace, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(outside, 0o700); err != nil {
					t.Fatal(err)
				}
				createTestSymlinkOrSkip(t, outside, filepath.Join(namespace, "targets"))
			},
		},
		{
			name: "cached root",
			setup: func(t *testing.T, _, namespace, outside string) {
				metadataDir := filepath.Join(namespace, "metadata")
				if err := os.MkdirAll(metadataDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(outside, []byte("not trusted"), 0o600); err != nil {
					t.Fatal(err)
				}
				createTestSymlinkOrSkip(t, outside, filepath.Join(metadataDir, "root.json"))
			},
		},
		{
			name: "index target destination",
			setup: func(t *testing.T, _, namespace, outside string) {
				targetsDir := filepath.Join(namespace, "targets")
				if err := os.MkdirAll(targetsDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(outside, []byte("not an index"), 0o600); err != nil {
					t.Fatal(err)
				}
				createTestSymlinkOrSkip(t, outside, filepath.Join(targetsDir, DefaultIndexTarget))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			rootPath := filepath.Join(dir, "root.json")
			if err := os.WriteFile(rootPath, root, 0o600); err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(dir, "cache")
			namespace := cacheNamespace(base, server.URL+"/metadata", root)
			test.setup(t, base, namespace, filepath.Join(dir, "outside"))
			client, err := New(Options{
				MetadataURL: server.URL + "/metadata", TargetsURL: server.URL + "/targets",
				TrustedRootPath: rootPath, CacheBaseDir: base, HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			assertUnsafeRefresh(t, client)
		})
	}
}

func TestClientMakesCacheTreeOwnerPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix owner permission bits")
	}
	keys := newTestKeys(t)
	root := makeTestRoot(t, keys, 1, false, time.Now().Add(time.Hour))
	server := newMutableRegistryServer(makeTestRepository(t, keys, 1, testIndex(testRegistryEntry()), nil, time.Now().Add(time.Hour)))
	defer server.Close()
	client := newTestClient(t, server.URL, root)
	if _, err := client.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := filepath.Walk(client.opts.CacheBaseDir, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			return fmt.Errorf("%s mode = %o, want %o", name, got, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func createTestSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
}

func assertUnsafeRefresh(t *testing.T, client *Client) {
	t.Helper()
	_, err := client.Refresh(context.Background())
	if err == nil {
		t.Fatal("operation succeeded through an unsafe cache path")
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "unsafe") && !strings.Contains(lower, "symbolic") && !strings.Contains(lower, "reparse") {
		t.Fatalf("unsafe path error = %v", err)
	}
}

type testKeys struct {
	rootOld   ed25519.PrivateKey
	rootNew   ed25519.PrivateKey
	targets   ed25519.PrivateKey
	snapshot  ed25519.PrivateKey
	timestamp ed25519.PrivateKey
}

func newTestKeys(t *testing.T) testKeys {
	t.Helper()
	generate := func() ed25519.PrivateKey {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	return testKeys{rootOld: generate(), rootNew: generate(), targets: generate(), snapshot: generate(), timestamp: generate()}
}

func makeTestRoot(t *testing.T, keys testKeys, version int64, rotated bool, expires time.Time) []byte {
	t.Helper()
	root := metadata.Root(expires.UTC())
	root.Signed.Version = version
	root.Signed.ConsistentSnapshot = true
	roles := map[string]ed25519.PrivateKey{"targets": keys.targets, "snapshot": keys.snapshot, "timestamp": keys.timestamp}
	for role, private := range roles {
		key, err := metadata.KeyFromPublicKey(private.Public())
		if err != nil {
			t.Fatal(err)
		}
		if err := root.Signed.AddKey(key, role); err != nil {
			t.Fatal(err)
		}
	}
	activeRoot := keys.rootOld
	if rotated {
		activeRoot = keys.rootNew
	}
	rootKey, err := metadata.KeyFromPublicKey(activeRoot.Public())
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Signed.AddKey(rootKey, "root"); err != nil {
		t.Fatal(err)
	}
	if rotated {
		signTestMetadata(t, root, keys.rootOld)
	}
	signTestMetadata(t, root, activeRoot)
	return metadataBytes(t, root)
}

func makeTestRepository(t *testing.T, keys testKeys, version int64, index Index, attestation []byte, expires time.Time) map[string][]byte {
	t.Helper()
	indexBytes, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	targets := metadata.Targets(expires.UTC())
	targets.Signed.Version = version
	indexInfo, err := metadata.TargetFile().FromBytes(DefaultIndexTarget, indexBytes, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	targets.Signed.Targets[DefaultIndexTarget] = indexInfo
	if len(attestation) > 0 {
		attestationInfo, err := metadata.TargetFile().FromBytes("attestations/demo.dsse.json", attestation, "sha256")
		if err != nil {
			t.Fatal(err)
		}
		targets.Signed.Targets["attestations/demo.dsse.json"] = attestationInfo
	}
	signTestMetadata(t, targets, keys.targets)
	targetsBytes := metadataBytes(t, targets)

	snapshot := metadata.Snapshot(expires.UTC())
	snapshot.Signed.Version = version
	snapshot.Signed.Meta["targets.json"] = testMetaFile(version, targetsBytes)
	signTestMetadata(t, snapshot, keys.snapshot)
	snapshotBytes := metadataBytes(t, snapshot)

	timestamp := metadata.Timestamp(expires.UTC())
	timestamp.Signed.Version = version
	timestamp.Signed.Meta["snapshot.json"] = testMetaFile(version, snapshotBytes)
	signTestMetadata(t, timestamp, keys.timestamp)
	timestampBytes := metadataBytes(t, timestamp)

	files := map[string][]byte{
		"/metadata/timestamp.json":                         timestampBytes,
		fmt.Sprintf("/metadata/%d.snapshot.json", version): snapshotBytes,
		fmt.Sprintf("/metadata/%d.targets.json", version):  targetsBytes,
	}
	files["/targets/"+targetHash(indexInfo)+"."+DefaultIndexTarget] = indexBytes
	if len(attestation) > 0 {
		info := targets.Signed.Targets["attestations/demo.dsse.json"]
		files["/targets/attestations/"+targetHash(info)+".demo.dsse.json"] = attestation
	}
	return files
}

func signTestMetadata[T metadata.Roles](t *testing.T, document *metadata.Metadata[T], private ed25519.PrivateKey) {
	t.Helper()
	signer, err := signature.LoadSigner(private, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.Sign(signer); err != nil {
		t.Fatal(err)
	}
}

func metadataBytes[T metadata.Roles](t *testing.T, document *metadata.Metadata[T]) []byte {
	t.Helper()
	body, err := document.ToBytes(false)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func testMetaFile(version int64, body []byte) *metadata.MetaFiles {
	sum := sha256.Sum256(body)
	return &metadata.MetaFiles{Version: version, Length: int64(len(body)), Hashes: metadata.Hashes{"sha256": metadata.HexBytes(sum[:])}}
}

func targetHash(info *metadata.TargetFiles) string {
	return hex.EncodeToString(info.Hashes["sha256"])
}

func testRegistryEntry() Entry {
	revision := strings.Repeat("a", 40)
	digest := GitTreeDigestPrefix + strings.Repeat("b", 64)
	return Entry{
		Name: "demo", Description: "Demo registry plugin", Version: "1.2.3", Author: "Reames",
		Source: "https://github.com/example/demo", Revision: revision, Digest: digest,
		Permissions: []string{"skills.load"}, Category: "development",
		Provenance: Provenance{
			Source: "https://github.com/example/demo", Revision: revision, Digest: digest,
			BuilderID: "https://registry.example/builders/release", AttestationTarget: "attestations/demo.dsse.json",
		},
	}
}

func testIndex(entry Entry) Index {
	if entry.Provenance.AttestationTarget != "" {
		return Index{SchemaVersion: SchemaVersion, Registry: "test-registry", Updated: time.Now().UTC(), Plugins: []Entry{entry}}
	}
	return Index{SchemaVersion: SchemaVersion, Registry: "test-registry", Updated: time.Now().UTC(), Plugins: []Entry{entry}}
}

type mutableRegistryServer struct {
	*httptest.Server
	mu    sync.RWMutex
	files map[string][]byte
}

func newMutableRegistryServer(files map[string][]byte) *mutableRegistryServer {
	server := &mutableRegistryServer{files: files}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.mu.RLock()
		body, ok := server.files[r.URL.Path]
		server.mu.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	return server
}

func (s *mutableRegistryServer) Replace(files map[string][]byte) {
	s.mu.Lock()
	s.files = files
	s.mu.Unlock()
}

func newTestClient(t *testing.T, serverURL string, root []byte) *Client {
	t.Helper()
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "root.json")
	if err := os.WriteFile(rootPath, root, 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := New(Options{
		MetadataURL: serverURL + "/metadata", TargetsURL: serverURL + "/targets",
		TrustedRootPath: rootPath, CacheBaseDir: filepath.Join(dir, "cache"), HTTPClient: serverClient(serverURL),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func serverClient(_ string) *http.Client { return &http.Client{Timeout: 5 * time.Second} }

func findCachedRoot(t *testing.T, base string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(base, "*", "metadata", "root.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("cached root matches = %v err=%v", matches, err)
	}
	return matches[0]
}
