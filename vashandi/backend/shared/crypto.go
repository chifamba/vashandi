package shared

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
)

// EncryptLocalSecret encrypts a plaintext value using AES-256-GCM.
// This is compatible with the Node.js local_encrypted provider.
func EncryptLocalSecret(plaintext string) (string, error) {
	encryptionSecret := os.Getenv("ENCRYPTION_SECRET")
	if encryptionSecret == "" {
		return "", fmt.Errorf("ENCRYPTION_SECRET environment variable is not set")
	}

	key := []byte(encryptionSecret)
	if len(key) != 32 {
		return "", fmt.Errorf("ENCRYPTION_SECRET must be exactly 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Generate random IV
	iv := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt
	ciphertextWithTag := aesgcm.Seal(nil, iv, []byte(plaintext), nil)

	// Split ciphertext and auth tag (last 16 bytes)
	tagSize := 16
	ciphertext := ciphertextWithTag[:len(ciphertextWithTag)-tagSize]
	authTag := ciphertextWithTag[len(ciphertextWithTag)-tagSize:]

	material := map[string]string{
		"encrypted": base64.StdEncoding.EncodeToString(ciphertext),
		"iv":        base64.StdEncoding.EncodeToString(iv),
		"authTag":   base64.StdEncoding.EncodeToString(authTag),
	}

	data, err := json.Marshal(material)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// DecryptLocalSecret decrypts a secret material using the ENCRYPTION_SECRET environment variable.
// This is compatible with the Node.js local_encrypted provider.
func DecryptLocalSecret(materialJson []byte) (string, error) {
	var material struct {
		Encrypted string `json:"encrypted"`
		IV        string `json:"iv"`
		AuthTag   string `json:"authTag"`
	}
	if err := json.Unmarshal(materialJson, &material); err != nil {
		return "", fmt.Errorf("failed to unmarshal secret material: %w", err)
	}

	encryptionSecret := os.Getenv("ENCRYPTION_SECRET")
	if encryptionSecret == "" {
		return "", fmt.Errorf("ENCRYPTION_SECRET environment variable is not set")
	}

	// In Node.js, the key is often derived or used directly if it's 32 bytes.
	// We'll assume a 32-byte key for AES-256.
	key := []byte(encryptionSecret)
	if len(key) != 32 {
		return "", fmt.Errorf("ENCRYPTION_SECRET must be exactly 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	iv, err := base64.StdEncoding.DecodeString(material.IV)
	if err != nil {
		return "", fmt.Errorf("invalid IV encoding: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(material.Encrypted)
	if err != nil {
		return "", fmt.Errorf("invalid ciphertext encoding: %w", err)
	}

	authTag, err := base64.StdEncoding.DecodeString(material.AuthTag)
	if err != nil {
		return "", fmt.Errorf("invalid authTag encoding: %w", err)
	}

	// Append auth tag to ciphertext for Go's GCM Open
	fullCiphertext := append(ciphertext, authTag...)

	plaintext, err := aesgcm.Open(nil, iv, fullCiphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
