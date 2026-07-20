package pluginpkg

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"reames-agent/internal/fileutil"
)

var ErrAlreadyInstalled = errors.New("plugin is already installed")

type InstallRequest struct {
	Name                 string
	Source               string
	SourceRoot           string
	SourceKind           string
	SourceRevision       string
	TrustStatus          string
	RegistryName         string
	RegistryMetadataURL  string
	RegistryRootVersion  int64
	RegistryRootDigest   string
	RegistryEntryDigest  string
	ProvenanceStatus     string
	AttestationDigest    string
	Mode                 string
	ExpectedDigest       string
	ExpectedCurrentState string
	BindCurrentState     bool
	Replace              bool
	AllowNameOverride    bool
}

type InstallResult struct {
	Installed InstalledPlugin
	Warnings  []string
}

type Verification struct {
	Installed InstalledPlugin
	Package   Package
	Warnings  []string
}

var (
	publishPluginGeneration = publishManagedGeneration
	removeManagedPluginPath = removeManagedPath
	digestInstalledContent  = ContentDigest
	beforePluginPublish     = func(string, string) error { return nil }
)

// Install publishes copied packages as immutable, content-addressed
// generations and atomically updates state to point at the new generation.
// A failed state write can leave at most an inactive orphan generation; the
// previous active state and content are never replaced in place.
func Install(reamesAgentHome string, req InstallRequest) (InstallResult, error) {
	req.Name = strings.TrimSpace(req.Name)
	nameKey, err := CanonicalNameKey(req.Name)
	if err != nil {
		return InstallResult{}, err
	}
	req.SourceRoot = filepath.Clean(req.SourceRoot)
	if req.Mode == "" {
		req.Mode = InstallModeCopy
	}
	if req.Mode != InstallModeCopy && req.Mode != InstallModeLink {
		return InstallResult{}, fmt.Errorf("unsupported plugin install mode %q", req.Mode)
	}
	if req.SourceKind == "" {
		req.SourceKind = SourceKindLocal
	}
	if req.TrustStatus == "" {
		req.TrustStatus = TrustLocalSnapshot
		if req.SourceKind == SourceKindGitHub {
			req.TrustStatus = TrustGitHubUnsigned
		}
		if req.Mode == InstallModeLink {
			req.TrustStatus = TrustMutableLink
		}
	}

	var result InstallResult
	err = withStateLock(reamesAgentHome, func() error {
		st, err := loadStateUnlocked(reamesAgentHome)
		if err != nil {
			return err
		}
		existingIndex := -1
		for i := range st.Plugins {
			if st.Plugins[i].Name == req.Name {
				existingIndex = i
				break
			}
			existingKey, _ := CanonicalNameKey(st.Plugins[i].Name)
			if existingKey == nameKey {
				return fmt.Errorf("plugin name %q conflicts with installed plugin %q on case-insensitive filesystems", req.Name, st.Plugins[i].Name)
			}
		}
		if req.BindCurrentState {
			if existingIndex < 0 && req.ExpectedCurrentState != "" {
				return fmt.Errorf("plugin state changed after approval: expected an installed generation")
			}
			if existingIndex >= 0 {
				actual := InstalledStateToken(st.Plugins[existingIndex])
				if req.ExpectedCurrentState == "" || actual != req.ExpectedCurrentState {
					return fmt.Errorf("plugin state changed after approval: got %s, want %s", actual, req.ExpectedCurrentState)
				}
			}
		}
		if existingIndex >= 0 && !req.Replace {
			return fmt.Errorf("%w: %s", ErrAlreadyInstalled, req.Name)
		}

		root, pkg, digest, published, warnings, err := materializeInstall(reamesAgentHome, req)
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			return err
		}
		if !req.AllowNameOverride && pkg.Manifest.Name != req.Name {
			if published {
				_ = removeManagedPluginPath(reamesAgentHome, root)
			}
			return fmt.Errorf("planned plugin name %q but source reports %q", req.Name, pkg.Manifest.Name)
		}

		next := InstalledPlugin{
			Name:                req.Name,
			Source:              req.Source,
			Root:                RelativeRoot(reamesAgentHome, root),
			Version:             pkg.Manifest.Version,
			Description:         pkg.Manifest.Description,
			ManifestKind:        pkg.ManifestKind,
			ManifestSchema:      pkg.Manifest.SchemaVersion,
			InstallMode:         req.Mode,
			SourceKind:          req.SourceKind,
			SourceRevision:      req.SourceRevision,
			TrustStatus:         req.TrustStatus,
			RegistryName:        req.RegistryName,
			RegistryMetadataURL: req.RegistryMetadataURL,
			RegistryRootVersion: req.RegistryRootVersion,
			RegistryRootDigest:  req.RegistryRootDigest,
			RegistryEntryDigest: req.RegistryEntryDigest,
			ProvenanceStatus:    req.ProvenanceStatus,
			AttestationDigest:   req.AttestationDigest,
			Digest:              digest,
			Permissions:         append([]string(nil), pkg.Manifest.Permissions...),
			MCPServerNames:      packageMCPServerNames(pkg),
			MCPServerNamesBound: true,
			LifecycleSecurity:   LifecycleSecurityVersion,
			Enabled:             false,
		}
		if req.Mode == InstallModeLink {
			next.Root = root
		}
		if existingIndex >= 0 {
			previous := st.Plugins[existingIndex]
			next.GrantedPermissions = append([]string(nil), previous.GrantedPermissions...)
			next.Enabled = previous.Enabled && previous.LifecycleSecurity == LifecycleSecurityVersion && permissionsCover(next.GrantedPermissions, next.Permissions)
			if previous.InstallMode == InstallModeCopy && (previous.Root != next.Root || previous.Digest != next.Digest) {
				release := releaseFromInstalled(previous)
				next.Previous = &release
			} else if previous.Previous != nil && previous.Previous.InstallMode == InstallModeCopy {
				next.Previous = previous.Previous
			}
			st.Plugins[existingIndex] = next
			if previous.Enabled && !next.Enabled {
				result.Warnings = append(result.Warnings, "plugin update requested permissions outside the previous grant or migrated a legacy install; it remains disabled until explicitly enabled")
			}
		} else {
			st.Plugins = append(st.Plugins, next)
			result.Warnings = append(result.Warnings, "plugin installed disabled; explicitly enable it after reviewing the requested permissions")
		}
		if err := SaveState(reamesAgentHome, st); err != nil {
			if published {
				if cleanupErr := removeManagedPluginPath(reamesAgentHome, root); cleanupErr != nil {
					return fmt.Errorf("persist plugin state: %w; inactive generation cleanup failed: %v", err, cleanupErr)
				}
			}
			return fmt.Errorf("persist plugin state: %w", err)
		}
		result.Installed = next
		result.Warnings = append(result.Warnings, prunePluginVersions(reamesAgentHome, next)...)
		return nil
	})
	return result, err
}

