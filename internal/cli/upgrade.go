package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/i18n"
	"reames-agent/internal/netclient"

	"golang.org/x/mod/semver"
)

const (
	ghOwner        = "Ebonyhtx"
	ghRepo         = "reames-agent"
	ghAPIReleases  = "https://api.github.com/repos/" + ghOwner + "/" + ghRepo + "/releases"
	ghDownloadBase = "https://github.com/" + ghOwner + "/" + ghRepo + "/releases/download"
	upgradeTimeout = 60 * time.Second
	versionTimeout = 10 * time.Second
)

var errUpgradeLocked = errors.New("another upgrade or rollback is already running")

// ghRelease is the subset of the GitHub release API response we need.
type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []ghAsset
}

// ghAsset is a single release asset.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// upgradeCommand handles `reamesAgent upgrade` (and `reamesAgent update`).
func upgradeCommand(args []string, version string) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	checkOnly := fs.Bool("check", false, "check for updates without installing")
	force := fs.Bool("force", false, "reinstall even if already on the latest version")
	rollback := fs.Bool("rollback", false, "swap the current binary with its retained predecessor")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || (*rollback && (*checkOnly || *force)) {
		fs.Usage()
		return 2
	}

	// 1. Normalize running version.
	cur, ok := normalizeVersion(version)
	if !ok {
		fmt.Fprintf(os.Stderr, "%s %s\n", i18n.M.ErrorPrefix, i18n.M.UpgradeDevBuild)
		return 1
	}
	if *rollback {
		fmt.Println(i18n.M.UpgradeRollbackApplying)
		previous, err := rollbackBinary(cur)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeRollbackFailed+"\n", i18n.M.ErrorPrefix, err)
			return 1
		}
		fmt.Printf(i18n.M.UpgradeRollbackSuccessFmt+"\n", cur, previous)
		fmt.Println(i18n.M.UpgradeGatewayRestartHint)
		return 0
	}

	// 2. Build HTTP client using configured proxy.
	cfg, _ := config.Load()
	spec := cfg.NetworkProxySpec()
	c, err := netclient.NewHTTPClient(spec, netclient.TransportOptions{
		ResponseHeaderTimeout: upgradeTimeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", i18n.M.ErrorPrefix, err)
		return 1
	}

	// 3. Fetch latest release from GitHub API.
	fmt.Println(i18n.M.UpgradeChecking)
	rel, err := fetchLatestRelease(c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeFetchFailed+"\n", i18n.M.ErrorPrefix, err)
		return 1
	}

	// 4. Compare versions.
	latest := rel.TagName
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}
	if !semver.IsValid(latest) {
		fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeInvalidVersion+"\n", i18n.M.ErrorPrefix, latest)
		return 1
	}
	if semver.Compare(latest, cur) <= 0 {
		if *force {
			fmt.Println(i18n.M.UpgradeForcing)
		} else {
			fmt.Println(i18n.M.UpgradeAlreadyLatest)
			return 0
		}
	} else {
		fmt.Printf(i18n.M.UpgradeAvailableFmt+"\n", cur, latest)
	}

	if *checkOnly {
		return 0
	}

	// 5. Find the asset for the current platform.
	assetName := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	asset := findReleaseAsset(rel.Assets, assetName)
	if asset == nil {
		fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeNoAssetFmt+"\n", i18n.M.ErrorPrefix, assetName)
		return 1
	}

	// 6. Find the checksum URL.
	checksumURL := fmt.Sprintf("%s/%s/SHA256SUMS", ghDownloadBase, rel.TagName)

	// 7. Download archive.
	fmt.Printf(i18n.M.UpgradeDownloadingFmt+"\n", asset.Name, humanSize(asset.Size))
	archiveData, err := fetchBytes(c, asset.BrowserDownloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeDownloadFailed+"\n", i18n.M.ErrorPrefix, err)
		return 1
	}

	// 8. Verify SHA256 checksum — fail closed: abort on any verification error.
	fmt.Println(i18n.M.UpgradeVerifying)
	checksumData, err := fetchBytes(c, checksumURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeChecksumFailed+"\n", i18n.M.ErrorPrefix, err)
		return 1
	}
	if err := verifyChecksum(archiveData, asset.Name, checksumData); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", i18n.M.ErrorPrefix, err)
		return 1
	}

	// 9. Extract binary from archive.
	binName := releaseBinaryName(runtime.GOOS)
	binary, err := extractBinary(archiveData, asset.Name, binName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeExtractFailed+"\n", i18n.M.ErrorPrefix, err)
		return 1
	}

	// 10. Health-check and transactionally replace the running binary.
	fmt.Println(i18n.M.UpgradeApplying)
	if err := replaceBinary(binary, latest); err != nil {
		fmt.Fprintf(os.Stderr, "%s "+i18n.M.UpgradeApplyFailed+"\n", i18n.M.ErrorPrefix, err)
		return 1
	}

	fmt.Println(upgradeSuccessMessage(cur, latest))
	fmt.Println(i18n.M.UpgradeGatewayRestartHint)
	return 0
}

