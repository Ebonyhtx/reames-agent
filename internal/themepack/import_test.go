package themepack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash/crc32"
	"os"
	"strings"
	"testing"
)

type zipFixtureFile struct {
	name string
	data []byte
	mode os.FileMode
}

func themeArchive(t *testing.T, manifest Manifest, files ...zipFixtureFile) []byte {
	t.Helper()
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	all := append([]zipFixtureFile{{name: ManifestName, data: manifestData, mode: 0o644}}, files...)
	return zipBytes(t, all...)
}

func zipBytes(t *testing.T, files ...zipFixtureFile) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		if file.mode == 0 {
			file.mode = 0o644
		}
		header.SetMode(file.mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func manifestWithPNG(t *testing.T, name string, png []byte) Manifest {
	t.Helper()
	manifest := validManifest()
	digest := sha256.Sum256(png)
	manifest.Scenes.Home = &Scene{
		Image:  AssetRef{File: name, SHA256: hex.EncodeToString(digest[:])},
		FocusX: .5, FocusY: .5, SafeArea: "center", Opacity: 1, OverlayStrength: .55,
	}
	return manifest
}

func tinyPNG() []byte { return pngConfigFixture(2, 2) }

func pngConfigFixture(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	writeChunk := func(kind string, data []byte) {
		_ = binary.Write(&out, binary.BigEndian, uint32(len(data)))
		out.WriteString(kind)
		out.Write(data)
		crc := crc32.NewIEEE()
		_, _ = crc.Write([]byte(kind))
		_, _ = crc.Write(data)
		_ = binary.Write(&out, binary.BigEndian, crc.Sum32())
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2
	writeChunk("IHDR", ihdr)
	writeChunk("IEND", nil)
	return out.Bytes()
}

func TestInspectArchiveAcceptsControlledPackage(t *testing.T) {
	png := tinyPNG()
	manifest := manifestWithPNG(t, "scene.png", png)
	candidate, err := inspectArchiveBytes(themeArchive(t, manifest, zipFixtureFile{name: "scene.png", data: png}))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Manifest.ID != manifest.ID || !digestPattern.MatchString(candidate.PackageDigest) || len(candidate.assets) != 1 {
		t.Fatalf("candidate = %+v assets=%d", candidate, len(candidate.assets))
	}
}

func TestInspectArchiveRejectsUnsafeEntries(t *testing.T) {
	manifestData, _ := json.Marshal(validManifest())
	tests := []struct {
		name  string
		files []zipFixtureFile
		want  string
	}{
		{"traversal", []zipFixtureFile{{name: "../theme.json", data: manifestData}}, "root-level"},
		{"nested", []zipFixtureFile{{name: "dir/theme.json", data: manifestData}}, "root-level"},
		{"windows separator", []zipFixtureFile{{name: `dir\theme.json`, data: manifestData}}, "root-level"},
		{"windows drive path", []zipFixtureFile{{name: `C:\theme.json`, data: manifestData}}, "root-level"},
		{"symlink", []zipFixtureFile{{name: ManifestName, data: manifestData, mode: os.ModeSymlink | 0o777}}, "non-symlink"},
		{"windows device", []zipFixtureFile{{name: ManifestName, data: manifestData}, {name: "CON.png", data: tinyPNG()}}, "Windows"},
		{"duplicate case", []zipFixtureFile{{name: ManifestName, data: manifestData}, {name: "THEME.JSON", data: manifestData}}, "duplicate"},
		{"extra file", []zipFixtureFile{{name: ManifestName, data: manifestData}, {name: "README.txt", data: []byte("no")}}, "disallowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := inspectArchiveBytes(zipBytes(t, tt.files...)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("inspectArchiveBytes error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestInspectArchiveRejectsDigestMismatchAndPixelBomb(t *testing.T) {
	png := tinyPNG()
	manifest := manifestWithPNG(t, "scene.png", png)
	manifest.Scenes.Home.Image.SHA256 = strings.Repeat("0", 64)
	if _, err := inspectArchiveBytes(themeArchive(t, manifest, zipFixtureFile{name: "scene.png", data: png})); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("digest mismatch error = %v", err)
	}

	large := pngConfigFixture(6000, 5000)
	manifest = manifestWithPNG(t, "scene.png", large)
	if _, err := inspectArchiveBytes(themeArchive(t, manifest, zipFixtureFile{name: "scene.png", data: large})); err == nil || !strings.Contains(err.Error(), "pixels") {
		t.Fatalf("pixel bomb error = %v", err)
	}
}

func TestInspectArchiveRejectsCompressionBombAndEntryCount(t *testing.T) {
	manifestData, _ := json.Marshal(validManifest())
	bomb := bytes.Repeat([]byte{0}, (1<<20)+1)
	if _, err := inspectArchiveBytes(zipBytes(t,
		zipFixtureFile{name: ManifestName, data: manifestData},
		zipFixtureFile{name: "scene.png", data: bomb},
	)); err == nil || !strings.Contains(err.Error(), "compression ratio") {
		t.Fatalf("compression bomb error = %v", err)
	}
	if _, err := inspectArchiveBytes(zipBytes(t,
		zipFixtureFile{name: ManifestName, data: manifestData},
		zipFixtureFile{name: "a.png", data: tinyPNG()},
		zipFixtureFile{name: "b.png", data: tinyPNG()},
		zipFixtureFile{name: "c.png", data: tinyPNG()},
	)); err == nil || !strings.Contains(err.Error(), "1-3") {
		t.Fatalf("entry count error = %v", err)
	}
}
