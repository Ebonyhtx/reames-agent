package config

import (
	"os"
	"testing"
)

func TestEncryptedCredentialRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", dir)
	os.MkdirAll(dir, 0o700)

	if err := WriteEncrypted("TEST_KEY", "secret-value-123"); err != nil {
		t.Fatal(err)
	}
	val, ok := ReadEncrypted("TEST_KEY")
	if !ok {
		t.Fatal("key not found")
	}
	if val != "secret-value-123" {
		t.Fatalf("got %q, want secret-value-123", val)
	}
}

func TestEncryptedCredentialNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", dir)

	_, ok := ReadEncrypted("NONEXISTENT")
	if ok {
		t.Fatal("should not find nonexistent key")
	}
}

func TestEncryptedCredentialOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", dir)
	os.MkdirAll(dir, 0o700)

	WriteEncrypted("KEY", "first")
	WriteEncrypted("KEY", "second")
	val, _ := ReadEncrypted("KEY")
	if val != "second" {
		t.Fatalf("got %q, want second", val)
	}
}

func TestEncryptedCredentialMultipleKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", dir)
	os.MkdirAll(dir, 0o700)

	WriteEncrypted("A", "1")
	WriteEncrypted("B", "2")
	a, _ := ReadEncrypted("A")
	b, _ := ReadEncrypted("B")
	if a != "1" || b != "2" {
		t.Fatalf("a=%q b=%q", a, b)
	}
}