func releaseAssetName(goos, goarch string) string {
	extension := ".tar.gz"
	if goos == "windows" {
		extension = ".zip"
	}
	return fmt.Sprintf("reames-agent-%s-%s%s", goos, goarch, extension)
}

func releaseBinaryName(goos string) string {
	if goos == "windows" {
		return "reames-agent.exe"
	}
	return "reames-agent"
}

func findReleaseAsset(assets []ghAsset, name string) *ghAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

func upgradeSuccessMessage(cur, latest string) string {
	return fmt.Sprintf(i18n.M.UpgradeSuccessFmt, cur, latest)
}

// normalizeVersion returns v as valid semver ("vX.Y.Z") or ok=false for dev.
func normalizeVersion(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return "", false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", false
	}
	return semver.Canonical(v), true
}

// isCLITag reports whether a tag belongs to the CLI release namespace (v*).
// Tags like "desktop-v1.5.0" or "npm-v1.4.0" are excluded.
func isCLITag(tag string) bool {
	tag = strings.TrimSpace(tag)
	return len(tag) >= 2 && tag[0] == 'v' && tag[1] >= '0' && tag[1] <= '9'
}

// pickCLIRelease returns the newest CLI-namespace (v*) release from a
// reverse-chronological list, skipping foreign namespaces ("desktop-v",
// "npm-v"). Prereleases are kept: only 1.x carries `reamesAgent upgrade`, and the
// 1.x line ships as rc on npm @next, so there is no stable user to hold back —
// the command should always move to the newest 1.x.
func pickCLIRelease(rels []ghRelease) *ghRelease {
	for i := range rels {
		if isCLITag(rels[i].TagName) {
			return &rels[i]
		}
	}
	return nil
}

// fetchLatestRelease queries the GitHub Releases API and returns the newest
// CLI-namespace (v*) release.
func fetchLatestRelease(c *http.Client) (*ghRelease, error) {
	req, err := http.NewRequest("GET", ghAPIReleases, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "reamesAgent-cli")

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API: %s", resp.Status)
	}

	var rels []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, err
	}

	if rel := pickCLIRelease(rels); rel != nil {
		return rel, nil
	}
	return nil, fmt.Errorf("no CLI release (v*) found in recent releases")
}

// fetchBytes GETs a URL fully into memory.
func fetchBytes(c *http.Client, url string) ([]byte, error) {
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// verifyChecksum checks that data's SHA256 matches the entry for fileName in
// the SHA256SUMS-format checksum file.
func verifyChecksum(data []byte, fileName string, checksumFile []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(strings.TrimSpace(string(checksumFile)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == fileName {
			if !strings.EqualFold(parts[0], got) {
				return fmt.Errorf(i18n.M.UpgradeChecksumMismatchFmt, got, parts[0])
			}
			return nil
		}
	}
	return fmt.Errorf(i18n.M.UpgradeChecksumNotFoundFmt, fileName)
}

// extractBinary pulls the "reames-agent" binary from a .tar.gz or .zip archive.
func extractBinary(data []byte, archiveName, binaryName string) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractFromZip(data, binaryName)
	}
	return extractFromTarGz(data, binaryName)
}

