package media

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/serviceauth"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Client) AuthorizeAssetUpload(ctx context.Context, business, targetType, targetRef, purpose, fileName, mime string, size int64, key string) (UploadGrant, error) {
	origin, err := url.Parse(c.publicURL)
	if err != nil || origin.Host == "" || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Path != "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return UploadGrant{}, fmt.Errorf("media public origin unavailable")
	}
	body, _ := json.Marshal(map[string]any{"ownerType": "VENDOR_BUSINESS", "ownerReference": business, "uploadPurpose": purpose, "visibility": "PRIVATE", "retentionClass": "STANDARD", "fileName": fileName, "allowedMimeTypes": []string{mime}, "maxSizeBytes": size, "bindingType": "ASSET", "bindingId": targetType + ":" + targetRef})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+privateBase+"/upload-authorizations", bytes.NewReader(body))
	if err != nil {
		return UploadGrant{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-ID", business)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("X-Service-Nonce", randomNonce())
	sum := sha256.Sum256(body)
	req.Header.Set("X-Request-Fingerprint", hex.EncodeToString(sum[:]))
	if err = serviceauth.SignRequest(req, "vendor-service", c.secret, body, time.Now()); err != nil {
		return UploadGrant{}, err
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return UploadGrant{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return UploadGrant{}, fmt.Errorf("media authorization refused")
	}
	var out struct {
		Success bool `json:"success"`
		Data    struct {
			Session string    `json:"uploadAuthorizationId"`
			Ref     string    `json:"mediaObjectId"`
			Token   string    `json:"uploadToken"`
			Expires time.Time `json:"expiresAt"`
			Max     int64     `json:"maxSizeBytes"`
			MIMEs   []string  `json:"allowedMimeTypes"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return UploadGrant{}, err
	}
	d := out.Data
	if !out.Success || d.Ref == "" || d.Session == "" || d.Token == "" || !d.Expires.After(time.Now()) || d.Max <= 0 || d.Max > size || len(d.MIMEs) != 1 || d.MIMEs[0] != mime {
		return UploadGrant{}, fmt.Errorf("media grant invalid")
	}
	return UploadGrant{SessionID: d.Session, MediaRef: d.Ref, Token: d.Token, ExpiresAt: d.Expires, MaxBytes: d.Max, AllowedMIMETypes: d.MIMEs, URL: strings.TrimRight(c.publicURL, "/") + "/api/v1/media-files/vendor-uploads/" + url.PathEscape(d.Ref) + "/content"}, nil
}
func (c *Client) AssetContent(ctx context.Context, business, ref string, maxBytes int64) ([]byte, error) {
	store := NewArtifactStore(c.baseURL, string(c.secret), c.http.Timeout, time.Minute)
	grant, err := store.Access(ctx, business, ref)
	if err != nil {
		return nil, err
	}
	return store.ReadExportContent(ctx, business, ref, grant.Token, maxBytes)
}

func (c *Client) ValidateAsset(ctx context.Context, business, kind, target, ref string) error {
	object, err := c.GetObject(ctx, business, ref)
	if err != nil {
		return fmt.Errorf("media reference could not be validated")
	}
	expected := "PRODUCT_IMAGE"
	if kind == "BRAND" {
		expected = "BRAND_IMAGE"
	}
	if kind == "PASTRY_REQUEST" {
		expected = "PASTRY_REFERENCE"
	}
	if object.OwnerService != "vendor-service" || object.Visibility != "PRIVATE" || object.UploadPurpose != expected || object.BindingType != "ASSET" || object.BindingID != kind+":"+target || object.UploadStatus != "COMPLETED" || object.SafetyStatus != "SAFE" || object.DeletionState != "ACTIVE" {
		return fmt.Errorf("media reference is not a safe completed file for this record")
	}
	return nil
}
