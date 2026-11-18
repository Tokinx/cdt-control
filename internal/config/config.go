package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Port                 string
	DataDir              string
	PanelUsername        string
	PanelPassword        string
	PanelPasswordHash    string
	SessionSecret        string
	EncryptionKeyRaw     string
	CookieSecure         bool
	DefaultCheckInterval int
	GeneratedPassword    bool
}

// Load reads configuration from environment variables and ensures DataDir and SessionSecret exist.
func Load() (*Config, error) {
	port := getEnv("PORT", "8080")
	dataDir := getEnv("DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", dataDir, err)
	}

	username := getEnv("PANEL_USERNAME", "admin")
	password := strings.TrimSpace(os.Getenv("PANEL_PASSWORD"))
	passwordHash := strings.TrimSpace(os.Getenv("PANEL_PASSWORD_HASH"))
	generatedPassword := false

	if password == "" && passwordHash == "" {
		var err error
		password, err = randomHex(12)
		if err != nil {
			return nil, fmt.Errorf("generate startup password: %w", err)
		}
		generatedPassword = true
	}

	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	if sessionSecret == "" {
		var err error
		sessionSecret, err = loadOrCreateSecretFile(filepath.Join(dataDir, ".session_secret"))
		if err != nil {
			return nil, fmt.Errorf("init session secret: %w", err)
		}
	}

	encKeyRaw := strings.TrimSpace(os.Getenv("DATA_ENCRYPTION_KEY"))
	if encKeyRaw == "" {
		encKeyRaw = sessionSecret
	}

	cookieSecure := parseBoolEnv("COOKIE_SECURE", false)
	interval := parseIntEnv("CHECK_INTERVAL", 60)
	if interval < 10 {
		interval = 10
	}

	return &Config{
		Port:                 port,
		DataDir:              dataDir,
		PanelUsername:        username,
		PanelPassword:        password,
		PanelPasswordHash:    passwordHash,
		SessionSecret:        sessionSecret,
		EncryptionKeyRaw:     encKeyRaw,
		CookieSecure:         cookieSecure,
		DefaultCheckInterval: interval,
		GeneratedPassword:    generatedPassword,
	}, nil
}

func loadOrCreateSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		secret := strings.TrimSpace(string(data))
		if len(secret) >= 32 {
			return secret, nil
		}
	}
	secret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

func randomHex(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func parseBoolEnv(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
