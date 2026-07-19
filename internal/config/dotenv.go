package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/joho/godotenv"

	fileenc "reames-agent/internal/fileutil/encoding"
)

type dotEnvFile struct {
	Path       string
	Values     map[string]string
	Duplicates []string
}

// loadDotEnv loads Reames Agent's global .env for provider credentials. The
// workspace .env values returned by loadDotEnvForRoot are ignored here because
// loadDotEnv has no Config to carry a workspace-scoped expansion environment.
func loadDotEnv() {
	loadDotEnvForRoot(".")
}

// loadDotEnvForRoot returns workspace .env values for scoped plugin/MCP/proxy
// expansion, then loads Reames Agent's global .env for provider credentials.
// Workspace .env values are deliberately not written into the process
// environment, so multiple desktop/ACP workspaces cannot leak tokens into each
// other and project files cannot redirect Reames Agent's own config/credential
// paths.
func loadDotEnvForRoot(root string) map[string]string {
	projectEnv := loadProjectDotEnvForExpansion(root)
	loadCredentialStoreForRoot(root)
	return projectEnv
}

func loadProjectDotEnvForExpansion(root string) map[string]string {
	root = resolveRoot(root)
	path := ".env"
	if root != "." {
		path = filepath.Join(root, ".env")
	}
	if current := UserCredentialsPath(); current != "" && samePath(path, current) {
		return nil
	}
	file, ok := readDotEnvFile(path)
	if !ok {
		return nil
	}
	return file.filtered(func(key string) bool {
		return !isProjectDotEnvControlKey(key)
	})
}

func isProjectDotEnvControlKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}
	upper := strings.ToUpper(key)
	if strings.HasPrefix(upper, "REAMES_AGENT_") {
		return true
	}
	switch upper {
	case "HOME", "USERPROFILE", "APPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME":
		return true
	default:
		return false
	}
}

func legacyCredentialsPaths() []string {
	current := UserCredentialsPath()
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		if current != "" && samePath(path, current) {
			return
		}
		if seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if dir := legacyOSSupportDir(); dir != "" {
		add(filepath.Join(dir, "credentials"))
	}
	if dir := userSupportDir(); dir != "" {
		add(filepath.Join(dir, "credentials"))
		add(filepath.Join(dir, ".env"))
	}
	for _, cfg := range legacyXDGConfigPaths() {
		add(filepath.Join(filepath.Dir(cfg), "credentials"))
	}
	return paths
}

func loadDotEnvFileAs(path string, source CredentialSource) {
	file, ok := readDotEnvFile(path)
	if !ok {
		return
	}
	for key, val := range file.Values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists && source.Kind != CredentialSourceCredentials {
			recordExistingCredentialSource(key)
			continue
		}
		if err := os.Setenv(key, val); err == nil && source.Kind != "" {
			source.Path = path
			recordCredentialSource(key, val, source)
		}
	}
}

func readDotEnvFile(path string) (dotEnvFile, bool) {
	data, err := readDotEnvText(path)
	if err != nil {
		return dotEnvFile{}, false
	}
	values, err := godotenv.Unmarshal(string(data))
	if err != nil {
		return dotEnvFile{}, false
	}
	return dotEnvFile{
		Path:       path,
		Values:     values,
		Duplicates: detectDotEnvDuplicateKeysIn(data),
	}, true
}

// readDotEnvText decodes supported text encodings without modifying the file.
// Windows Notepad commonly writes UTF-16, including for credential files that
// users edit by hand. UTF-32 and malformed/binary input fail closed so a later
// credential save cannot rewrite already-corrupted bytes into a different
// corruption. Reames-owned credential files are normalized to UTF-8 only after
// a successful atomic write.
func readDotEnvText(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if hasUTF32BOM(raw) {
		return nil, fmt.Errorf("unsupported UTF-32 .env encoding")
	}
	enc, _ := fileenc.Detect(raw)
	if enc == fileenc.LossyUTF8 {
		return nil, fmt.Errorf("invalid .env text encoding")
	}
	switch enc {
	case fileenc.UTF16LE, fileenc.UTF16BE:
		if (len(raw)-2)%2 != 0 {
			return nil, fmt.Errorf("truncated UTF-16 .env encoding")
		}
	case fileenc.UTF16LENoBOM, fileenc.UTF16BENoBOM:
		if len(raw)%2 != 0 {
			return nil, fmt.Errorf("truncated UTF-16 .env encoding")
		}
	}
	decoded := fileenc.Decode(raw, enc)
	if !utf8.Valid(decoded) || bytes.IndexByte(decoded, 0) >= 0 {
		return nil, fmt.Errorf("invalid .env text content")
	}
	return decoded, nil
}

func hasUTF32BOM(data []byte) bool {
	return len(data) >= 4 &&
		(bytes.Equal(data[:4], []byte{0x00, 0x00, 0xFE, 0xFF}) ||
			bytes.Equal(data[:4], []byte{0xFF, 0xFE, 0x00, 0x00}))
}

func (f dotEnvFile) filtered(allow func(string) bool) map[string]string {
	out := map[string]string{}
	for key, val := range f.Values {
		key = strings.TrimSpace(key)
		if key == "" || allow != nil && !allow(key) {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (f dotEnvFile) warnings() []string {
	if len(f.Duplicates) == 0 {
		return nil
	}
	warnings := make([]string, 0, len(f.Duplicates))
	for _, key := range f.Duplicates {
		warnings = append(warnings, "duplicate .env key "+key+" in "+f.Path+"; last parsed value wins")
	}
	return warnings
}

func detectDotEnvDuplicateKeys(path string) []string {
	raw, err := readDotEnvText(path)
	if err != nil {
		return nil
	}
	return detectDotEnvDuplicateKeysIn(raw)
}

func detectDotEnvDuplicateKeysIn(raw []byte) []string {
	seen := map[string]bool{}
	dups := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		values, err := godotenv.Unmarshal(line)
		if err != nil {
			continue
		}
		for key := range values {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if seen[key] {
				dups[key] = true
			}
			seen[key] = true
		}
	}
	out := make([]string, 0, len(dups))
	for key := range dups {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func envFileValue(path, wantKey string) (string, bool) {
	file, ok := readDotEnvFile(path)
	if !ok {
		return "", false
	}
	val, ok := file.Values[wantKey]
	return val, ok
}
