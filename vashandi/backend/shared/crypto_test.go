package shared

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"testing"
)

// encryptForTest encrypts plaintext using AES-256-GCM and returns a JSON material blob
// compatible with DecryptLocalSecret.
func encryptForTest(plaintext, key string) ([]byte, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	sealed := aesgcm.Seal(nil, nonce, []byte(plaintext), nil)
	// GCM appends a 16-byte auth tag at the end.
	tagSize := aesgcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagSize]
	authTag := sealed[len(sealed)-tagSize:]

	material := map[string]string{
		"encrypted": base64.StdEncoding.EncodeToString(ciphertext),
		"iv":        base64.StdEncoding.EncodeToString(nonce),
		"authTag":   base64.StdEncoding.EncodeToString(authTag),
	}
	return json.Marshal(material)
}

func TestDecryptLocalSecret_RoundTrip(t *testing.T) {
	const key = "12345678901234567890123456789012" // 32 bytes
	const plaintext = "super-secret-value"

	t.Setenv("ENCRYPTION_SECRET", key)

	material, err := encryptForTest(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := DecryptLocalSecret(material)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("expected %q, got %q", plaintext, got)
	}
}

func TestDecryptLocalSecret_MissingEnvVar(t *testing.T) {
	os.Unsetenv("ENCRYPTION_SECRET")

	material := []byte(`{"encrypted":"abc","iv":"def","authTag":"ghi"}`)
	_, err := DecryptLocalSecret(material)
	if err == nil {
		t.Error("expected error when ENCRYPTION_SECRET is not set")
	}
}

func TestDecryptLocalSecret_WrongKeySize(t *testing.T) {
	t.Setenv("ENCRYPTION_SECRET", "tooshort") // not 32 bytes

	material := []byte(`{"encrypted":"abc","iv":"def","authTag":"ghi"}`)
	_, err := DecryptLocalSecret(material)
	if err == nil {
		t.Error("expected error for wrong key size")
	}
}

func TestDecryptLocalSecret_InvalidJSON(t *testing.T) {
	t.Setenv("ENCRYPTION_SECRET", "12345678901234567890123456789012")

	_, err := DecryptLocalSecret([]byte("not-json"))
	if err == nil {
		t.Error("expected error for invalid JSON material")
	}
}

func TestDecryptLocalSecret_InvalidBase64IV(t *testing.T) {
	t.Setenv("ENCRYPTION_SECRET", "12345678901234567890123456789012")

	material := []byte(`{"encrypted":"YWJj","iv":"!!!invalid!!!","authTag":"YWJj"}`)
	_, err := DecryptLocalSecret(material)
	if err == nil {
		t.Error("expected error for invalid IV base64")
	}
}

func TestDecryptLocalSecret_TamperedCiphertext(t *testing.T) {
	const key = "12345678901234567890123456789012"
	const plaintext = "my secret"

	t.Setenv("ENCRYPTION_SECRET", key)

	material, err := encryptForTest(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Tamper with the material JSON.
	var m map[string]string
	json.Unmarshal(material, &m)
	// Flip the first byte of the ciphertext.
	raw, _ := base64.StdEncoding.DecodeString(m["encrypted"])
	if len(raw) > 0 {
		raw[0] ^= 0xFF
	}
	m["encrypted"] = base64.StdEncoding.EncodeToString(raw)
	tampered, _ := json.Marshal(m)

	_, err = DecryptLocalSecret(tampered)
	if err == nil {
		t.Error("expected decryption error for tampered ciphertext")
	}
}
