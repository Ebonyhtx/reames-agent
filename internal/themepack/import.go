package themepack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Candidate is a fully verified package ready to enter a Store. Asset bytes are
// deliberately private so callers cannot bypass validation before installation.
type Candidate struct {
	Manifest      Manifest `json:"manifest"`
	PackageDigest string   `json:"packageDigest"`
	assets        map[string]candidateAsset
}

type candidateAsset struct {
	Ref  AssetRef
	Data []byte
	Info imageInfo
}

// InspectArchive reads and verifies a package without mutating the local store.
func InspectArchive(path string) (Candidate, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Candidate{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Candidate{}, fmt.Errorf("theme package must be a regular non-symlink file")
	}
	if info.Size() <= 0 || info.Size() > MaxArchiveBytes {
		return Candidate{}, fmt.Errorf("theme package size %d is outside the allowed range", info.Size())
	}
	file, err := os.Open(path)
	if err != nil {
		return Candidate{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return Candidate{}, err
	}
	if opened.Size() != info.Size() || opened.Size() > MaxArchiveBytes {
		return Candidate{}, fmt.Errorf("theme package changed while opening")
	}
	reader, err := zip.NewReader(file, opened.Size())
	if err != nil {
		return Candidate{}, fmt.Errorf("open theme ZIP: %w", err)
	}
	return inspectZip(reader)
}

func inspectArchiveBytes(data []byte) (Candidate, error) {
	if len(data) == 0 || len(data) > MaxArchiveBytes {
		return Candidate{}, fmt.Errorf("theme package size is outside the allowed range")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Candidate{}, fmt.Errorf("open theme ZIP: %w", err)
	}
	return inspectZip(reader)
}

func inspectZip(reader *zip.Reader) (Candidate, error) {
	if len(reader.File) == 0 || len(reader.File) > MaxArchiveEntries {
		return Candidate{}, fmt.Errorf("theme package must contain 1-%d root files", MaxArchiveEntries)
	}
	seen := map[string]struct{}{}
	images := map[string]*zip.File{}
	var manifestEntry *zip.File
	var expanded uint64
	for _, entry := range reader.File {
		name, err := validateArchiveEntry(entry)
		if err != nil {
			return Candidate{}, err
		}
		folded := strings.ToLower(name)
		if _, exists := seen[folded]; exists {
			return Candidate{}, fmt.Errorf("theme package contains duplicate case-folded entry %q", name)
		}
		seen[folded] = struct{}{}
		expanded += entry.UncompressedSize64
		if expanded > MaxExpandedBytes {
			return Candidate{}, fmt.Errorf("theme package expands beyond %d bytes", MaxExpandedBytes)
		}
		if name == ManifestName {
			if entry.UncompressedSize64 > MaxManifestBytes {
				return Candidate{}, fmt.Errorf("theme manifest exceeds %d bytes", MaxManifestBytes)
			}
			manifestEntry = entry
			continue
		}
		if !imagePattern.MatchString(name) {
			return Candidate{}, fmt.Errorf("theme package contains disallowed file %q", name)
		}
		if entry.UncompressedSize64 > MaxImageBytes {
			return Candidate{}, fmt.Errorf("theme image %q exceeds %d bytes", name, MaxImageBytes)
		}
		images[folded] = entry
	}
	if manifestEntry == nil {
		return Candidate{}, fmt.Errorf("theme package is missing exact root file %s", ManifestName)
	}
	manifestReader, err := manifestEntry.Open()
	if err != nil {
		return Candidate{}, err
	}
	manifestData, readErr := readLimited(manifestReader, MaxManifestBytes, ManifestName)
	closeErr := manifestReader.Close()
	if readErr != nil {
		return Candidate{}, readErr
	}
	if closeErr != nil {
		return Candidate{}, closeErr
	}
	manifest, err := DecodeManifest(manifestData)
	if err != nil {
		return Candidate{}, err
	}
	refs := manifestAssetRefs(manifest)
	if len(images) != len(refs) {
		return Candidate{}, fmt.Errorf("theme package images do not exactly match manifest scene references")
	}
	assets := make(map[string]candidateAsset, len(refs))
	for _, ref := range refs {
		entry := images[strings.ToLower(ref.File)]
		if entry == nil || entry.Name != ref.File {
			return Candidate{}, fmt.Errorf("manifest image %q is missing with exact case", ref.File)
		}
		r, err := entry.Open()
		if err != nil {
			return Candidate{}, err
		}
		data, readErr := readLimited(r, MaxImageBytes, ref.File)
		closeErr := r.Close()
		if readErr != nil {
			return Candidate{}, readErr
		}
		if closeErr != nil {
			return Candidate{}, closeErr
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != ref.SHA256 {
			return Candidate{}, fmt.Errorf("image %q digest mismatch: got %s", ref.File, got)
		}
		imageInfo, err := validateImage(data, ref.File)
		if err != nil {
			return Candidate{}, err
		}
		assets[strings.ToLower(ref.File)] = candidateAsset{Ref: ref, Data: data, Info: imageInfo}
	}
	canonical, err := canonicalManifest(manifest)
	if err != nil {
		return Candidate{}, err
	}
	packageHash := sha256.New()
	_, _ = packageHash.Write(canonical)
	for _, ref := range refs {
		_, _ = io.WriteString(packageHash, "\x00"+ref.File+"\x00"+ref.SHA256)
	}
	return Candidate{
		Manifest:      manifest,
		PackageDigest: hex.EncodeToString(packageHash.Sum(nil)),
		assets:        assets,
	}, nil
}

func validateArchiveEntry(entry *zip.File) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("theme package contains a nil ZIP entry")
	}
	name := entry.Name
	if name == "" || name != strings.TrimSpace(name) || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("theme package contains an empty or padded entry name")
	}
	if isWindowsDeviceName(name) {
		return "", fmt.Errorf("theme package entry %q is reserved by Windows", name)
	}
	if strings.Contains(name, `\`) || strings.Contains(name, "/") || strings.Contains(name, ":") || filepath.IsAbs(name) || !filepath.IsLocal(name) {
		return "", fmt.Errorf("theme package entry %q is not a root-level local file", name)
	}
	if name == "." || name == ".." || strings.Contains(name, "..") && !imagePattern.MatchString(name) {
		return "", fmt.Errorf("theme package entry %q has an unsafe path", name)
	}
	mode := entry.Mode()
	if entry.FileInfo().IsDir() || mode&os.ModeSymlink != 0 || !mode.IsRegular() {
		return "", fmt.Errorf("theme package entry %q must be a regular non-symlink file", name)
	}
	if entry.Flags&0x1 != 0 {
		return "", fmt.Errorf("theme package entry %q must not be encrypted", name)
	}
	if entry.UncompressedSize64 > 0 && entry.CompressedSize64 == 0 {
		return "", fmt.Errorf("theme package entry %q has an invalid compression ratio", name)
	}
	if entry.UncompressedSize64 > 1<<20 && entry.UncompressedSize64 > entry.CompressedSize64*MaxCompressionRatio {
		return "", fmt.Errorf("theme package entry %q exceeds compression ratio %d:1", name, MaxCompressionRatio)
	}
	return name, nil
}

func canonicalManifest(manifest Manifest) ([]byte, error) {
	return canonicalManifestWithPolicy(manifest, false)
}

func canonicalOfficialManifest(manifest Manifest) ([]byte, error) {
	return canonicalManifestWithPolicy(manifest, true)
}

func canonicalManifestWithPolicy(manifest Manifest, allowOfficialID bool) ([]byte, error) {
	if err := validateManifest(&manifest, allowOfficialID); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode canonical theme manifest: %w", err)
	}
	if len(data) > MaxManifestBytes {
		return nil, fmt.Errorf("canonical theme manifest exceeds %d bytes", MaxManifestBytes)
	}
	return append(data, '\n'), nil
}

func sortedCandidateDigests(candidate Candidate) []string {
	keys := make([]string, 0, len(candidate.assets))
	for digest := range candidate.assets {
		keys = append(keys, digest)
	}
	sort.Strings(keys)
	return keys
}