func materializeInstall(reamesAgentHome string, req InstallRequest) (root string, pkg Package, digest string, published bool, warnings []string, err error) {
	if req.Mode == InstallModeLink {
		root, err = filepath.Abs(req.SourceRoot)
		if err != nil {
			return "", Package{}, "", false, nil, err
		}
		pkg, warnings, digest, err = InspectDir(root)
		if err != nil {
			return "", Package{}, "", false, warnings, err
		}
		if req.ExpectedDigest != "" && digest != req.ExpectedDigest {
			return "", Package{}, "", false, warnings, fmt.Errorf("plugin content changed after approval: got %s, want %s", digest, req.ExpectedDigest)
		}
		return root, pkg, digest, false, warnings, nil
	}

	versions := filepath.Join(InstallRoot(reamesAgentHome, req.Name), "versions")
	if err := ensureManagedDir(reamesAgentHome, req.Name, versions); err != nil {
		return "", Package{}, "", false, nil, err
	}
	if err := ensureManagedRelativeDir(reamesAgentHome, filepath.Join("plugins", ".staging")); err != nil {
		return "", Package{}, "", false, nil, err
	}
	stage, stageRoot, err := makeManagedTempDir(reamesAgentHome, filepath.Join("plugins", ".staging"), req.Name+"-")
	if err != nil {
		return "", Package{}, "", false, nil, err
	}
	defer func() {
		if stageRoot != nil {
			_ = stageRoot.Close()
		}
		if stage != "" {
			_ = removeManagedPluginPath(reamesAgentHome, stage)
		}
	}()
	if err := validateManagedPathIdentity(reamesAgentHome, stage, stageRoot); err != nil {
		return "", Package{}, "", false, nil, err
	}
	if err := copyPluginTree(req.SourceRoot, stageRoot, stage); err != nil {
		return "", Package{}, "", false, nil, err
	}
	if err := validateManagedPathIdentity(reamesAgentHome, stage, stageRoot); err != nil {
		return "", Package{}, "", false, nil, err
	}
	pkg, warnings, err = ParseDir(stage)
	if err != nil {
		return "", Package{}, "", false, warnings, err
	}
	digest, err = contentDigestRoot(stageRoot)
	if err != nil {
		return "", Package{}, "", false, warnings, err
	}
	if err := validateManagedPathIdentity(reamesAgentHome, stage, stageRoot); err != nil {
		return "", Package{}, "", false, warnings, err
	}
	if req.ExpectedDigest != "" && digest != req.ExpectedDigest {
		return "", Package{}, "", false, warnings, fmt.Errorf("plugin content changed after approval: got %s, want %s", digest, req.ExpectedDigest)
	}
	id, err := digestID(digest)
	if err != nil {
		return "", Package{}, "", false, warnings, err
	}
	root = filepath.Join(versions, id)
	if _, statErr := os.Lstat(root); statErr == nil {
		existingDigest, digestErr := ContentDigest(root)
		if digestErr != nil || existingDigest != digest {
			return "", Package{}, "", false, warnings, fmt.Errorf("existing plugin generation %s failed integrity verification", root)
		}
		_ = stageRoot.Close()
		stageRoot = nil
		_ = removeManagedPluginPath(reamesAgentHome, stage)
		stage = ""
		pkg, warnings, err = ParseDir(root)
		return root, pkg, digest, false, warnings, err
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", Package{}, "", false, warnings, statErr
	}
	if err := beforePluginPublish(reamesAgentHome, stage); err != nil {
		return "", Package{}, "", false, warnings, err
	}
	if err := validateManagedPathIdentity(reamesAgentHome, stage, stageRoot); err != nil {
		return "", Package{}, "", false, warnings, err
	}
	if err := publishPluginGeneration(reamesAgentHome, stage, root); err != nil {
		_ = stageRoot.Close()
		stageRoot = nil
		if _, statErr := os.Lstat(root); statErr == nil {
			if cleanupErr := removeManagedPluginPath(reamesAgentHome, root); cleanupErr != nil {
				return "", Package{}, "", false, warnings, fmt.Errorf("publish plugin generation: %w; cleanup published orphan: %v", err, cleanupErr)
			}
		}
		return "", Package{}, "", false, warnings, err
	}
	stage = ""
	if err := validateManagedPathIdentity(reamesAgentHome, root, stageRoot); err != nil {
		_ = stageRoot.Close()
		stageRoot = nil
		_ = removeManagedPluginPath(reamesAgentHome, root)
		return "", Package{}, "", false, warnings, err
	}
	pkg, warnings, err = ParseDir(root)
	if err != nil {
		_ = stageRoot.Close()
		stageRoot = nil
		_ = removeManagedPluginPath(reamesAgentHome, root)
		return "", Package{}, "", false, warnings, err
	}
	publishedDigest, err := contentDigestRoot(stageRoot)
	if err != nil || publishedDigest != digest {
		_ = stageRoot.Close()
		stageRoot = nil
		_ = removeManagedPluginPath(reamesAgentHome, root)
		if err != nil {
			return "", Package{}, "", false, warnings, err
		}
		return "", Package{}, "", false, warnings, fmt.Errorf("published plugin generation digest changed: got %s, want %s", publishedDigest, digest)
	}
	if err := validateManagedPathIdentity(reamesAgentHome, root, stageRoot); err != nil {
		_ = stageRoot.Close()
		stageRoot = nil
		_ = removeManagedPluginPath(reamesAgentHome, root)
		return "", Package{}, "", false, warnings, err
	}
	return root, pkg, digest, true, warnings, nil
}

