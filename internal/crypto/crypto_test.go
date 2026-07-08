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
