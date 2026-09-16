package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	apiKeyPrefix = "ck"
	secretBytes  = 32
)

// HashAPIKey returns the verifier persisted for a high-entropy API key.
func HashAPIKey(secret string) []byte {
	digest := sha256.Sum256([]byte(secret))
	return digest[:]
}

// GenerateAPIKey creates a key with a public lookup ID and a one-time secret.
func GenerateAPIKey(credentialID string) (string, []byte, error) {
	secretBytesValue := make([]byte, secretBytes)
	if _, err := rand.Read(secretBytesValue); err != nil {
		return "", nil, fmt.Errorf("generate API key secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytesValue)
	return FormatAPIKey(credentialID, secret), HashAPIKey(secret), nil
}

func FormatAPIKey(credentialID, secret string) string {
	return apiKeyPrefix + "_" + credentialID + "." + secret
}

// ParseAPIKey returns the public credential ID and secret from a generated key.
func ParseAPIKey(key string) (string, string, error) {
	if !strings.HasPrefix(key, apiKeyPrefix+"_") {
		return "", "", errors.New("invalid API key prefix")
	}
	value := strings.TrimPrefix(key, apiKeyPrefix+"_")
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid API key format")
	}
	return parts[0], parts[1], nil
}