func copyPluginTree(source string, destinationRoot *os.Root, destination string) error {
	source = filepath.Clean(source)
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer sourceRoot.Close()
	var files int
	var total int64
	dirs := []string{destination}
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if strings.EqualFold(entry.Name(), ".git") {
				return filepath.SkipDir
			}
			if err := destinationRoot.MkdirAll(rel, 0o755); err != nil {
				return err
			}
			dirs = append(dirs, filepath.Join(destination, rel))
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin package contains symlink %s", filepath.ToSlash(rel))
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("plugin package contains special file %s", filepath.ToSlash(rel))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		total += info.Size()
		if files > maxPluginFiles || total > maxPluginBytes {
			return fmt.Errorf("plugin package exceeds copy limits (%d files, %d bytes)", maxPluginFiles, maxPluginBytes)
		}
		input, err := sourceRoot.Open(rel)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		output, err := destinationRoot.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			_ = input.Close()
			return err
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, info.Size()+1))
		syncErr := output.Sync()
		closeOutErr := output.Close()
		closeInErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		if written != info.Size() {
			return fmt.Errorf("plugin file changed while copying: %s", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err := fileutil.SyncParentDir(filepath.Join(dir, ".")); err != nil {
			return err
		}
	}
	return nil
}

func ensureManagedDir(home, name, dir string) error {
	if !IsValidName(name) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	expected := filepath.Join(InstallRoot(home, name), "versions")
	if !samePath(expected, dir) {
		return fmt.Errorf("plugin managed directory %s does not match %s", dir, expected)
	}
	return ensureManagedRelativeDir(home, filepath.Join("plugins", name, "versions"))
}

