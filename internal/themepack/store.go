package themepack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/fileutil"
)

const (
	storeDirName        = "theme-packs"
	storeStateVersion   = 1
	transactionVersion  = 1
	stateFileName       = "state.json"
	transactionFileName = "transaction.json"
)

var ErrPackExists = errors.New("theme pack already exists")

// InstalledPack is the durable record for one verified user package.
type InstalledPack struct {
	Manifest      Manifest `json:"manifest"`
	PackageDigest string   `json:"packageDigest"`
	ImportedAt    string   `json:"importedAt"`
}

type State struct {
	SchemaVersion        int    `json:"schemaVersion"`
	AppliedThemeID       string `json:"appliedThemeId,omitempty"`
	AppliedPackageDigest string `json:"appliedPackageDigest,omitempty"`
	Revision             uint64 `json:"revision"`
}

type Snapshot struct {
	AppliedThemeID string          `json:"appliedThemeId,omitempty"`
	PreviewThemeID string          `json:"previewThemeId,omitempty"`
	EffectiveID    string          `json:"effectiveId,omitempty"`
	Packs          []InstalledPack `json:"packs"`
	Warnings       []string        `json:"warnings"`
	SafeMode       bool            `json:"safeMode"`
}

type AssetContent struct {
	Data    []byte
	MIME    string
	Name    string
	ModTime time.Time
}

type Store struct {
	root string
	mu   sync.Mutex

	previewID string
	now       func() time.Time
	failpoint func(string) error
}

type transaction struct {
	SchemaVersion int             `json:"schemaVersion"`
	Operation     string          `json:"operation"` // install|delete
	ID            string          `json:"id"`
	DesiredDigest string          `json:"desiredDigest,omitempty"`
	PreviousPack  json.RawMessage `json:"previousPack,omitempty"`
	PreviousState State           `json:"previousState"`
	DesiredState  State           `json:"desiredState,omitempty"`
	Tombstone     string          `json:"tombstone,omitempty"`
	PreparedAt    string          `json:"preparedAt"`
}

