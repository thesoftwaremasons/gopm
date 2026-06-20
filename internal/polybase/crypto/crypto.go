package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LoadOrCreateMasterKey reads a 32-byte AES-256 key from path, or generates
// and saves one if the file does not exist.
func LoadOrCreateMasterKey(path string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("crypto: mkdir: %w", err)
	}

	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("crypto: master key file %q has wrong size (%d bytes, want 32)", path, len(data))
		}
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("crypto: read key: %w", err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("crypto: generate key: %w", err)
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("crypto: save key: %w", err)
	}
	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with a random nonce.
// The returned ciphertext has the nonce prepended.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext (nonce prepended) using AES-256-GCM.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

// EncryptString encrypts s and returns base64(ciphertext).
func EncryptString(key []byte, s string) (string, error) {
	ct, err := Encrypt(key, []byte(s))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptString decodes base64 then decrypts.
func DecryptString(key []byte, s string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("crypto: base64 decode: %w", err)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