func ensureManagedRelativeDir(home, rel string) error {
	if strings.TrimSpace(home) == "" {
		return fmt.Errorf("plugin home directory is empty")
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(rel, 0o700); err != nil {
		return fmt.Errorf("create managed plugin directory %s: %w", rel, err)
	}
	return nil
}

func makeManagedTempDir(home, parent, prefix string) (string, *os.Root, error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	for range 100 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		rel := filepath.Join(parent, prefix+hex.EncodeToString(random[:]))
		if err := root.Mkdir(rel, 0o700); err == nil {
			stageRoot, openErr := root.OpenRoot(rel)
			if openErr != nil {
				_ = root.RemoveAll(rel)
				return "", nil, openErr
			}
			return filepath.Join(home, rel), stageRoot, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("create managed staging directory: exhausted unique names")
}

func validateManagedPathIdentity(home, path string, opened *os.Root) error {
	if opened == nil {
		return fmt.Errorf("managed plugin path %s has no open identity", path)
	}
	openedInfo, err := opened.Stat(".")
	if err != nil {
		return err
	}
	rel, err := managedRelativePath(home, path)
	if err != nil {
		return err
	}
	homeRoot, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer homeRoot.Close()
	pathInfo, err := homeRoot.Lstat(rel)
	if err != nil {
		return err
	}
	if !pathInfo.IsDir() || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, pathInfo) {
		return fmt.Errorf("managed plugin path identity changed: %s", path)
	}
	return nil
}

func managedRelativePath(home, path string) (string, error) {
	homeAbs, err := filepath.Abs(home)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(homeAbs, pathAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("managed plugin path %s escapes %s", path, home)
	}
	return rel, nil
}

func publishManagedGeneration(home, staged, destination string) error {
	stagedRel, err := managedRelativePath(home, staged)
	if err != nil {
		return err
	}
	destinationRel, err := managedRelativePath(home, destination)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Rename(stagedRel, destinationRel); err != nil {
		return err
	}
	return fileutil.SyncParentDir(destination)
}

func removeManagedPath(home, path string) error {
	rel, err := managedRelativePath(home, path)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(rel)
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftAbs = filepath.Clean(leftAbs)
	rightAbs = filepath.Clean(rightAbs)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftAbs, rightAbs)
	}
	return leftAbs == rightAbs
}

func VerifyInstalled(reamesAgentHome, name string) (Verification, error) {
	installed, ok, err := FindInstalled(reamesAgentHome, name)
	if err != nil {
		return Verification{}, err
	}
	if !ok {
		return Verification{}, fmt.Errorf("plugin %q is not installed", name)
	}
	verified, pkg, warnings, err := verifyInstalledDetailed(reamesAgentHome, installed, false)
	return Verification{Installed: verified, Package: pkg, Warnings: warnings}, err
}

