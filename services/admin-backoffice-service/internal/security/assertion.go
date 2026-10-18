// Package security implements the remote boundary without changing internal HMAC v2.
package security

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var ErrDenied = errors.New("authorization denied")
var opaque = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$`)

func Opaque(v string) bool { return opaque.MatchString(v) }

type Claims struct {
	Issuer        string   `json:"iss"`
	Subject       string   `json:"sub"`
	Audience      string   `json:"aud"`
	Product       string   `json:"product"`
	Roles         []string `json:"roles"`
	Permissions   []string `json:"permissions"`
	Markets       []string `json:"market_scopes"`
	Zones         []string `json:"zone_scopes,omitempty"`
	Organizations []string `json:"organization_scopes"`
	SessionID     string   `json:"session_id"`
	AuthTime      int64    `json:"auth_time"`
	ACR           string   `json:"acr"`
	JTI           string   `json:"jti"`
	IssuedAt      int64    `json:"iat"`
	NotBefore     int64    `json:"nbf"`
	Expires       int64    `json:"exp"`
	RequestDigest string   `json:"request_digest"`
}

// Digest covers the exact method, raw request URI, product and body hash.
// Authorization scope and purpose live in the URI or typed command body.
func Digest(r *http.Request, body []byte) string {
	b := sha256.Sum256(body)
	sum := sha256.Sum256([]byte(strings.Join([]string{r.Method, r.URL.RequestURI(), config.Product, hex.EncodeToString(b[:])}, "\n")))
	return hex.EncodeToString(sum[:])
}
func Hash(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }

func Strict(data []byte, out any) error {
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		return ErrDenied
	}
	// Duplicate object keys are ambiguous across implementations; reject them.
	d := json.NewDecoder(strings.NewReader(string(data)))
	if err := uniqueValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrDenied
	}
	d = json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func uniqueValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return ErrDenied
				}
				seen[key] = true
				if e = uniqueValue(d); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e := uniqueValue(d); e != nil {
					return e
				}
			}
		default:
			return ErrDenied
		}
		_, err = d.Token()
		return err
	}
	return nil
}

func Verify(token string, trust config.Trust, now time.Time) (Claims, error) {
	var c Claims
	if len(token) > 16384 {
		return c, ErrDenied
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, ErrDenied
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, ErrDenied
	}
	var h struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
		Type      string `json:"typ"`
	}
	if Strict(header, &h) != nil || h.Algorithm != "EdDSA" || h.Type != "JWT" || trust.StaffKeyExpires[h.KeyID] <= now.Unix() {
		return c, ErrDenied
	}
	key, err := base64.RawURLEncoding.DecodeString(trust.StaffKeys[h.KeyID])
	if err != nil || len(key) != ed25519.PublicKeySize {
		return c, ErrDenied
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(ed25519.PublicKey(key), []byte(parts[0]+"."+parts[1]), signature) {
		return c, ErrDenied
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || Strict(payload, &c) != nil {
		return c, ErrDenied
	}
	n := now.Unix()
	if _, err := hex.DecodeString(c.RequestDigest); err != nil || c.RequestDigest != strings.ToLower(c.RequestDigest) {
		return Claims{}, ErrDenied
	}
	if c.Issuer != trust.Issuer || c.Audience != trust.Audience || c.Product != config.Product || !Opaque(c.Subject) || !Opaque(c.SessionID) || !Opaque(c.JTI) || len(c.Roles) == 0 || len(c.Permissions) == 0 || len(c.Markets) == 0 || len(c.Organizations) == 0 || !config.Contains(trust.AllowedACR, c.ACR) || c.AuthTime <= 0 || c.AuthTime > c.IssuedAt || n-c.AuthTime > trust.SessionLifetimeSeconds || c.IssuedAt <= 0 || c.IssuedAt > n || c.NotBefore < c.IssuedAt || c.NotBefore > n || c.Expires <= n || c.Expires <= c.NotBefore || c.Expires-c.IssuedAt > trust.AssertionLifetimeSeconds || len(c.RequestDigest) != 64 {
		return Claims{}, ErrDenied
	}
	for _, values := range [][]string{c.Roles, c.Permissions, c.Markets, c.Zones, c.Organizations} {
		if len(values) > 128 {
			return Claims{}, ErrDenied
		}
		seen := map[string]bool{}
		for _, v := range values {
			if !Opaque(v) || seen[v] {
				return Claims{}, ErrDenied
			}
			seen[v] = true
		}
	}
	return c, nil
}
