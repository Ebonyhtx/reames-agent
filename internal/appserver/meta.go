package appserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reames-agent/internal/fileutil"
	"reames-agent/internal/store"
)

const appServerMetaVersion = 2

type appServerMeta struct {
	Version          int    `json:"version"`
	ThreadID         string `json:"threadId"`
	OriginTranscript string `json:"originTranscript"`
	ActiveTranscript string `json:"activeTranscript"`
	ForkedFromID     string `json:"forkedFromId,omitempty"`
}

func appServerMetaPath(sessionPath string) string {
	if strings.TrimSpace(sessionPath) == "" {
		return ""
	}
	return store.SessionAppServerMeta(sessionPath)
}

func loadAppServerMeta(sessionPath string) (appServerMeta, bool, error) {
	path := appServerMetaPath(sessionPath)
	if path == "" {
		return appServerMeta{}, false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return appServerMeta{}, false, nil
		}
		return appServerMeta{}, false, err
	}
	var meta appServerMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return appServerMeta{}, false, fmt.Errorf("decode App-Server metadata %s: %w", path, err)
	}
	if meta.Version != 1 && meta.Version != appServerMetaVersion {
		return appServerMeta{}, false, fmt.Errorf("unsupported App-Server metadata version %d", meta.Version)
	}
	if strings.TrimSpace(meta.ThreadID) == "" || !safeTranscriptBase(meta.OriginTranscript) || !safeTranscriptBase(meta.ActiveTranscript) {
		return appServerMeta{}, false, fmt.Errorf("invalid App-Server metadata %s", path)
	}
	return meta, true, nil
}

func saveAppServerMeta(sessionPath string, meta appServerMeta) error {
	path := appServerMetaPath(sessionPath)
	if path == "" {
		return fmt.Errorf("empty App-Server metadata path")
	}
	meta.Version = appServerMetaVersion
	if strings.TrimSpace(meta.ThreadID) == "" || !safeTranscriptBase(meta.OriginTranscript) || !safeTranscriptBase(meta.ActiveTranscript) {
		return fmt.Errorf("invalid App-Server metadata")
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return fileutil.AtomicWriteFile(path, raw, 0o600)
}

func safeTranscriptBase(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && filepath.Base(name) == name && name != "." && name != ".."
}

func activePathFromMeta(dir string, meta appServerMeta) (string, bool) {
	if strings.TrimSpace(dir) == "" || !safeTranscriptBase(meta.ActiveTranscript) {
		return "", false
	}
	path := filepath.Join(dir, meta.ActiveTranscript)
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return path, true
}

type metaSnapshot struct {
	raw     []byte
	existed bool
}

func snapshotMeta(path string) (metaSnapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return metaSnapshot{}, nil
		}
		return metaSnapshot{}, err
	}
	return metaSnapshot{raw: raw, existed: true}, nil
}
func restoreMeta(path string, snapshot metaSnapshot) error {
	if snapshot.existed {
		return fileutil.AtomicWriteFile(path, snapshot.raw, 0o600)
	}
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