func verifyInstalled(reamesAgentHome string, installed InstalledPlugin, adopt bool) (InstalledPlugin, error) {
	verified, _, _, err := verifyInstalledDetailed(reamesAgentHome, installed, adopt)
	return verified, err
}

func verifyInstalledDetailed(reamesAgentHome string, installed InstalledPlugin, adopt bool) (InstalledPlugin, Package, []string, error) {
	root := ResolveRoot(reamesAgentHome, installed.Root)
	if installed.InstallMode == InstallModeCopy && installed.LifecycleSecurity == LifecycleSecurityVersion {
		if err := validateManagedGeneration(reamesAgentHome, installed.Name, root); err != nil {
			return installed, Package{}, nil, err
		}
	}
	digestBefore, err := digestInstalledContent(root)
	if err != nil {
		return installed, Package{}, nil, err
	}
	pkg, warnings, err := ParseDir(root)
	if err != nil {
		return installed, Package{}, warnings, err
	}
	digest, err := digestInstalledContent(root)
	if err != nil {
		return installed, Package{}, warnings, err
	}
	if digestBefore != digest {
		return installed, Package{}, warnings, fmt.Errorf("plugin %s content changed during verification: before %s, after %s", installed.Name, digestBefore, digest)
	}
	actualPermissions := append([]string(nil), pkg.Manifest.Permissions...)
	actualMCPServerNames := packageMCPServerNames(pkg)
	if installed.LifecycleSecurity == LifecycleSecurityVersion {
		mutableAdoption := adopt && installed.InstallMode == InstallModeLink
		if installed.Digest == "" {
			return installed, Package{}, warnings, fmt.Errorf("plugin %s has no persisted content digest", installed.Name)
		}
		if installed.Digest != digest && !mutableAdoption {
			return installed, Package{}, warnings, fmt.Errorf("plugin %s content digest mismatch: got %s, want %s", installed.Name, digest, installed.Digest)
		}
		if !sameStrings(installed.Permissions, actualPermissions) && !mutableAdoption {
			return installed, Package{}, warnings, fmt.Errorf("plugin %s permission set changed: got %v, want %v", installed.Name, actualPermissions, installed.Permissions)
		}
		if installed.MCPServerNamesBound && !sameStrings(installed.MCPServerNames, actualMCPServerNames) && !mutableAdoption {
			return installed, Package{}, warnings, fmt.Errorf("plugin %s MCP server set changed: got %v, want %v", installed.Name, actualMCPServerNames, installed.MCPServerNames)
		}
		if installed.Enabled && !adopt && !permissionsCover(installed.GrantedPermissions, actualPermissions) {
			return installed, Package{}, warnings, fmt.Errorf("plugin %s is enabled without grants for %v", installed.Name, actualPermissions)
		}
	} else {
		warnings = append(warnings, "legacy installation is not content-addressed; re-enable or reinstall it to persist integrity and permission grants")
	}
	installed.Digest = digest
	installed.Permissions = actualPermissions
	installed.MCPServerNames = actualMCPServerNames
	installed.MCPServerNamesBound = true
	installed.Version = pkg.Manifest.Version
	installed.Description = pkg.Manifest.Description
	installed.ManifestKind = pkg.ManifestKind
	installed.ManifestSchema = pkg.Manifest.SchemaVersion
	if installed.InstallMode == "" {
		if pathWithin(InstallRoot(reamesAgentHome, installed.Name), root) {
			installed.InstallMode = InstallModeCopy
			installed.TrustStatus = TrustLocalSnapshot
		} else {
			installed.InstallMode = InstallModeLink
			installed.TrustStatus = TrustMutableLink
		}
	}
	return installed, pkg, warnings, nil
}

func packageMCPServerNames(pkg Package) []string {
	names := make([]string, 0, len(pkg.Manifest.MCPServers))
	for name := range pkg.Manifest.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateManagedGeneration(home, name, root string) error {
	if !IsValidName(name) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	versions := filepath.Join(InstallRoot(home, name), "versions")
	if !pathWithin(versions, root) || samePath(versions, root) {
		return fmt.Errorf("plugin %s root is outside its managed generation directory", name)
	}
	rel, err := managedRelativePath(home, root)
	if err != nil {
		return err
	}
	homeRoot, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer homeRoot.Close()
	info, err := homeRoot.Lstat(rel)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("plugin %s generation root is not a real directory", name)
	}
	return nil
}

