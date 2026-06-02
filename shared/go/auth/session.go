package auth

import (
	"crypto/rand"
	"encoding/base64"
)

func GenerateOpaqueToken(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 32
	}

	raw := make([]byte, byteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func HashRefreshToken(secret []byte, token string) string {
	return hmacHex(secret, "refresh", token)
}
