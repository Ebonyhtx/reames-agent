package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSaveExportFileWritesTextAndBinaryPayloads(t *testing.T) {
	t.Parallel()
	app := &App{}
	dir := t.TempDir()

	textPath := filepath.Join(dir, "session.md")
	if err := app.SaveExportFile(textPath, "# session\n", false); err != nil {
		t.Fatalf("save text export: %v", err)
	}
	if text, err := os.ReadFile(textPath); err != nil || string(text) != "# session\n" {
		t.Fatalf("text export = %q, err = %v", text, err)
	}

	binaryPath := filepath.Join(dir, "session.png")
	binary := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0xff}
	if err := app.SaveExportFile(binaryPath, base64.StdEncoding.EncodeToString(binary), true); err != nil {
		t.Fatalf("save binary export: %v", err)
	}
	if written, err := os.ReadFile(binaryPath); err != nil || !bytes.Equal(written, binary) {
		t.Fatalf("binary export = %v, err = %v", written, err)
	}
}

func TestSaveExportFileRejectsInvalidBase64(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "broken.pdf")
	if err := (&App{}).SaveExportFile(path, "not base64!", true); err == nil {
		t.Fatal("expected invalid base64 error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid payload should not create a file, stat error = %v", err)
	}
}

func TestExportErrorsDoNotExposeSelectedDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missingDir := filepath.Join(dir, "private-export-directory")
	payload := base64.StdEncoding.EncodeToString([]byte("image"))
	tests := []struct {
		name string
		path string
		run  func(string) error
	}{
		{name: "single file", path: filepath.Join(missingDir, "session.pdf"), run: func(path string) error {
			return (&App{}).SaveExportFile(path, payload, true)
		}},
		{name: "multipart image", path: filepath.Join(missingDir, "session.png"), run: func(path string) error {
			return (&App{}).SaveExportImageFiles(path, []string{payload, payload})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run(test.path)
			if err == nil {
				t.Fatal("expected missing export directory to fail")
			}
			if strings.Contains(err.Error(), dir) {
				t.Fatalf("export error exposed selected directory: %q", err)
			}
			if !strings.Contains(err.Error(), "session") {
				t.Fatalf("export error should retain a safe file name: %q", err)
			}
		})
	}
}

func TestSaveExportImageFilesWritesNumberedParts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.archive.png")
	payloads := [][]byte{{0x01, 0x02}, {0x03, 0x04}, {0x05, 0x06}}
	encoded := make([]string, len(payloads))
	for i, payload := range payloads {
		encoded[i] = base64.StdEncoding.EncodeToString(payload)
	}

	if err := (&App{}).SaveExportImageFiles(path, encoded); err != nil {
		t.Fatalf("save image parts: %v", err)
	}
	for i, want := range payloads {
		partPath := filepath.Join(dir, fmt.Sprintf("session.archive-%d-of-3.png", i+1))
		got, err := os.ReadFile(partPath)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("image part %d = %v, err = %v", i+1, got, err)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("multi-part export should not write the selected base path, stat error = %v", err)
	}
	assertNoExportTemps(t, dir)
}

func TestSaveExportImageFilesRejectsCollisionWithoutPartialOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.png")
	collisionPath := filepath.Join(dir, "session-2-of-3.png")
	if err := os.WriteFile(collisionPath, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte("new image"))

	if err := (&App{}).SaveExportImageFiles(path, []string{payload, payload, payload}); err == nil {
		t.Fatal("expected existing numbered export to reject the batch")
	}
	if got, err := os.ReadFile(collisionPath); err != nil || string(got) != "keep me" {
		t.Fatalf("existing image part changed: data=%q err=%v", got, err)
	}
	for _, name := range []string{"session-1-of-3.png", "session-3-of-3.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("collision left partial output %s, stat error = %v", name, err)
		}
	}
}

func TestSaveExportImageFilesDecodesAllPartsBeforeWriting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.png")
	valid := base64.StdEncoding.EncodeToString([]byte("image"))

	if err := (&App{}).SaveExportImageFiles(path, []string{valid, "not base64!", valid}); err == nil {
		t.Fatal("expected invalid image payload to reject the batch")
	}
	for i := 1; i <= 3; i++ {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("session-%d-of-3.png", i))); !os.IsNotExist(err) {
			t.Fatalf("invalid payload left image part %d, stat error = %v", i, err)
		}
	}
	assertNoExportTemps(t, dir)
}

func TestSaveExclusiveExportFilesRollsBackCommittedTargets(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "duplicate.png")
	if err := saveExclusiveExportFiles([]string{target, target}, [][]byte{[]byte("first"), []byte("second")}); err == nil {
		t.Fatal("expected duplicate exclusive target to fail")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("failed batch should roll back its committed target, stat error = %v", err)
	}
}

func TestRollbackDoesNotRemoveReplacedExportTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tempPath := filepath.Join(dir, ".staged.png")
	targetPath := filepath.Join(dir, "session.png")
	if err := os.WriteFile(tempPath, []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err := commitStagedExportFile(tempPath, targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}

	rollbackCommittedExportFiles([]committedExportFile{{path: targetPath, info: created}})
	if got, err := os.ReadFile(targetPath); err != nil || string(got) != "replacement" {
		t.Fatalf("rollback removed replacement: data=%q err=%v", got, err)
	}
}

func TestConcurrentMultipartExportsHaveSingleCompleteWinner(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.png")
	encode := func(prefix string) []string {
		out := make([]string, 3)
		for i := range out {
			out[i] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%d", prefix, i+1)))
		}
		return out
	}
	batches := [][]string{encode("a"), encode("b")}
	start := make(chan struct{})
	errs := make(chan error, len(batches))
	var ready sync.WaitGroup
	ready.Add(len(batches))
	for _, batch := range batches {
		go func(batch []string) {
			ready.Done()
			<-start
			errs <- (&App{}).SaveExportImageFiles(path, batch)
		}(batch)
	}
	ready.Wait()
	close(start)

	successes := 0
	for range batches {
		if err := <-errs; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent exports = %d, want exactly one", successes)
	}
	first, err := os.ReadFile(filepath.Join(dir, "session-1-of-3.png"))
	if err != nil {
		t.Fatal(err)
	}
	winner := string(first[:1])
	for i := 1; i <= 3; i++ {
		got, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("session-%d-of-3.png", i)))
		if err != nil || string(got) != fmt.Sprintf("%s-%d", winner, i) {
			t.Fatalf("winning part %d = %q, err = %v", i, got, err)
		}
	}
	assertNoExportTemps(t, dir)
}

func assertNoExportTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".reames-agent-export-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staged export files remain: matches=%v err=%v", matches, err)
	}
}
