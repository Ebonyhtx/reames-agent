package installsource

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/pluginregistry"
)

func TestRegistryGitSourceEvidenceIgnoresWorkingTreeAndBindsGitMode(t *testing.T) {
	repo := initRegistryGitRepository(t)
	writeRegistryGitFile(t, repo, "script.sh", "#!/bin/sh\nprintf 'ok\\n'\n")
	runRegistryTestGit(t, repo, "add", "script.sh")
	runRegistryTestGit(t, repo, "update-index", "--chmod=+x", "script.sh")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "executable script")

	revision, digest, err := RegistryGitSourceEvidence(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(revision) != 40 || !strings.HasPrefix(digest, pluginregistry.GitTreeDigestPrefix) {
		t.Fatalf("revision=%q digest=%q", revision, digest)
	}

	writeRegistryGitFile(t, repo, "script.sh", "changed\r\nworking tree\r\n")
	if err := os.Chmod(filepath.Join(repo, "script.sh"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, unchanged, err := RegistryGitSourceEvidence(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != digest {
		t.Fatalf("working-tree mutation changed canonical Git digest: %s -> %s", digest, unchanged)
	}

	runRegistryTestGit(t, repo, "update-index", "--chmod=-x", "script.sh")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "remove executable intent")
	_, modeDigest, err := RegistryGitSourceEvidence(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if modeDigest == digest {
		t.Fatal("Git executable-bit change did not change canonical digest")
	}
}

func TestRegistryGitSourceEvidenceSubpathIgnoresOutsideChanges(t *testing.T) {
	repo := initRegistryGitRepository(t)
	writeRegistryGitFile(t, repo, "README.md", "one\n")
	writeRegistryGitFile(t, repo, "plugin/reames-plugin.json", `{"schemaVersion":1}`+"\n")
	runRegistryTestGit(t, repo, "add", ".")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "initial")
	_, first, err := RegistryGitSourceEvidence(context.Background(), repo, "plugin")
	if err != nil {
		t.Fatal(err)
	}

	writeRegistryGitFile(t, repo, "README.md", "two\n")
	runRegistryTestGit(t, repo, "add", "README.md")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "outside change")
	_, second, err := RegistryGitSourceEvidence(context.Background(), repo, "plugin")
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("outside-subpath change changed digest: %s -> %s", first, second)
	}
}

func TestRegistryGitSourceEvidenceUsesLiteralSubpath(t *testing.T) {
	repo := initRegistryGitRepository(t)
	writeRegistryGitFile(t, repo, "[plugin]/reames-plugin.json", `{"schemaVersion":1}`+"\n")
	writeRegistryGitFile(t, repo, "p/reames-plugin.json", "different\n")
	runRegistryTestGit(t, repo, "add", ".")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "literal pathspec")
	if _, digest, err := RegistryGitSourceEvidence(context.Background(), repo, "[plugin]"); err != nil {
		t.Fatal(err)
	} else if !strings.HasPrefix(digest, pluginregistry.GitTreeDigestPrefix) {
		t.Fatalf("literal subpath digest = %q", digest)
	}
}

func TestRegistryGitSourceEvidenceRejectsGitSymlink(t *testing.T) {
	repo := initRegistryGitRepository(t)
	writeRegistryGitFile(t, repo, "regular.txt", "target\n")
	runRegistryTestGit(t, repo, "add", "regular.txt")
	oid := strings.TrimSpace(runRegistryTestGit(t, repo, "hash-object", "-w", "regular.txt"))
	runRegistryTestGit(t, repo, "update-index", "--add", "--cacheinfo", "120000", oid, "link")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "symlink entry")
	if _, _, err := RegistryGitSourceEvidence(context.Background(), repo, ""); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Git symlink digest err = %v, want unsupported entry", err)
	}
}

