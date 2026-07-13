package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"dev", "", false},
		{"", "", false},
		{"  ", "", false},
		{"abc", "", false},
		{"v1.2.3", "v1.2.3", true},
		{"1.2.3", "v1.2.3", true},
		{"v1.2.3-rc1", "v1.2.3-rc1", true},
		{"  v0.10.0  ", "v0.10.0", true},
	}
	for _, tt := range tests {
		got, ok := normalizeVersion(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("normalizeVersion(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	content := []byte("hello world")
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	t.Run("match", func(t *testing.T) {
		checksumFile := []byte(fmt.Sprintf("%s  reames-agent-linux-amd64.tar.gz\n", hash))
		if err := verifyChecksum(content, "reames-agent-linux-amd64.tar.gz", checksumFile); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		checksumFile := []byte(fmt.Sprintf("%s  reames-agent-linux-amd64.tar.gz\n", "0000000000000000000000000000000000000000000000000000000000000000"))
		if err := verifyChecksum(content, "reames-agent-linux-amd64.tar.gz", checksumFile); err == nil {
			t.Error("expected checksum mismatch error")
		}
	})

	t.Run("not found", func(t *testing.T) {
		checksumFile := []byte(fmt.Sprintf("%s  reames-agent-darwin-arm64.tar.gz\n", hash))
		if err := verifyChecksum(content, "reames-agent-linux-amd64.tar.gz", checksumFile); err == nil {
			t.Error("expected not-found error")
		}
	})
}

func TestOfficialReleaseContract(t *testing.T) {
	if ghAPIReleases != "https://api.github.com/repos/Ebonyhtx/reames-agent/releases" {
		t.Fatalf("release API = %q, want official Reames Agent repository", ghAPIReleases)
	}
	if ghDownloadBase != "https://github.com/Ebonyhtx/reames-agent/releases/download" {
		t.Fatalf("download base = %q, want official Reames Agent repository", ghDownloadBase)
	}

	tests := []struct {
		goos, goarch string
		archive      string
		binary       string
	}{
		{"linux", "amd64", "reames-agent-linux-amd64.tar.gz", "reames-agent"},
		{"linux", "arm64", "reames-agent-linux-arm64.tar.gz", "reames-agent"},
		{"darwin", "amd64", "reames-agent-darwin-amd64.tar.gz", "reames-agent"},
		{"darwin", "arm64", "reames-agent-darwin-arm64.tar.gz", "reames-agent"},
		{"windows", "amd64", "reames-agent-windows-amd64.zip", "reames-agent.exe"},
		{"windows", "arm64", "reames-agent-windows-arm64.zip", "reames-agent.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"-"+tt.goarch, func(t *testing.T) {
			if got := releaseAssetName(tt.goos, tt.goarch); got != tt.archive {
				t.Fatalf("releaseAssetName = %q, want %q", got, tt.archive)
			}
			if got := releaseBinaryName(tt.goos); got != tt.binary {
				t.Fatalf("releaseBinaryName = %q, want %q", got, tt.binary)
			}
		})
	}

	assets := []ghAsset{
		{Name: "reames-agent-linux-amd64.tar.gz.sig"},
		{Name: "reames-agent-linux-amd64.tar.gz"},
	}
	asset := findReleaseAsset(assets, releaseAssetName("linux", "amd64"))
	if asset == nil || asset.Name != "reames-agent-linux-amd64.tar.gz" {
		t.Fatalf("findReleaseAsset selected %#v, want exact archive match", asset)
	}
}

func TestUpgradeSuccessMessageIncludesCurrentAndLatestVersions(t *testing.T) {
	cur := "v1.10.0"
	latest := "v1.11.0"

	got := upgradeSuccessMessage(cur, latest)
	if !strings.Contains(got, cur) {
		t.Fatalf("success message %q does not include current version %q", got, cur)
	}
	if !strings.Contains(got, latest) {
		t.Fatalf("success message %q does not include latest version %q", got, latest)
	}
	if strings.Index(got, cur) > strings.Index(got, latest) {
		t.Fatalf("success message %q should report current version before latest version", got)
	}
	if strings.Contains(got, "%!") {
		t.Fatalf("success message %q contains a missing fmt argument marker", got)
	}
}

func TestExtractFromTarGz(t *testing.T) {
	// Build a .tar.gz in memory containing a "reames-agent" entry.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	body := []byte("fake binary content")
	if err := tw.WriteHeader(&tar.Header{
		Name: "reames-agent",
		Mode: 0o755,
		Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromTarGz(buf.Bytes(), "reames-agent")
	if err != nil {
		t.Fatalf("extractFromTarGz: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("extracted body = %q, want %q", got, body)
	}
}

func TestExtractFromTarGz_Nested(t *testing.T) {
	// Archives from goreleaser have the binary at the root with its name.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	body := []byte("nested binary")
	if err := tw.WriteHeader(&tar.Header{
		Name: "reames-agent-linux-amd64/reames-agent",
		Mode: 0o755,
		Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromTarGz(buf.Bytes(), "reames-agent")
	if err != nil {
		t.Fatalf("extractFromTarGz: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("extracted body = %q, want %q", got, body)
	}
}

func TestExtractFromTarGz_NotFound(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{
		Name: "other-file.txt",
		Mode: 0o644,
		Size: 3,
	}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("foo"))
	tw.Close()
	gw.Close()

	_, err := extractFromTarGz(buf.Bytes(), "reames-agent")
	if err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestIsCLITag(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{"v1.6.0", true},
		{"v0.1.0", true},
		{"v2.0.0-rc.1", true},
		{"desktop-v1.5.0", false},
		{"npm-v1.4.0", false},
		{"", false},
		{"v", false},
	}
	for _, tt := range tests {
		if got := isCLITag(tt.tag); got != tt.want {
			t.Errorf("isCLITag(%q) = %v, want %v", tt.tag, got, tt.want)
		}
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{2048, "2.0 KiB"},
		{19_000_000, "18.1 MiB"},
	}
	for _, tt := range tests {
		if got := humanSize(tt.bytes); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestPickCLIRelease(t *testing.T) {
	pick := func(rels []ghRelease) string {
		if r := pickCLIRelease(rels); r != nil {
			return r.TagName
		}
		return ""
	}

	// Skips foreign namespaces (GitHub's "latest" can be a desktop-v release).
	mixed := []ghRelease{
		{TagName: "desktop-v1.6.0"},
		{TagName: "npm-v1.4.0"},
		{TagName: "v1.6.0"},
	}
	if got := pick(mixed); got != "v1.6.0" {
		t.Errorf("foreign namespaces: got %q, want v1.6.0", got)
	}

	// The 1.x line ships as rc on npm @next, so a newer prerelease must be
	// selected, not skipped — `reamesAgent upgrade` always moves to the newest 1.x.
	withRC := []ghRelease{
		{TagName: "v1.7.0-rc.1"},
		{TagName: "v1.6.0"},
	}
	if got := pick(withRC); got != "v1.7.0-rc.1" {
		t.Errorf("newest 1.x (incl. rc) must win: got %q, want v1.7.0-rc.1", got)
	}

	if got := pick([]ghRelease{{TagName: "desktop-v1.0.0"}}); got != "" {
		t.Errorf("no CLI release should return nil, got %q", got)
	}
}

func TestParseBinaryVersionOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   string
		ok     bool
	}{
		{name: "release", output: "reames-agent v1.2.3\n", want: "v1.2.3", ok: true},
		{name: "prerelease", output: "reames-agent v2.0.0-rc.1\n", want: "v2.0.0-rc.1", ok: true},
		{name: "missing prefix", output: "v1.2.3", ok: false},
		{name: "wrong product", output: "other v1.2.3", ok: false},
		{name: "non canonical", output: "reames-agent 1.2.3", ok: false},
		{name: "extra output", output: "reames-agent v1.2.3\nnoise", ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBinaryVersionOutput(tc.output)
			if tc.ok && err != nil {
				t.Fatalf("parseBinaryVersionOutput: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("parseBinaryVersionOutput(%q) unexpectedly succeeded", tc.output)
			}
			if got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProbeBinaryVersionExecutesCandidate(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "candidate.go")
	program := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) != 2 || os.Args[1] != "version" {
		os.Exit(2)
	}
	fmt.Println("reames-agent v9.8.7")
}`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "candidate")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if output, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("build candidate: %v\n%s", err, output)
	}
	got, err := probeBinaryVersion(binary)
	if err != nil {
		t.Fatalf("probeBinaryVersion: %v", err)
	}
	if got != "v9.8.7" {
		t.Fatalf("probeBinaryVersion = %q, want v9.8.7", got)
	}
}

func TestReplaceBinaryAtRejectsUnhealthyCandidateBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent")
	previous := target + ".previous"
	writeUpgradeTestFile(t, target, "current")
	writeUpgradeTestFile(t, previous, "older")
	ops := testUpgradeOps(func(string, string) error {
		return errors.New("injected unhealthy candidate")
	})

	err := replaceBinaryAt(target, []byte("candidate"), "v2.0.0", ops)
	if err == nil || !strings.Contains(err.Error(), "candidate health check") || !strings.Contains(err.Error(), "injected unhealthy candidate") {
		t.Fatalf("replaceBinaryAt error = %v", err)
	}
	assertUpgradeTestFile(t, target, "current")
	assertUpgradeTestFile(t, previous, "older")
}

func TestInstallUpgradeRetainsImmediatePredecessor(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent")
	previous := target + ".previous"
	writeUpgradeTestFile(t, target, "current")
	writeUpgradeTestFile(t, previous, "older")
	ops := testUpgradeOps(func(path, expected string) error {
		if expected != "v2.0.0" {
			t.Fatalf("expected version = %q", expected)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != "candidate" {
			return fmt.Errorf("content = %q", data)
		}
		return nil
	})

	if err := replaceBinaryAt(target, []byte("candidate"), "v2.0.0", ops); err != nil {
		t.Fatalf("replaceBinaryAt: %v", err)
	}
	assertUpgradeTestFile(t, target, "candidate")
	assertUpgradeTestFile(t, previous, "current")
	matches, err := filepath.Glob(filepath.Join(dir, ".reames-agent.previous.saved-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("superseded previous snapshots remain: %v", matches)
	}
}

func TestInstallUpgradePublishFailureRestoresBothVersions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent")
	previous := target + ".previous"
	writeUpgradeTestFile(t, target, "current")
	writeUpgradeTestFile(t, previous, "older")
	injected := errors.New("injected publish failure")
	ops := testUpgradeOps(func(string, string) error { return nil })
	realRename := ops.rename
	ops.rename = func(from, to string) error {
		data, _ := os.ReadFile(from)
		if to == target && string(data) == "candidate" {
			return injected
		}
		return realRename(from, to)
	}

	err := replaceBinaryAt(target, []byte("candidate"), "v2.0.0", ops)
	if err == nil || !errors.Is(err, injected) {
		t.Fatalf("replaceBinaryAt error = %v, want injected failure", err)
	}
	assertUpgradeTestFile(t, target, "current")
	assertUpgradeTestFile(t, previous, "older")
}

func TestInstallUpgradePostCheckFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent")
	previous := target + ".previous"
	writeUpgradeTestFile(t, target, "current")
	writeUpgradeTestFile(t, previous, "older")
	injected := errors.New("injected post-check failure")
	ops := testUpgradeOps(func(path, _ string) error {
		if path == target {
			return injected
		}
		return nil
	})

	err := replaceBinaryAt(target, []byte("candidate"), "v2.0.0", ops)
	if err == nil || !errors.Is(err, injected) || !strings.Contains(err.Error(), "post-install health check") {
		t.Fatalf("replaceBinaryAt error = %v", err)
	}
	assertUpgradeTestFile(t, target, "current")
	assertUpgradeTestFile(t, previous, "older")
}

func TestInstallUpgradeQuarantineFailureStillRestoresBothVersions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent")
	previous := target + ".previous"
	writeUpgradeTestFile(t, target, "current")
	writeUpgradeTestFile(t, previous, "older")
	postCheckErr := errors.New("injected post-check failure")
	quarantineErr := errors.New("injected quarantine failure")
	ops := testUpgradeOps(func(path, _ string) error {
		if path == target {
			return postCheckErr
		}
		return nil
	})
	realRename := ops.rename
	ops.rename = func(from, to string) error {
		if from == target && strings.Contains(filepath.Base(to), ".failed-") {
			return quarantineErr
		}
		return realRename(from, to)
	}

	err := replaceBinaryAt(target, []byte("candidate"), "v2.0.0", ops)
	if err == nil || !errors.Is(err, postCheckErr) {
		t.Fatalf("replaceBinaryAt error = %v", err)
	}
	if errors.Is(err, quarantineErr) || strings.Contains(err.Error(), "degraded") {
		t.Fatalf("successful fallback restoration was reported as degraded: %v", err)
	}
	assertUpgradeTestFile(t, target, "current")
	assertUpgradeTestFile(t, previous, "older")
}

func TestSwapWithPreviousRetainsReplacedVersion(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent")
	previous := target + ".previous"
	writeUpgradeTestFile(t, target, "current")
	writeUpgradeTestFile(t, previous, "previous")
	ops := testUpgradeOps(func(path, expected string) error {
		if path != target || expected != "v1.0.0" {
			return fmt.Errorf("verify(%q, %q)", path, expected)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != "previous" {
			return fmt.Errorf("content = %q", data)
		}
		return nil
	})

	if err := swapWithPrevious(target, "v2.0.0", "v1.0.0", ops); err != nil {
		t.Fatalf("swapWithPrevious: %v", err)
	}
	assertUpgradeTestFile(t, target, "previous")
	assertUpgradeTestFile(t, previous, "current")
}

func TestSwapWithPreviousFailuresRestoreOriginalPair(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(target, previous string, ops *upgradeOps) error
	}{
		{
			name: "publish previous",
			configure: func(target, previous string, ops *upgradeOps) error {
				injected := errors.New("injected previous publish failure")
				realRename := ops.rename
				ops.rename = func(from, to string) error {
					if from == previous && to == target {
						return injected
					}
					return realRename(from, to)
				}
				return injected
			},
		},
		{
			name: "health check",
			configure: func(_, _ string, ops *upgradeOps) error {
				injected := errors.New("injected rollback health failure")
				ops.verify = func(string, string) error { return injected }
				return injected
			},
		},
		{
			name: "retain current",
			configure: func(_, previous string, ops *upgradeOps) error {
				injected := errors.New("injected current retention failure")
				realRename := ops.rename
				ops.rename = func(from, to string) error {
					data, _ := os.ReadFile(from)
					if to == previous && string(data) == "current" {
						return injected
					}
					return realRename(from, to)
				}
				return injected
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "reames-agent")
			previous := target + ".previous"
			writeUpgradeTestFile(t, target, "current")
			writeUpgradeTestFile(t, previous, "previous")
			ops := testUpgradeOps(func(string, string) error { return nil })
			injected := tc.configure(target, previous, &ops)

			err := swapWithPrevious(target, "v2.0.0", "v1.0.0", ops)
			if err == nil || !errors.Is(err, injected) {
				t.Fatalf("swapWithPrevious error = %v, want injected error", err)
			}
			assertUpgradeTestFile(t, target, "current")
			assertUpgradeTestFile(t, previous, "previous")
		})
	}
}

func TestUpgradeLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".reames-agent.upgrade.lock")
	unlock, err := acquireUpgradeLock(path)
	if err != nil {
		t.Fatalf("first acquireUpgradeLock: %v", err)
	}
	if _, err := acquireUpgradeLock(path); !errors.Is(err, errUpgradeLocked) {
		unlock()
		t.Fatalf("second acquireUpgradeLock = %v, want errUpgradeLocked", err)
	}
	unlock()
	unlockAgain, err := acquireUpgradeLock(path)
	if err != nil {
		t.Fatalf("acquireUpgradeLock after release: %v", err)
	}
	unlockAgain()
}

func testUpgradeOps(verify func(string, string) error) upgradeOps {
	return upgradeOps{
		rename:   os.Rename,
		remove:   os.Remove,
		lstat:    os.Lstat,
		tempPath: unusedSiblingPath,
		verify:   verify,
	}
}

func writeUpgradeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertUpgradeTestFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