// extractFromTarGz extracts a named binary from a .tar.gz archive.
func extractFromTarGz(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && (h.Name == name || strings.HasSuffix(h.Name, "/"+name)) {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%q not found in archive", name)
}

// extractFromZip extracts a named binary from a .zip archive (Windows).
func extractFromZip(data []byte, name string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if base == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%q not found in zip archive", name)
}

// replaceBinary verifies and publishes newBin while retaining the immediate
// predecessor at <executable>.previous.
func replaceBinary(newBin []byte, expectedVersion string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	resolved, err := resolveSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	dir := filepath.Dir(resolved)
	base := filepath.Base(resolved)
	unlock, err := acquireUpgradeLock(filepath.Join(dir, "."+base+".upgrade.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	return replaceBinaryAt(resolved, newBin, expectedVersion, realUpgradeOps())
}

func replaceBinaryAt(target string, newBin []byte, expectedVersion string, ops upgradeOps) error {
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	staged, err := writeUpgradeCandidate(dir, base, newBin)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	if err := ops.verify(staged, expectedVersion); err != nil {
		return fmt.Errorf("candidate health check: %w", err)
	}
	return installUpgradeCandidate(target, staged, expectedVersion, ops)
}

func rollbackBinary(currentVersion string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	target, err := resolveSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}
	base := filepath.Base(target)
	unlock, err := acquireUpgradeLock(filepath.Join(filepath.Dir(target), "."+base+".upgrade.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()

	previousVersion, err := probeBinaryVersion(target + ".previous")
	if err != nil {
		return "", fmt.Errorf("verify previous binary: %w", err)
	}
	if err := swapWithPrevious(target, currentVersion, previousVersion, realUpgradeOps()); err != nil {
		return "", err
	}
	return previousVersion, nil
}

type upgradeOps struct {
	rename   func(string, string) error
	remove   func(string) error
	lstat    func(string) (os.FileInfo, error)
	tempPath func(string, string) (string, error)
	verify   func(string, string) error
}

func realUpgradeOps() upgradeOps {
	return upgradeOps{
		rename:   os.Rename,
		remove:   os.Remove,
		lstat:    os.Lstat,
		tempPath: unusedSiblingPath,
		verify:   verifyBinaryVersion,
	}
}

func writeUpgradeCandidate(dir, base string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, "."+base+".candidate-*")
	if err != nil {
		return "", fmt.Errorf("create candidate: %w", err)
	}
	path := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := f.Chmod(0o755); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("set candidate permissions: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write candidate: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("sync candidate: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close candidate: %w", err)
	}
	ok = true
	return path, nil
}

func installUpgradeCandidate(target, staged, expectedVersion string, ops upgradeOps) error {
	previous := target + ".previous"
	failed, err := ops.tempPath(filepath.Dir(target), "."+filepath.Base(target)+".failed-*")
	if err != nil {
		return fmt.Errorf("reserve failed-candidate path: %w", err)
	}
	oldPrevious, hadPrevious, err := preserveExistingPrevious(previous, ops)
	if err != nil {
		return err
	}
	restoreOldPrevious := func() error {
		if !hadPrevious {
			return nil
		}
		return ops.rename(oldPrevious, previous)
	}

	if err := ops.rename(target, previous); err != nil {
		return joinUpgradeRollback(fmt.Errorf("retain current binary: %w", err), restoreOldPrevious())
	}
	if err := ops.rename(staged, target); err != nil {
		return joinUpgradeRollback(
			fmt.Errorf("publish candidate: %w", err),
			rollbackUpgradePublish(target, previous, oldPrevious, hadPrevious, "", ops),
		)
	}
	if err := ops.verify(target, expectedVersion); err != nil {
		return joinUpgradeRollback(
			fmt.Errorf("post-install health check: %w", err),
			rollbackUpgradePublish(target, previous, oldPrevious, hadPrevious, failed, ops),
		)
	}
	if hadPrevious {
		if err := ops.remove(oldPrevious); err != nil {
			return joinUpgradeRollback(
				fmt.Errorf("remove superseded previous binary: %w", err),
				rollbackUpgradePublish(target, previous, oldPrevious, true, failed, ops),
			)
		}
	}
	return nil
}

func preserveExistingPrevious(previous string, ops upgradeOps) (string, bool, error) {
	info, err := ops.lstat(previous)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect previous binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("previous binary is not a regular file: %s", previous)
	}
	saved, err := ops.tempPath(filepath.Dir(previous), "."+filepath.Base(previous)+".saved-*")
	if err != nil {
		return "", false, fmt.Errorf("reserve previous snapshot: %w", err)
	}
	if err := ops.rename(previous, saved); err != nil {
		return "", false, fmt.Errorf("snapshot previous binary: %w", err)
	}
	return saved, true, nil
}

func rollbackUpgradePublish(target, previous, oldPrevious string, hadPrevious bool, failed string, ops upgradeOps) error {
	var errs []error
	quarantined := false
	if failed != "" {
		if err := ops.rename(target, failed); err != nil {
			if removeErr := ops.remove(target); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("quarantine failed candidate: %w", err))
				errs = append(errs, fmt.Errorf("remove failed candidate in place: %w", removeErr))
				return errors.Join(errs...)
			}
		} else {
			quarantined = true
		}
	}
	if err := ops.rename(previous, target); err != nil {
		errs = append(errs, fmt.Errorf("restore current binary: %w", err))
	} else if hadPrevious {
		if err := ops.rename(oldPrevious, previous); err != nil {
			errs = append(errs, fmt.Errorf("restore previous binary: %w", err))
		}
	}
	if quarantined {
		if err := ops.remove(failed); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove failed candidate: %w", err))
		}
	}
	return errors.Join(errs...)
}

