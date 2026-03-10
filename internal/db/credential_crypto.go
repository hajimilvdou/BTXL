package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const credentialEnvelopePrefix = "enc:v1:"

var (
	credentialProtectionMu  sync.RWMutex
	credentialProtectionKey []byte
)

// ConfigureCredentialProtection configures the credential encryption key.
// Priority: env BTXL_CREDENTIAL_ENCRYPTION_KEY -> env CREDENTIAL_ENCRYPTION_KEY -> primary -> fallback.
func ConfigureCredentialProtection(primary string, fallback string) {
	secret := strings.TrimSpace(os.Getenv("BTXL_CREDENTIAL_ENCRYPTION_KEY"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("CREDENTIAL_ENCRYPTION_KEY"))
	}
	if secret == "" {
		secret = strings.TrimSpace(primary)
	}
	if secret == "" {
		secret = strings.TrimSpace(fallback)
	}

	credentialProtectionMu.Lock()
	defer credentialProtectionMu.Unlock()

	if secret == "" {
		credentialProtectionKey = nil
		return
	}

	sum := sha256.Sum256([]byte(secret))
	credentialProtectionKey = sum[:]
}

func credentialProtectionEnabled() bool {
	credentialProtectionMu.RLock()
	defer credentialProtectionMu.RUnlock()
	return len(credentialProtectionKey) == 32
}

func credentialProtectionKeyCopy() []byte {
	credentialProtectionMu.RLock()
	defer credentialProtectionMu.RUnlock()
	if len(credentialProtectionKey) == 0 {
		return nil
	}
	key := make([]byte, len(credentialProtectionKey))
	copy(key, credentialProtectionKey)
	return key
}

func maybeEncryptCredentialData(plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, credentialEnvelopePrefix) || !credentialProtectionEnabled() {
		return plain, nil
	}

	key := credentialProtectionKeyCopy()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("init credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("init credential AEAD: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate credential nonce: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, sealed...)
	return credentialEnvelopePrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

func maybeDecryptCredentialData(raw string) (string, error) {
	if raw == "" || !strings.HasPrefix(raw, credentialEnvelopePrefix) {
		return raw, nil
	}
	if !credentialProtectionEnabled() {
		return "", fmt.Errorf("credential data is encrypted but no credential protection key is configured")
	}

	encoded := strings.TrimPrefix(raw, credentialEnvelopePrefix)
	payload, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode credential payload: %w", err)
	}

	key := credentialProtectionKeyCopy()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("init credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("init credential AEAD: %w", err)
	}
	if len(payload) < gcm.NonceSize() {
		return "", fmt.Errorf("credential payload too short")
	}

	nonce := payload[:gcm.NonceSize()]
	sealed := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt credential payload: %w", err)
	}
	return string(plain), nil
}
