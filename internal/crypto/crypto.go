package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const encPrefix = "v1:"

// Cipher encrypts and decrypts sensitive fields stored in SQLite.
type Cipher struct {
	key [32]byte
}

// NewCipher derives a 256-bit key from the provided master secret.
func NewCipher(masterSecret string) *Cipher {
	sum := sha256.Sum256([]byte("ecs-cdt-panel-enc:" + masterSecret))
	return &Cipher{key: sum}
}

// Encrypt encrypts plaintext using AES-256-GCM. Empty input returns empty output.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts strings produced by Encrypt.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if !strings.HasPrefix(encoded, encPrefix) {
		return "", errors.New("unsupported ciphertext format")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return string(plain), nil
}

type sessionPayload struct {
	Username string `json:"u"`
	ExpUnix  int64  `json:"exp"`
}

// SignToken creates an HMAC-SHA256 signed session cookie token.
func SignToken(secret, username string, ttl time.Duration) (string, error) {
	payload := sessionPayload{
		Username: username,
		ExpUnix:  time.Now().Add(ttl).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	sig := computeHMAC(secret, body)
	return body + "." + sig, nil
}

// VerifyToken checks signature and expiration, returning the authenticated username.
func VerifyToken(secret, token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	body, sig := parts[0], parts[1]
	expectedSig := computeHMAC(secret, body)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", false
	}
	var payload sessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}
	if payload.Username == "" || time.Now().Unix() > payload.ExpUnix {
		return "", false
	}
	return payload.Username, true
}

func computeHMAC(secret, data string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyPassword checks input against either a bcrypt hash or a plaintext secret in constant time.
func VerifyPassword(input, plainCfg, hashCfg string) bool {
	if hashCfg != "" {
		return bcrypt.CompareHashAndPassword([]byte(hashCfg), []byte(input)) == nil
	}
	if plainCfg == "" {
		return false
	}
	a := sha256.Sum256([]byte(input))
	b := sha256.Sum256([]byte(plainCfg))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// VerifyUsername checks username in constant time.
func VerifyUsername(input, expected string) bool {
	a := sha256.Sum256([]byte(input))
	b := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// MaskSecret obscures sensitive identifiers/tokens for safe UI display.
func MaskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// KeyFingerprint produces a stable short identifier for deduplicating identical AccessKey IDs.
func KeyFingerprint(accessKeyID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(accessKeyID)))
	return fmt.Sprintf("%x", sum[:8])
}