func TestSecureGitCommandIgnoresAmbientAutoCRLF(t *testing.T) {
	repo := initRegistryGitRepository(t)
	writeRegistryGitFile(t, repo, "text.txt", "line one\nline two\n")
	runRegistryTestGit(t, repo, "add", "text.txt")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "text")

	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(globalConfig, []byte("[core]\n\tautocrlf = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	destination := filepath.Join(t.TempDir(), "checkout")
	if out, err := secureGitCommand(context.Background(), "clone", "--quiet", repo, destination).CombinedOutput(); err != nil {
		t.Fatalf("secure clone: %v: %s", err, out)
	}
	body, err := os.ReadFile(filepath.Join(destination, "text.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "\r\n") || string(body) != "line one\nline two\n" {
		t.Fatalf("secure checkout bytes = %q, want canonical LF blob", body)
	}
}

func TestRegistryGitSourceEvidenceIgnoresReplaceObjects(t *testing.T) {
	repo := initRegistryGitRepository(t)
	writeRegistryGitFile(t, repo, "content.txt", "first\n")
	runRegistryTestGit(t, repo, "add", "content.txt")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "first")
	firstRevision := strings.TrimSpace(runRegistryTestGit(t, repo, "rev-parse", "HEAD"))
	writeRegistryGitFile(t, repo, "content.txt", "second\n")
	runRegistryTestGit(t, repo, "add", "content.txt")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "second")
	secondRevision := strings.TrimSpace(runRegistryTestGit(t, repo, "rev-parse", "HEAD"))
	runRegistryTestGit(t, repo, "replace", firstRevision, secondRevision)
	runRegistryTestGit(t, repo, "checkout", "--quiet", "--detach", firstRevision)
	_, withReplaceRef, err := RegistryGitSourceEvidence(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	runRegistryTestGit(t, repo, "replace", "-d", firstRevision)
	_, withoutReplaceRef, err := RegistryGitSourceEvidence(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if withReplaceRef != withoutReplaceRef {
		t.Fatalf("replace ref changed canonical digest: %s -> %s", withoutReplaceRef, withReplaceRef)
	}
}

func TestValidatePortableRegistryGitPath(t *testing.T) {
	for _, valid := range []string{"reames-plugin.json", "skills/demo/SKILL.md", "资料/说明.md"} {
		if err := validatePortableRegistryGitPath(valid); err != nil {
			t.Errorf("valid path %q: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "../escape", "bad\\path", "bad\nname", "aux.txt", "nested/trailing. ", "a:b", "cafe\u0301/file"} {
		if err := validatePortableRegistryGitPath(invalid); err == nil {
			t.Errorf("invalid path %q accepted", invalid)
		}
	}
}

func TestParseRegistryGitTreeRejectsUnicodeCaseFoldCollision(t *testing.T) {
	oid := strings.Repeat("a", 40)
	tree := []byte("100644 blob " + oid + "\ts.txt\x00" + "100644 blob " + oid + "\tſ.txt\x00")
	if _, err := parseRegistryGitTree(tree, ""); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("Unicode case-fold collision err = %v", err)
	}
}

func TestRegistryGitSourceEvidenceRejectsNonPortableSubpath(t *testing.T) {
	repo := initRegistryGitRepository(t)
	writeRegistryGitFile(t, repo, "plugin.txt", "content\n")
	runRegistryTestGit(t, repo, "add", ".")
	runRegistryTestGit(t, repo, "commit", "--quiet", "-m", "portable fixture")
	if _, _, err := RegistryGitSourceEvidence(context.Background(), repo, "aux"); err == nil || !strings.Contains(err.Error(), "portable") {
		t.Fatalf("reserved subpath err = %v", err)
	}
}

func initRegistryGitRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runRegistryTestGit(t, repo, "init", "--quiet")
	runRegistryTestGit(t, repo, "config", "user.name", "Registry Test")
	runRegistryTestGit(t, repo, "config", "user.email", "registry-test@example.invalid")
	return repo
}

func writeRegistryGitFile(t *testing.T, repo, relative, content string) {
	t.Helper()
	name := filepath.Join(repo, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRegistryTestGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
