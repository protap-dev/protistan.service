package auth

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
)

// Secrets expected to be provided via Encore secrets.
// Encore requires secret fields to be strings. Parse numeric values at startup.
var secrets struct {
	JWTSecret  string
	BCryptCost string
}

// parsed bcrypt cost used by handlers
var bcryptCost int

func initSecrets() {
	// Validate secrets at startup
	if secrets.JWTSecret == "" {
		panic("missing JWTSecret secret")
	}
	// Parse BCryptCost from string, fallback to default 12
	bcryptCost = 12
	if secrets.BCryptCost != "" {
		if v, err := strconv.Atoi(secrets.BCryptCost); err == nil && v > 0 {
			bcryptCost = v
		}
	}
}

// generateSecureToken creates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// derefOrEmpty safely dereferences a string pointer, returning an empty string if it's nil.
func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
