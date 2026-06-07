package verification

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

func signJWTForTest(secret []byte, unsigned string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
