package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i)
	}
	plain := []byte("hello, Reames Agent secret store")

	enc, err := Encrypt(plain, key)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decrypt(enc, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != string(plain) {
		t.Fatalf("round-trip: got %q, want %q", dec, plain)
	}
}

func TestDeriveKey(t *testing.T) {
	salt, _ := NewSalt()
	k1 := DeriveKey("password123", salt)
	k2 := DeriveKey("password123", salt)
	if !ConstantTimeEqual(k1, k2) {
		t.Fatal("same inputs should produce same key")
	}
}

func TestZeroize(t *testing.T) {
	buf := []byte("sensitive data")
	Zeroize(buf)
	for _, b := range buf {
		if b != 0 {
			t.Fatal("Zeroize incomplete")
		}
	}
}

func TestSecureWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := SecureWrite(path, []byte("test")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test" {
		t.Fatalf("read mismatch: %q", data)
	}
}

func TestEncryptDecryptShortInput(t *testing.T) {
	key := make([]byte, KeySize)
	enc, err := Encrypt([]byte("a"), key)
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := Decrypt(enc, key)
	if string(dec) != "a" {
		t.Fatalf("short round-trip failed: %q", dec)
	}
}

func TestDecryptCorruptData(t *testing.T) {
	key := make([]byte, KeySize)
	_, err := Decrypt("not-base64!!!", key)
	if err == nil {
		t.Fatal("expected error for non-base64")
	}
	_, err = Decrypt("AAAA", key)
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i)
	}
	enc, _ := Encrypt([]byte("secret"), key)
	wrong := make([]byte, KeySize)
	_, err := Decrypt(enc, wrong)
	if err == nil {
		t.Fatal("expected error with wrong key")
	}
}

func TestEncryptLargeInput(t *testing.T) {
	key := make([]byte, KeySize)
	large := make([]byte, 10000)
	for i := range large {
		large[i] = byte(i % 256)
	}
	enc, err := Encrypt(large, key)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decrypt(enc, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(dec) != len(large) {
		t.Fatalf("length mismatch: %d vs %d", len(dec), len(large))
	}
}

func TestNewSalt(t *testing.T) {
	s1, _ := NewSalt()
	s2, _ := NewSalt()
	if string(s1) == string(s2) {
		t.Fatal("salts should be unique")
	}
	if len(s1) != SaltSize {
		t.Fatalf("salt size: %d", len(s1))
	}
}
