package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

type UploadGrant struct {
	SessionID        string
	MediaRef         string
	URL              string
	Token            string
	ExpiresAt        time.Time
	MaxBytes         int64
	AllowedMIMETypes []string
}

func (c *Client) WithPublicURL(origin string) *Client {
	c.publicURL = strings.TrimRight(origin, "/")
	return c
}

func (c *Client) AuthorizeOnboardingUpload(ctx context.Context, businessID, dimension, mimeType string, size int64, key string) (UploadGrant, error) {
	public, err := url.Parse(c.publicURL)
	if err != nil || public.Host == "" || (public.Scheme != "https" && public.Scheme != "http") || public.User != nil || public.RawQuery != "" || public.Fragment != "" || public.Path != "" {
		return UploadGrant{}, fmt.Errorf("public media origin is not configured")
	}
	body, err := json.Marshal(map[string]any{
		"ownerType": "VENDOR_BUSINESS", "ownerReference": businessID,
		"documentCategory": "VENDOR_ONBOARDING_" + dimension,
		"uploadPurpose":    "COMPLIANCE_DOCUMENT", "visibility": "PRIVATE",
		"retentionClass": "COMPLIANCE", "allowedMimeTypes": []string{mimeType}, "maxSizeBytes": size,
	})
	if err != nil {
		return UploadGrant{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/media-files/internal/media/upload-authorizations", bytes.NewReader(body))
	if err != nil {
		return UploadGrant{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-ID", businessID)
	req.Header.Set("Idempotency-Key", businessID+":"+key)
	sum := sha256.Sum256(body)
	req.Header.Set("X-Request-Fingerprint", hex.EncodeToString(sum[:]))
	if err = serviceauth.SignRequest(req, "vendor-service", c.secret, body, time.Now()); err != nil {
		return UploadGrant{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return UploadGrant{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return UploadGrant{}, fmt.Errorf("media authorization returned status %d", response.StatusCode)
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			SessionID        string    `json:"uploadAuthorizationId"`
			MediaRef         string    `json:"mediaObjectId"`
			Token            string    `json:"uploadToken"`
			ExpiresAt        time.Time `json:"expiresAt"`
			MaxBytes         int64     `json:"maxSizeBytes"`
			AllowedMIMETypes []string  `json:"allowedMimeTypes"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil {
		return UploadGrant{}, err
	}
	if !result.Success || result.Data.MediaRef == "" {
		return UploadGrant{}, fmt.Errorf("media upload grant was refused")
	}
	return UploadGrant{SessionID: result.Data.SessionID, MediaRef: result.Data.MediaRef,
		URL:   c.publicURL + "/api/v1/media-files/vendor-uploads/" + url.PathEscape(result.Data.MediaRef) + "/content",
		Token: result.Data.Token, ExpiresAt: result.Data.ExpiresAt, MaxBytes: result.Data.MaxBytes, AllowedMIMETypes: result.Data.AllowedMIMETypes}, nil
}