func pathWithin(base, candidate string) bool {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func permissionsCover(granted, required []string) bool {
	set := map[string]bool{}
	for _, permission := range granted {
		set[permission] = true
	}
	for _, permission := range required {
		if !set[permission] {
			return false
		}
	}
	return true
}

func releaseFromInstalled(installed InstalledPlugin) PluginRelease {
	return PluginRelease{
		Source:              installed.Source,
		Root:                installed.Root,
		Version:             installed.Version,
		Description:         installed.Description,
		ManifestKind:        installed.ManifestKind,
		ManifestSchema:      installed.ManifestSchema,
		InstallMode:         installed.InstallMode,
		SourceKind:          installed.SourceKind,
		SourceRevision:      installed.SourceRevision,
		TrustStatus:         installed.TrustStatus,
		RegistryName:        installed.RegistryName,
		RegistryMetadataURL: installed.RegistryMetadataURL,
		RegistryRootVersion: installed.RegistryRootVersion,
		RegistryRootDigest:  installed.RegistryRootDigest,
		RegistryEntryDigest: installed.RegistryEntryDigest,
		ProvenanceStatus:    installed.ProvenanceStatus,
		AttestationDigest:   installed.AttestationDigest,
		Digest:              installed.Digest,
		Permissions:         append([]string(nil), installed.Permissions...),
		GrantedPermissions:  append([]string(nil), installed.GrantedPermissions...),
		MCPServerNames:      append([]string(nil), installed.MCPServerNames...),
		MCPServerNamesBound: installed.MCPServerNamesBound,
		LifecycleSecurity:   installed.LifecycleSecurity,
		Enabled:             installed.Enabled,
	}
}

func installedFromRelease(name string, release PluginRelease) InstalledPlugin {
	return InstalledPlugin{
		Name:                name,
		Source:              release.Source,
		Root:                release.Root,
		Version:             release.Version,
		Description:         release.Description,
		ManifestKind:        release.ManifestKind,
		ManifestSchema:      release.ManifestSchema,
		InstallMode:         release.InstallMode,
		SourceKind:          release.SourceKind,
		SourceRevision:      release.SourceRevision,
		TrustStatus:         release.TrustStatus,
		RegistryName:        release.RegistryName,
		RegistryMetadataURL: release.RegistryMetadataURL,
		RegistryRootVersion: release.RegistryRootVersion,
		RegistryRootDigest:  release.RegistryRootDigest,
		RegistryEntryDigest: release.RegistryEntryDigest,
		ProvenanceStatus:    release.ProvenanceStatus,
		AttestationDigest:   release.AttestationDigest,
		Digest:              release.Digest,
		Permissions:         append([]string(nil), release.Permissions...),
		GrantedPermissions:  append([]string(nil), release.GrantedPermissions...),
		MCPServerNames:      append([]string(nil), release.MCPServerNames...),
		MCPServerNamesBound: release.MCPServerNamesBound,
		LifecycleSecurity:   release.LifecycleSecurity,
		Enabled:             release.Enabled,
	}
}

func Rollback(reamesAgentHome, name string) (InstalledPlugin, []string, error) {
	return RollbackApproved(reamesAgentHome, RollbackRequest{Name: name})
}

type RollbackRequest struct {
	Name                 string
	ExpectedCurrentState string
	BindCurrentState     bool
}

func RollbackApproved(reamesAgentHome string, req RollbackRequest) (InstalledPlugin, []string, error) {
	name := req.Name
	if !IsValidName(name) {
		return InstalledPlugin{}, nil, fmt.Errorf("invalid plugin name %q", name)
	}
	var restored InstalledPlugin
	var warnings []string
	err := withStateLock(reamesAgentHome, func() error {
		st, err := loadStateUnlocked(reamesAgentHome)
		if err != nil {
			return err
		}
		for i := range st.Plugins {
			current := st.Plugins[i]
			if current.Name != name {
				continue
			}
			if req.BindCurrentState {
				actual := InstalledStateToken(current)
				if req.ExpectedCurrentState == "" || actual != req.ExpectedCurrentState {
					return fmt.Errorf("plugin state changed after approval: got %s, want %s", actual, req.ExpectedCurrentState)
				}
			}
			if current.Previous == nil {
				return fmt.Errorf("plugin %q has no rollback generation", name)
			}
			next := installedFromRelease(name, *current.Previous)
			verified, _, verifyWarnings, err := verifyInstalledDetailed(reamesAgentHome, next, false)
			warnings = append(warnings, verifyWarnings...)
			if err != nil {
				return fmt.Errorf("verify rollback generation: %w", err)
			}
			if verified.Enabled && !permissionsCover(verified.GrantedPermissions, verified.Permissions) {
				verified.Enabled = false
				warnings = append(warnings, "rollback generation remains disabled because its permissions are not granted")
			}
			if current.InstallMode == InstallModeCopy {
				previous := releaseFromInstalled(current)
				verified.Previous = &previous
			} else {
				verified.Previous = nil
			}
			st.Plugins[i] = verified
			if err := SaveState(reamesAgentHome, st); err != nil {
				return err
			}
			restored = verified
			return nil
		}
		return fmt.Errorf("plugin %q is not installed", name)
	})
	return restored, warnings, err
}

type UninstallRequest struct {
	Name                 string
	ExpectedCurrentState string
	BindCurrentState     bool
}

func Uninstall(reamesAgentHome, name string) (InstalledPlugin, []string, bool, error) {
	return UninstallApproved(reamesAgentHome, UninstallRequest{Name: name})
}

// UninstallApproved removes only the exact lifecycle state shown to the
// approver. The comparison and state removal occur under the same process and
// cross-process lock, so a concurrent update cannot turn an approved removal
// into deletion of a different generation or permission state.
func UninstallApproved(reamesAgentHome string, req UninstallRequest) (InstalledPlugin, []string, bool, error) {
	name := strings.TrimSpace(req.Name)
	if !IsValidName(name) {
		return InstalledPlugin{}, nil, false, fmt.Errorf("invalid plugin name %q", name)
	}
	var removed InstalledPlugin
	var found bool
	var warnings []string
	err := withStateLock(reamesAgentHome, func() error {
		st, err := loadStateUnlocked(reamesAgentHome)
		if err != nil {
			return err
		}
		for i, installed := range st.Plugins {
			if installed.Name != name {
				continue
			}
			if req.BindCurrentState {
				actual := InstalledStateToken(installed)
				if req.ExpectedCurrentState == "" || actual != req.ExpectedCurrentState {
					return fmt.Errorf("plugin state changed after approval: got %s, want %s", actual, req.ExpectedCurrentState)
				}
			}
			removed, found = installed, true
			st.Plugins = append(st.Plugins[:i], st.Plugins[i+1:]...)
			if err := SaveState(reamesAgentHome, st); err != nil {
				return err
			}
			if err := removeManagedPluginPath(reamesAgentHome, InstallRoot(reamesAgentHome, name)); err != nil {
				warnings = append(warnings, fmt.Sprintf("plugin was disabled and removed from state, but managed content cleanup failed: %v", err))
			}
			return nil
		}
		if req.BindCurrentState {
			return fmt.Errorf("plugin state changed after approval: %q is no longer installed", name)
		}
		return nil
	})
	return removed, warnings, found, err
}

func prunePluginVersions(home string, installed InstalledPlugin) []string {
	if installed.InstallMode != InstallModeCopy {
		return nil
	}
	keep := map[string]bool{filepath.Clean(ResolveRoot(home, installed.Root)): true}
	if installed.Previous != nil && installed.Previous.InstallMode == InstallModeCopy {
		keep[filepath.Clean(ResolveRoot(home, installed.Previous.Root))] = true
	}
	versions := filepath.Join(InstallRoot(home, installed.Name), "versions")
	entries, err := os.ReadDir(versions)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []string{fmt.Sprintf("failed to inspect inactive plugin generations %s: %v", versions, err)}
	}
	var warnings []string
	for _, entry := range entries {
		path := filepath.Join(versions, entry.Name())
		if keep[filepath.Clean(path)] {
			continue
		}
		if err := removeManagedPluginPath(home, path); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to prune inactive plugin generation %s: %v", path, err))
		}
	}
	return warnings
}
