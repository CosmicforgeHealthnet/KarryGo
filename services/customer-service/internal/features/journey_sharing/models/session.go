package sharemodels

import (
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/journeyshare"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

type Session struct {
	ID                string     `json:"id"`
	CustomerID        string     `json:"-"`
	Authority         string     `json:"authority"`
	SourceRef         string     `json:"source_ref"`
	ProductCode       string     `json:"product_code"`
	TokenHash         string     `json:"-"`
	Permissions       []string   `json:"permissions"`
	Status            string     `json:"status"`
	ExpiresAt         time.Time  `json:"expires_at"`
	SourceCompletedAt *time.Time `json:"source_completed_at,omitempty"`
	Version           int64      `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	LastAccessAt      *time.Time `json:"last_access_at,omitempty"`
}
type Create struct {
	Authority   string   `json:"authority"`
	SourceRef   string   `json:"source_ref"`
	ProductCode string   `json:"product_code"`
	Permissions []string `json:"permissions"`
}

func (in *Create) Validate() error {
	if !journeyshare.ValidCombination(in.Authority, in.ProductCode) || strings.TrimSpace(in.SourceRef) != in.SourceRef || len(in.SourceRef) == 0 || len(in.SourceRef) > 200 {
		return apperrors.Validation("Invalid journey source.", []apperrors.FieldViolation{{Field: "source_ref", Message: "A valid authority, source and product are required."}})
	}
	if in.Permissions == nil {
		in.Permissions = []string{"route", "provider", "location", "eta"}
	}
	if len(in.Permissions) > 4 {
		return apperrors.BadRequest("Invalid share permissions.", nil)
	}
	seen := map[string]bool{}
	for _, p := range in.Permissions {
		if seen[p] || (p != "route" && p != "provider" && p != "location" && p != "eta") {
			return apperrors.BadRequest("Invalid share permissions.", nil)
		}
		seen[p] = true
	}
	return nil
}
func Token() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:]), err
}
func Digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func (s Session) Readable(now time.Time) bool {
	return (s.Status == "ACTIVE" || s.Status == "COMPLETED") && now.Before(s.ExpiresAt)
}
