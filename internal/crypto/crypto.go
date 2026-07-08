// Package crypto provides secure key storage primitives: zeroizable buffers,
// AES-256-GCM encryption, and Argon2id key derivation.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

// KeySize is the AES-256 key length (32 bytes).
const KeySize = 32

// NonceSize is the GCM nonce length (12 bytes).
const NonceSize = 12

// SaltSize is the Argon2id salt length (16 bytes).
const SaltSize = 16

// Argon2id parameters matching security best practices (64 MiB, 3 iterations, 4 parallelism).
const (
	argonMemory  = 64 * 1024 // 64 MiB
	argonTime    = 3
	argonThreads = 4
)

// Zeroize overwrites buf with zeros. Use after sensitive data is no longer needed.
func Zeroize(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}

// DeriveKey derives a 32-byte AES key from a password and salt using Argon2id.
func DeriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, KeySize)
}

// NewSalt generates a cryptographically random salt.
func NewSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("crypto: salt: %w", err)
	}
	return salt, nil
}

// Encrypt encrypts plaintext with AES-256-GCM using key. Returns base64(nonce || ciphertext).
func Encrypt(plaintext []byte, key []byte) (string, error) {
	if len(key) != KeySize {
		return "", fmt.Errorf("crypto: key must be %d bytes", KeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64(nonce || ciphertext) string with AES-256-GCM.
func Decrypt(encoded string, key []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: key must be %d bytes", KeySize)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < NonceSize {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce := ciphertext[:NonceSize]
	encrypted := ciphertext[NonceSize:]
	return aesGCM.Open(nil, nonce, encrypted, nil)
}

// ConstantTimeEqual performs a constant-time comparison of two byte slices.
// Use for comparing hashes, tokens, and derived keys.
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// SecureWrite writes data to path atomically: temp file + rename, with 0o600
// permissions. On Windows, the atomic rename may not be fully atomic; callers
// must ensure the target directory is on the same volume.
func SecureWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("crypto: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("crypto: atomic rename: %w", err)
	}
	return nil
}
