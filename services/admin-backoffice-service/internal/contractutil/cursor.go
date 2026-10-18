// Package contractutil provides scoped pagination primitives for future adapters.
package contractutil

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Cursor struct {
	Product    string         `json:"product"`
	Staff      string         `json:"staff"`
	Permission string         `json:"permission"`
	Scope      security.Scope `json:"scope"`
	Sort       string         `json:"sort"`
	AfterTime  time.Time      `json:"after_time"`
	AfterID    string         `json:"after_id"`
	Expires    int64          `json:"exp"`
}

func Encode(c Cursor, key []byte) (string, error) {
	if len(key) < 32 || c.Product != "karryam" || !security.Opaque(c.Staff) || !security.Opaque(c.AfterID) || c.Sort != "created_at,id" {
		return "", errors.New("invalid cursor")
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func Decode(token string, key []byte, expected Cursor, now time.Time) (Cursor, error) {
	var c Cursor
	if len(token) > 2048 || len(key) < 32 {
		return c, security.ErrDenied
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return c, security.ErrDenied
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, security.ErrDenied
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return c, security.ErrDenied
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || security.Strict(raw, &c) != nil {
		return c, security.ErrDenied
	}
	if c.Product != "karryam" || c.Product != expected.Product || c.Staff != expected.Staff || c.Permission != expected.Permission || c.Scope != expected.Scope || c.Sort != "created_at,id" || c.Sort != expected.Sort || c.Expires <= now.Unix() || !security.Opaque(c.AfterID) {
		return Cursor{}, security.ErrDenied
	}
	return c, nil
}
func Bounds(pageSize int, start, end time.Time) error {
	if pageSize < 1 || pageSize > 100 || start.IsZero() || end.Before(start) || end.Sub(start) > 31*24*time.Hour {
		return errors.New("query bounds exceeded")
	}
	return nil
}