func swapWithPrevious(target, currentVersion, previousVersion string, ops upgradeOps) error {
	previous := target + ".previous"
	savedCurrent, err := ops.tempPath(filepath.Dir(target), "."+filepath.Base(target)+".rollback-*")
	if err != nil {
		return fmt.Errorf("reserve rollback path: %w", err)
	}
	if err := ops.rename(target, savedCurrent); err != nil {
		return fmt.Errorf("snapshot current binary: %w", err)
	}
	if err := ops.rename(previous, target); err != nil {
		return joinUpgradeRollback(fmt.Errorf("publish previous binary: %w", err), restoreRename(savedCurrent, target, ops))
	}
	if err := ops.verify(target, previousVersion); err != nil {
		return joinUpgradeRollback(fmt.Errorf("rollback health check: %w", err), restoreSwap(target, previous, savedCurrent, ops))
	}
	if err := ops.rename(savedCurrent, previous); err != nil {
		return joinUpgradeRollback(fmt.Errorf("retain replaced %s binary: %w", currentVersion, err), restoreSwap(target, previous, savedCurrent, ops))
	}
	return nil
}

func joinUpgradeRollback(primary, rollback error) error {
	if rollback == nil {
		return primary
	}
	return errors.Join(primary, fmt.Errorf("binary rollback encountered errors; installed state may be degraded; manual repair is required: %w", rollback))
}

func restoreRename(from, to string, ops upgradeOps) error {
	if err := ops.rename(from, to); err != nil {
		return fmt.Errorf("restore current binary: %w", err)
	}
	return nil
}

func restoreSwap(target, previous, savedCurrent string, ops upgradeOps) error {
	if err := ops.rename(target, previous); err != nil {
		return fmt.Errorf("restore previous binary: %w", err)
	}
	return restoreRename(savedCurrent, target, ops)
}

func unusedSiblingPath(dir, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func verifyBinaryVersion(path, expected string) error {
	version, err := probeBinaryVersion(path)
	if err != nil {
		return err
	}
	if version != expected {
		return fmt.Errorf("reported version %s, want %s", version, expected)
	}
	return nil
}

func probeBinaryVersion(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("binary is not a regular file: %s", path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version")
	var output limitedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("version command timed out: %w", ctx.Err())
		}
		return "", fmt.Errorf("version command failed: %w (%s)", err, strings.TrimSpace(output.String()))
	}
	return parseBinaryVersionOutput(output.String())
}

func parseBinaryVersionOutput(output string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 2 || fields[0] != "reames-agent" {
		return "", fmt.Errorf("unexpected version output %q", strings.TrimSpace(output))
	}
	version, ok := normalizeVersion(fields[1])
	if !ok {
		return "", fmt.Errorf("invalid reported version %q", fields[1])
	}
	if fields[1] != version {
		return "", fmt.Errorf("non-canonical reported version %q", fields[1])
	}
	return version, nil
}

type limitedOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	const limit = 4096
	if remaining := limit - w.buf.Len(); remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		_, _ = w.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func (w *limitedOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// resolveSymlinks follows symlinks; falls back to the original path on error.
func resolveSymlinks(p string) (string, error) {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p, nil
	}
	return r, nil
}

// humanSize returns a human-readable byte size.
func humanSize(b int64) string {
	const (
		_KiB = 1024
		_MiB = 1024 * _KiB
	)
	switch {
	case b >= _MiB:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(_MiB))
	case b >= _KiB:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(_KiB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