// NewStore creates a store rooted beneath the Reames Agent home. It performs no
// I/O until the first operation so Safe Mode can avoid reading user themes.
func NewStore(home string) *Store {
	root := ""
	if strings.TrimSpace(home) != "" {
		root = filepath.Join(filepath.Clean(home), storeDirName)
	}
	return &Store{
		root: root,
		now:  time.Now,
	}
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// Inspect verifies an archive without modifying store state.
func (s *Store) Inspect(path string) (Candidate, error) { return InspectArchive(path) }

// Install publishes a verified archive. Replacements preserve the old record
// until the new canonical record is durably ready.
func (s *Store) Install(candidate Candidate, replace bool) (InstalledPack, error) {
	if s == nil {
		return InstalledPack{}, fmt.Errorf("theme store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.prepareLocked()
	if err != nil {
		return InstalledPack{}, err
	}
	defer root.Close()

	canonical, err := canonicalManifest(candidate.Manifest)
	if err != nil {
		return InstalledPack{}, err
	}
	wantDigest := canonicalPackageDigest(canonical, manifestAssetRefs(candidate.Manifest))
	if candidate.PackageDigest != wantDigest {
		return InstalledPack{}, fmt.Errorf("candidate package digest mismatch")
	}
	packName := packRecordName(candidate.Manifest.ID)
	previous, readErr := readRootFileLimited(root, packName, MaxManifestBytes+4096)
	exists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return InstalledPack{}, readErr
	}
	if exists && !replace {
		return InstalledPack{}, fmt.Errorf("%w: %s", ErrPackExists, candidate.Manifest.ID)
	}
	state, err := readState(root)
	if err != nil {
		return InstalledPack{}, err
	}
	desiredState := state
	if desiredState.AppliedThemeID == candidate.Manifest.ID {
		desiredState.Revision++
		desiredState.AppliedPackageDigest = candidate.PackageDigest
	}
	for _, ref := range manifestAssetRefs(candidate.Manifest) {
		asset, ok := candidate.assets[strings.ToLower(ref.File)]
		if !ok || asset.Ref != ref {
			return InstalledPack{}, fmt.Errorf("candidate is missing verified asset %s", ref.SHA256)
		}
		if err := s.writeAssetLocked(root, asset); err != nil {
			return InstalledPack{}, err
		}
	}
	if err := s.fail("after-assets"); err != nil {
		return InstalledPack{}, err
	}

	txn := transaction{
		SchemaVersion: transactionVersion,
		Operation:     "install",
		ID:            candidate.Manifest.ID,
		DesiredDigest: candidate.PackageDigest,
		PreviousPack:  previous,
		PreviousState: state,
		DesiredState:  desiredState,
		PreparedAt:    s.now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeRootJSON(root, transactionFileName, txn, 0o600); err != nil {
		return InstalledPack{}, err
	}
	if err := s.fail("after-prepare"); err != nil {
		return InstalledPack{}, err
	}
	record := InstalledPack{
		Manifest:      candidate.Manifest,
		PackageDigest: candidate.PackageDigest,
		ImportedAt:    s.now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeRootJSON(root, packName, record, 0o600); err != nil {
		return InstalledPack{}, err
	}
	if err := s.fail("after-pack-write"); err != nil {
		return InstalledPack{}, err
	}
	if desiredState != state {
		if err := writeRootJSON(root, stateFileName, desiredState, 0o600); err != nil {
			return InstalledPack{}, err
		}
	}
	if err := s.fail("after-install-state"); err != nil {
		return InstalledPack{}, err
	}
	if err := clearTransaction(root); err != nil {
		return InstalledPack{}, err
	}
	_ = s.gcAssetsLocked(root)
	return record, nil
}

// InstallArchive verifies and publishes an archive in one call.
func (s *Store) InstallArchive(path string, replace bool) (InstalledPack, error) {
	candidate, err := InspectArchive(path)
	if err != nil {
		return InstalledPack{}, err
	}
	return s.Install(candidate, replace)
}

// Snapshot returns deterministic installed and active state. Safe Mode bypasses
// all user-theme filesystem reads and always projects the built-in Graphite base.
func (s *Store) Snapshot(safeMode bool) (Snapshot, error) {
	if safeMode {
		return Snapshot{Packs: []InstalledPack{}, Warnings: []string{}, SafeMode: true}, nil
	}
	if s == nil {
		return Snapshot{}, fmt.Errorf("theme store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.prepareLocked()
	if err != nil {
		return Snapshot{}, err
	}
	defer root.Close()
	_ = s.gcAssetsLocked(root)
	packs, warnings, err := listPacks(root)
	if err != nil {
		return Snapshot{}, err
	}
	state, err := readState(root)
	if err != nil {
		return Snapshot{}, err
	}
	byID := make(map[string]InstalledPack, len(packs))
	for _, pack := range packs {
		byID[pack.Manifest.ID] = pack
	}
	applied := state.AppliedThemeID
	if applied != "" {
		pack, ok := officialPack(applied)
		if !ok {
			pack, ok = byID[applied]
		}
		if !ok || pack.PackageDigest != state.AppliedPackageDigest {
			warnings = append(warnings, fmt.Sprintf("applied theme %q is unavailable or changed; using the configured base style", applied))
			applied = ""
		}
	}
	preview := s.previewID
	if preview != "" {
		_, official := officialPack(preview)
		_, user := byID[preview]
		if !official && !user {
			preview = ""
			s.previewID = ""
		}
	}
	effective := applied
	if preview != "" {
		effective = preview
	}
	return Snapshot{
		AppliedThemeID: applied,
		PreviewThemeID: preview,
		EffectiveID:    effective,
		Packs:          packs,
		Warnings:       warnings,
	}, nil
}

// Active reads only the committed active record, allowing desktop startup to
// restore appearance without enumerating the lazy Gallery library.
func (s *Store) Active(safeMode bool) (State, *InstalledPack, string, error) {
	if safeMode {
		return State{SchemaVersion: storeStateVersion}, nil, "", nil
	}
	if s == nil {
		return State{}, nil, "", fmt.Errorf("theme store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.prepareLocked()
	if err != nil {
		return State{}, nil, "", err
	}
	defer root.Close()
	state, err := readState(root)
	if err != nil {
		return State{}, nil, "", err
	}
	if state.AppliedThemeID == "" {
		return state, nil, "", nil
	}
	pack, ok := officialPack(state.AppliedThemeID)
	if ok {
		if pack.PackageDigest != state.AppliedPackageDigest {
			return state, nil, fmt.Sprintf("applied theme %q is unavailable or changed; using the configured base style", state.AppliedThemeID), nil
		}
		return state, &pack, "", nil
	}
	pack, err = readPack(root, state.AppliedThemeID)
	if err != nil || pack.PackageDigest != state.AppliedPackageDigest {
		return state, nil, fmt.Sprintf("applied theme %q is unavailable or changed; using the configured base style", state.AppliedThemeID), nil
	}
	for _, ref := range manifestAssetRefs(pack.Manifest) {
		if err := validateStoredAsset(root, ref.SHA256); err != nil {
			return state, nil, fmt.Sprintf("applied theme %q has an invalid asset; using the configured base style", state.AppliedThemeID), nil
		}
	}
	return state, &pack, "", nil
}

// Has reports whether id currently names a valid installed package.
func (s *Store) Has(id string) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("theme store is nil")
	}
	if IsOfficialID(id) {
		return true, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.prepareLocked()
	if err != nil {
		return false, err
	}
	defer root.Close()
	_, err = readPack(root, strings.TrimSpace(id))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// Preview changes only process memory. A crash or relaunch therefore always
// returns to the last durable Apply operation.
func (s *Store) Preview(id string, safeMode bool) (Snapshot, error) {
	if safeMode {
		return Snapshot{}, fmt.Errorf("user theme preview is disabled in Safe Mode")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return s.CancelPreview(false)
	}
	s.mu.Lock()
	var root *os.Root
	var err error
	if _, ok := officialPack(id); !ok {
		root, err = s.prepareLocked()
		if err == nil {
			_, err = readPack(root, id)
		}
	}
	if root != nil {
		_ = root.Close()
	}
	if err == nil {
		s.previewID = id
	}
	s.mu.Unlock()
	if err != nil {
		return Snapshot{}, err
	}
	return s.Snapshot(false)
}

func (s *Store) CancelPreview(safeMode bool) (Snapshot, error) {
	if safeMode {
		return s.Snapshot(true)
	}
	s.mu.Lock()
	s.previewID = ""
	s.mu.Unlock()
	return s.Snapshot(false)
}

// Apply atomically persists one installed pack id. Empty id returns to the
// separately configured base style.
func (s *Store) Apply(id string, safeMode bool) (Snapshot, error) {
	if safeMode {
		return Snapshot{}, fmt.Errorf("user themes are disabled in Safe Mode")
	}
	if s == nil {
		return Snapshot{}, fmt.Errorf("theme store is nil")
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	root, err := s.prepareLocked()
	if err != nil {
		s.mu.Unlock()
		return Snapshot{}, err
	}
	state, err := readState(root)
	if err == nil {
		state.SchemaVersion = storeStateVersion
		state.Revision++
		state.AppliedThemeID = ""
		state.AppliedPackageDigest = ""
		if id != "" {
			var pack InstalledPack
			var ok bool
			pack, ok = officialPack(id)
			if !ok {
				pack, err = readPack(root, id)
			}
			if err == nil {
				state.AppliedThemeID = id
				state.AppliedPackageDigest = pack.PackageDigest
			}
		}
	}
	if err == nil {
		err = writeRootJSON(root, stateFileName, state, 0o600)
	}
	_ = root.Close()
	if err == nil {
		s.previewID = ""
	}
	s.mu.Unlock()
	if err != nil {
		return Snapshot{}, err
	}
	return s.Snapshot(false)
}

// Delete rolls forward through a tombstone journal. If the deleted pack was
// active, state is cleared before its record disappears.
func (s *Store) Delete(id string, safeMode bool) (Snapshot, error) {
	if safeMode {
		return Snapshot{}, fmt.Errorf("user themes are disabled in Safe Mode")
	}
	if s == nil {
		return Snapshot{}, fmt.Errorf("theme store is nil")
	}
	id = strings.TrimSpace(id)
	if !idPattern.MatchString(id) || IsBaseStyle(id) || IsOfficialID(id) {
		return Snapshot{}, fmt.Errorf("invalid user theme id %q", id)
	}
	s.mu.Lock()
	root, err := s.prepareLocked()
	if err != nil {
		s.mu.Unlock()
		return Snapshot{}, err
	}
	if _, err = readPack(root, id); err != nil {
		_ = root.Close()
		s.mu.Unlock()
		return Snapshot{}, err
	}
	state, err := readState(root)
	if err != nil {
		_ = root.Close()
		s.mu.Unlock()
		return Snapshot{}, err
	}
	tombstone := filepath.Join("trash", id+"-"+fmt.Sprintf("%d", state.Revision+1)+".json")
	txn := transaction{
		SchemaVersion: transactionVersion,
		Operation:     "delete",
		ID:            id,
		PreviousState: state,
		Tombstone:     tombstone,
		PreparedAt:    s.now().UTC().Format(time.RFC3339Nano),
	}
	if err = writeRootJSON(root, transactionFileName, txn, 0o600); err == nil {
		err = s.fail("after-prepare")
	}
	if err == nil && state.AppliedThemeID == id {
		state.Revision++
		state.AppliedThemeID = ""
		state.AppliedPackageDigest = ""
		err = writeRootJSON(root, stateFileName, state, 0o600)
	}
	if err == nil {
		err = s.fail("after-delete-state")
	}
	if err == nil {
		err = root.Rename(packRecordName(id), tombstone)
	}
	if err == nil {
		_ = fileutil.SyncParentDir(filepath.Join(s.root, packRecordName(id)))
		_ = fileutil.SyncParentDir(filepath.Join(s.root, tombstone))
		err = s.fail("after-delete-rename")
	}
	if err == nil {
		_ = root.Remove(tombstone)
		err = clearTransaction(root)
	}
	if err == nil {
		if s.previewID == id {
			s.previewID = ""
		}
		_ = s.gcAssetsLocked(root)
	}
	_ = root.Close()
	s.mu.Unlock()
	if err != nil {
		return Snapshot{}, err
	}
	return s.Snapshot(false)
}

// Asset returns verified immutable content for the Desktop asset middleware.
func (s *Store) Asset(digest string, safeMode bool) (AssetContent, error) {
	if safeMode {
		return AssetContent{}, os.ErrNotExist
	}
	if s == nil || !digestPattern.MatchString(digest) {
		return AssetContent{}, os.ErrNotExist
	}
	if asset, ok := officialAsset(digest); ok {
		return asset, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.prepareLocked()
	if err != nil {
		return AssetContent{}, err
	}
	defer root.Close()
	entries, err := readRootDir(root, filepath.Join("assets", "sha256"))
	if err != nil {
		return AssetContent{}, err
	}
	var name string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), digest+".") {
			if name != "" || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return AssetContent{}, fmt.Errorf("ambiguous or unsafe theme asset %s", digest)
			}
			name = entry.Name()
		}
	}
	if name == "" {
		return AssetContent{}, os.ErrNotExist
	}
	rel := filepath.Join("assets", "sha256", name)
	file, err := root.Open(rel)
	if err != nil {
		return AssetContent{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxImageBytes {
		return AssetContent{}, fmt.Errorf("unsafe theme asset %s", digest)
	}
	data, err := readLimited(file, MaxImageBytes, name)
	if err != nil {
		return AssetContent{}, err
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != digest {
		return AssetContent{}, fmt.Errorf("theme asset %s failed content-address verification", digest)
	}
	imageInfo, err := validateImage(data, name)
	if err != nil {
		return AssetContent{}, err
	}
	return AssetContent{Data: data, MIME: imageInfo.MIME, Name: name, ModTime: info.ModTime()}, nil
}

func (s *Store) prepareLocked() (*os.Root, error) {
	if s.root == "" || s.root == "." {
		return nil, fmt.Errorf("theme store root is unavailable")
	}
	for _, dir := range []string{s.root, filepath.Join(s.root, "packs"), filepath.Join(s.root, "assets", "sha256"), filepath.Join(s.root, "trash")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	if err := s.recoverLocked(root); err != nil {
		_ = root.Close()
		return nil, err
	}
	return root, nil
}

func (s *Store) recoverLocked(root *os.Root) error {
	data, err := readRootFileLimited(root, transactionFileName, MaxManifestBytes+4096)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var txn transaction
	if err := decodeStrict(data, &txn); err != nil {
		return fmt.Errorf("decode theme transaction: %w", err)
	}
	if txn.SchemaVersion != transactionVersion || !idPattern.MatchString(txn.ID) {
		return fmt.Errorf("invalid theme transaction journal")
	}
	switch txn.Operation {
	case "install":
		current, currentErr := readPack(root, txn.ID)
		if currentErr == nil && current.PackageDigest == txn.DesiredDigest {
			desiredState := txn.DesiredState
			if desiredState.SchemaVersion == 0 {
				desiredState = normalizeState(txn.PreviousState)
				if desiredState.AppliedThemeID == txn.ID {
					desiredState.Revision++
					desiredState.AppliedPackageDigest = txn.DesiredDigest
				}
			}
			if err := writeRootJSON(root, stateFileName, normalizeState(desiredState), 0o600); err != nil {
				return err
			}
			return clearTransaction(root)
		}
		if len(txn.PreviousPack) > 0 {
			var previous InstalledPack
			if err := decodeAndValidatePack(txn.PreviousPack, &previous); err != nil {
				return fmt.Errorf("recover previous theme pack: %w", err)
			}
			if err := fileutil.AtomicWriteRootFile(root, packRecordName(txn.ID), txn.PreviousPack, 0o600); err != nil {
				return err
			}
		} else if err := root.Remove(packRecordName(txn.ID)); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := writeRootJSON(root, stateFileName, normalizeState(txn.PreviousState), 0o600); err != nil {
			return err
		}
		return clearTransaction(root)
	case "delete":
		if err := root.Remove(packRecordName(txn.ID)); err != nil && !os.IsNotExist(err) {
			return err
		}
		if txn.Tombstone != "" {
			if !filepath.IsLocal(txn.Tombstone) || filepath.Dir(txn.Tombstone) != "trash" {
				return fmt.Errorf("invalid theme tombstone path")
			}
			if err := root.Remove(txn.Tombstone); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		state, err := readState(root)
		if err != nil {
			return err
		}
		if state.AppliedThemeID == txn.ID {
			state.Revision++
			state.AppliedThemeID = ""
			state.AppliedPackageDigest = ""
			if err := writeRootJSON(root, stateFileName, state, 0o600); err != nil {
				return err
			}
		}
		return clearTransaction(root)
	default:
		return fmt.Errorf("unknown theme transaction operation %q", txn.Operation)
	}
}

func (s *Store) writeAssetLocked(root *os.Root, asset candidateAsset) error {
	name := asset.Ref.SHA256 + asset.Info.Ext
	rel := filepath.Join("assets", "sha256", name)
	if existing, err := readRootFileLimited(root, rel, MaxImageBytes); err == nil {
		hash := sha256.Sum256(existing)
		if hex.EncodeToString(hash[:]) != asset.Ref.SHA256 {
			return fmt.Errorf("existing content-addressed asset %s is corrupt", asset.Ref.SHA256)
		}
		_, err = validateImage(existing, name)
		return err
	} else if !os.IsNotExist(err) {
		return err
	}
	return fileutil.AtomicWriteRootFile(root, rel, asset.Data, 0o600)
}

func (s *Store) gcAssetsLocked(root *os.Root) error {
	packs, _, err := listPacks(root)
	if err != nil {
		return err
	}
	referenced := map[string]struct{}{}
	for _, pack := range packs {
		for _, ref := range manifestAssetRefs(pack.Manifest) {
			referenced[ref.SHA256] = struct{}{}
		}
	}
	entries, err := readRootDir(root, filepath.Join("assets", "sha256"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		dot := strings.IndexByte(name, '.')
		if dot != 64 || !digestPattern.MatchString(name[:dot]) {
			continue
		}
		if _, ok := referenced[name[:dot]]; !ok {
			_ = root.Remove(filepath.Join("assets", "sha256", name))
		}
	}
	return nil
}

func listPacks(root *os.Root) ([]InstalledPack, []string, error) {
	entries, err := readRootDir(root, "packs")
	if err != nil {
		return nil, nil, err
	}
	packs := make([]InstalledPack, 0, len(entries))
	var warnings []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		pack, err := readPack(root, id)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("ignored invalid theme record %q: %v", entry.Name(), err))
			continue
		}
		validAssets := true
		for _, ref := range manifestAssetRefs(pack.Manifest) {
			if err := validateStoredAsset(root, ref.SHA256); err != nil {
				warnings = append(warnings, fmt.Sprintf("ignored theme %q with invalid asset %s: %v", id, ref.SHA256, err))
				validAssets = false
				break
			}
		}
		if !validAssets {
			continue
		}
		packs = append(packs, pack)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].Manifest.ID < packs[j].Manifest.ID })
	sort.Strings(warnings)
	if packs == nil {
		packs = []InstalledPack{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	return packs, warnings, nil
}

func validateStoredAsset(root *os.Root, digest string) error {
	entries, err := readRootDir(root, filepath.Join("assets", "sha256"))
	if err != nil {
		return err
	}
	name := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), digest+".") {
			if name != "" || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("ambiguous or unsafe content-addressed file")
			}
			name = entry.Name()
		}
	}
	if name == "" {
		return os.ErrNotExist
	}
	data, err := readRootFileLimited(root, filepath.Join("assets", "sha256", name), MaxImageBytes)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != digest {
		return fmt.Errorf("content digest mismatch")
	}
	_, err = validateImage(data, name)
	return err
}

func readPack(root *os.Root, id string) (InstalledPack, error) {
	if !idPattern.MatchString(id) || IsBaseStyle(id) || IsOfficialID(id) {
		return InstalledPack{}, fmt.Errorf("invalid user theme id %q", id)
	}
	data, err := readRootFileLimited(root, packRecordName(id), MaxManifestBytes+4096)
	if err != nil {
		return InstalledPack{}, err
	}
	var pack InstalledPack
	if err := decodeAndValidatePack(data, &pack); err != nil {
		return InstalledPack{}, err
	}
	if pack.Manifest.ID != id {
		return InstalledPack{}, fmt.Errorf("theme record id %q does not match file %q", pack.Manifest.ID, id)
	}
	return pack, nil
}

func decodeAndValidatePack(data []byte, pack *InstalledPack) error {
	if err := decodeStrict(data, pack); err != nil {
		return err
	}
	if err := ValidateManifest(&pack.Manifest); err != nil {
		return err
	}
	if !digestPattern.MatchString(pack.PackageDigest) {
		return fmt.Errorf("invalid package digest")
	}
	canonical, err := canonicalManifest(pack.Manifest)
	if err != nil {
		return err
	}
	if canonicalPackageDigest(canonical, manifestAssetRefs(pack.Manifest)) != pack.PackageDigest {
		return fmt.Errorf("package digest does not match canonical manifest")
	}
	if _, err := time.Parse(time.RFC3339Nano, pack.ImportedAt); err != nil {
		return fmt.Errorf("invalid importedAt: %w", err)
	}
	return nil
}

func readState(root *os.Root) (State, error) {
	data, err := readRootFileLimited(root, stateFileName, 16<<10)
	if os.IsNotExist(err) {
		return State{SchemaVersion: storeStateVersion}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := decodeStrict(data, &state); err != nil {
		return State{}, fmt.Errorf("decode theme state: %w", err)
	}
	if state.SchemaVersion != storeStateVersion {
		return State{}, fmt.Errorf("unsupported theme state version %d", state.SchemaVersion)
	}
	if state.AppliedThemeID == "" {
		state.AppliedPackageDigest = ""
		return state, nil
	}
	if !idPattern.MatchString(state.AppliedThemeID) || IsBaseStyle(state.AppliedThemeID) || !digestPattern.MatchString(state.AppliedPackageDigest) {
		return State{}, fmt.Errorf("invalid applied theme state")
	}
	return state, nil
}

func normalizeState(state State) State {
	state.SchemaVersion = storeStateVersion
	if state.AppliedThemeID == "" {
		state.AppliedPackageDigest = ""
	}
	return state
}

func writeRootJSON(root *os.Root, name string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileutil.AtomicWriteRootFile(root, name, data, mode)
}

func readRootFileLimited(root *os.Root, name string, max int64) ([]byte, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > max {
		return nil, fmt.Errorf("unsafe or oversized theme store file %q", name)
	}
	return readLimited(file, max, name)
}

func readRootDir(root *os.Root, name string) ([]os.DirEntry, error) {
	dir, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func clearTransaction(root *os.Root) error {
	err := root.Remove(transactionFileName)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return fileutil.SyncParentDir(filepath.Join(root.Name(), transactionFileName))
}

func packRecordName(id string) string { return filepath.Join("packs", id+".json") }

func canonicalPackageDigest(canonical []byte, refs []AssetRef) string {
	hash := sha256.New()
	_, _ = hash.Write(canonical)
	for _, ref := range refs {
		_, _ = io.WriteString(hash, "\x00"+ref.File+"\x00"+ref.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Store) fail(stage string) error {
	if s.failpoint == nil {
		return nil
	}
	if err := s.failpoint(stage); err != nil {
		return fmt.Errorf("theme transaction failpoint %s: %w", stage, err)
	}
	return nil
}
