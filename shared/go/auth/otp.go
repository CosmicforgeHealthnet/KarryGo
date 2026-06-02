package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

const DefaultOTPLength = 6

func GenerateNumericOTP(length int) (string, error) {
	if length <= 0 {
		length = DefaultOTPLength
	}

	if length > 10 {
		return "", fmt.Errorf("otp length %d is too long", length)
	}

	code := make([]byte, length)
	for i := range code {
		value, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		code[i] = byte('0' + value.Int64())
	}

	return string(code), nil
}

func HashOTP(secret []byte, challengeID string, phone string, otp string) string {
	return hmacHex(secret, "otp", challengeID, phone, otp)
}

func VerifyOTP(secret []byte, challengeID string, phone string, otp string, expectedHash string) bool {
	actual := HashOTP(secret, challengeID, phone, otp)
	return hmac.Equal([]byte(actual), []byte(expectedHash))
}

func hmacHex(secret []byte, parts ...string) string {
	mac := hmac.New(sha256.New, secret)
	for index, part := range parts {
		if index > 0 {
			_, _ = mac.Write([]byte{0})
		}
		_, _ = mac.Write([]byte(part))
	}
	return hex.EncodeToString(mac.Sum(nil))
}
