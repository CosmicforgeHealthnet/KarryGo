package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/vendor-service/internal/features/crm"
)

// ArtifactStore stores Section 8H export artifacts in media-file-service's
// private object contract.
//
// Vendor Service deliberately owns no file storage of its own. A CRM export is
// the widest customer-data disclosure this service produces, and the media
// service already provides exactly the controls it needs: HMAC service
// authentication, a caller allowlist, single-use request nonces, private
// objects with no public URL, content inspection, short-lived download grants,
// retention and deletion. Reimplementing any of that here would be a second,
// weaker copy.
//
// The upload is a two-step grant-then-send flow, so the bytes only travel once
// the store has agreed to a purpose, a size ceiling and a content-type
// allowlist for this specific object.
type ArtifactStore struct {
	baseURL string
	secret  []byte
	http    *http.Client
	// downloadTTLHint is reported to callers when the store does not state an
	// expiry of its own; the store's own shorter window always wins.
	downloadTTLHint time.Duration
}

// NewArtifactStore builds the adapter.
func NewArtifactStore(baseURL, secret string, timeout, downloadTTLHint time.Duration) *ArtifactStore {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if downloadTTLHint <= 0 {
		downloadTTLHint = 5 * time.Minute
	}
	return &ArtifactStore{
		baseURL: strings.TrimRight(baseURL, "/"), secret: []byte(secret),
		http: &http.Client{Timeout: timeout}, downloadTTLHint: downloadTTLHint,
	}
}

const (
	privateBase = "/api/v1/media-files/internal/media"
	// ownerTypeExport and purposeDataExport name what the object is, so the
	// store applies its export rules rather than its document rules.
	ownerTypeExport   = "VENDOR_CRM_EXPORT"
	purposeDataExport = "DATA_EXPORT"
)

type uploadGrant struct {
	AuthorizationID string `json:"uploadAuthorizationId"`
	MediaObjectID   string `json:"mediaObjectId"`
	UploadToken     string `json:"uploadToken"`
	MaxSizeBytes    int64  `json:"maxSizeBytes"`
	Replayed        bool   `json:"replayed"`
}

type grantEnvelope struct {
	Success bool        `json:"success"`
	Data    uploadGrant `json:"data"`
}

type objectEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		ID            string `json:"mediaObjectId"`
		Purpose       string `json:"uploadPurpose"`
		Visibility    string `json:"visibility"`
		DetectedMIME  string `json:"detectedMime"`
		SizeBytes     int64  `json:"sizeBytes"`
		Checksum      string `json:"checksumSha256"`
		UploadStatus  string `json:"uploadStatus"`
		DeletionState string `json:"deletionState"`
	} `json:"data"`
}

type deletionEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Status      string `json:"status"`
		BlockReason string `json:"blockReason"`
	} `json:"data"`
}

type downloadEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		MediaObjectID string `json:"mediaObjectId"`
		Token         string `json:"downloadToken"`
		Path          string `json:"downloadPath"`
		ExpiresAt     string `json:"expiresAt"`
		ContentType   string `json:"contentType"`
		SizeBytes     int64  `json:"sizeBytes"`
	} `json:"data"`
}

// Put stores one generated export privately.
func (s *ArtifactStore) Put(ctx context.Context, in crm.PutArtifactInput) (crm.StoredArtifact, error) {
	checksum := sha256Hex(in.Content)
	body, err := json.Marshal(map[string]any{
		"ownerType":        ownerTypeExport,
		"ownerReference":   in.ExportID,
		"documentCategory": "CRM_EXPORT",
		"uploadPurpose":    purposeDataExport,
		"allowedMimeTypes": []string{in.MediaType},
		"maxSizeBytes":     int64(len(in.Content)),
		"retentionClass":   "STANDARD",
		// The store settles the format at authorization time: the declared type
		// and the file extension must agree, and a data export may only ever be
		// stored privately. Both are sent explicitly rather than relying on a
		// default, so a change to either side is a visible break.
		"visibility": "PRIVATE",
		"fileName":   in.Filename,
	})
	if err != nil {
		return crm.StoredArtifact{}, err
	}
	// The idempotency key is the export job, so a retried generation adopts the
	// grant the first attempt created instead of orphaning an object.
	headers := map[string]string{
		"Idempotency-Key":       in.IdempotencyKey,
		"X-Request-Fingerprint": fingerprintOf(body),
	}
	raw, err := s.call(ctx, http.MethodPost, privateBase+"/upload-authorizations",
		in.BusinessID, body, headers, nil)
	if err != nil {
		return crm.StoredArtifact{}, err
	}
	var grant grantEnvelope
	if err = json.Unmarshal(raw, &grant); err != nil {
		return crm.StoredArtifact{}, err
	}
	if grant.Data.MediaObjectID == "" {
		return crm.StoredArtifact{}, fmt.Errorf("media store returned no object id")
	}

	if grant.Data.Replayed && grant.Data.UploadToken == "" {
		// Media completed this idempotent DATA_EXPORT upload before the caller
		// received its response. Reconcile the same opaque object rather than
		// trying to spend the consumed upload grant or creating another object.
		raw, err = s.call(ctx, http.MethodGet,
			privateBase+"/objects/"+url.PathEscape(grant.Data.MediaObjectID),
			in.BusinessID, nil, nil, nil)
		if err != nil {
			return crm.StoredArtifact{}, err
		}
	} else {
		if grant.Data.UploadToken == "" {
			return crm.StoredArtifact{}, fmt.Errorf("media store returned no upload token")
		}
		uploadHeaders := map[string]string{
			"X-Upload-Authorization-ID":  grant.Data.AuthorizationID,
			"X-Upload-Token":             grant.Data.UploadToken,
			"X-Declared-Content-Type":    in.MediaType,
			"X-Declared-Size":            strconv.Itoa(len(in.Content)),
			"X-Declared-Checksum-SHA256": checksum,
		}
		raw, err = s.call(ctx, http.MethodPost,
			privateBase+"/objects/"+url.PathEscape(grant.Data.MediaObjectID)+"/content",
			in.BusinessID, in.Content, uploadHeaders, map[string]string{"Content-Type": in.MediaType})
		if err != nil {
			return crm.StoredArtifact{}, err
		}
	}
	var object objectEnvelope
	if err = json.Unmarshal(raw, &object); err != nil {
		return crm.StoredArtifact{}, err
	}
	stored, err := verifiedStoredArtifact(object, grant.Data.MediaObjectID, in.MediaType,
		int64(len(in.Content)), checksum)
	if err != nil {
		return crm.StoredArtifact{}, err
	}
	if err = s.setRetention(ctx, in.BusinessID, stored.Ref, in.RetainUntil); err != nil {
		// Treat retention as part of durable persistence. A retry will reconcile
		// this same object and try the retention update again; the export cannot
		// become READY while Media lacks its independent expiry bound.
		return crm.StoredArtifact{}, err
	}
	return stored, nil
}

func verifiedStoredArtifact(object objectEnvelope, expectedRef, expectedMIME string,
	expectedBytes int64, expectedChecksum string) (crm.StoredArtifact, error) {
	if !object.Success || object.Data.ID != expectedRef ||
		object.Data.Purpose != purposeDataExport || object.Data.Visibility != "PRIVATE" ||
		object.Data.UploadStatus != "COMPLETED" || object.Data.DeletionState != "ACTIVE" ||
		object.Data.DetectedMIME != expectedMIME || object.Data.SizeBytes != expectedBytes ||
		object.Data.Checksum != expectedChecksum {
		return crm.StoredArtifact{}, fmt.Errorf("media store could not verify the persisted export")
	}
	return crm.StoredArtifact{
		Ref: object.Data.ID, Bytes: object.Data.SizeBytes, Checksum: object.Data.Checksum,
	}, nil
}

func (s *ArtifactStore) setRetention(ctx context.Context, businessID, ref string, until time.Time) error {
	if until.IsZero() {
		return fmt.Errorf("media artifact retention is required")
	}
	body, err := json.Marshal(map[string]any{
		"retentionClass": "STANDARD",
		"retentionUntil": until.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	_, err = s.call(ctx, http.MethodPost,
		privateBase+"/objects/"+url.PathEscape(ref)+"/retention", businessID, body, nil, nil)
	return err
}

// Access issues a short-lived read grant.
//
// Nothing durable is produced: the token is minted per request by the store,
// expires on the store's own clock, and is never written down here.
func (s *ArtifactStore) Access(ctx context.Context, businessID, ref string) (crm.ArtifactAccess, error) {
	raw, err := s.call(ctx, http.MethodPost,
		privateBase+"/objects/"+url.PathEscape(ref)+"/download-authorizations",
		businessID, []byte("{}"), nil, nil)
	if err != nil {
		return crm.ArtifactAccess{}, err
	}
	var grant downloadEnvelope
	if err = json.Unmarshal(raw, &grant); err != nil {
		return crm.ArtifactAccess{}, err
	}
	if grant.Data.Token == "" {
		return crm.ArtifactAccess{}, fmt.Errorf("media store returned no download grant")
	}
	expires := time.Now().UTC().Add(s.downloadTTLHint)
	if parsed, parseErr := time.Parse(time.RFC3339, grant.Data.ExpiresAt); parseErr == nil {
		expires = parsed
	}
	return crm.ArtifactAccess{
		Ref: ref, Path: grant.Data.Path, Token: grant.Data.Token, ExpiresAt: expires,
		MediaType: grant.Data.ContentType, Bytes: grant.Data.SizeBytes,
	}, nil
}

// Delete revokes an artifact. It is safe to call for something already gone,
// because cancellation and expiry both reach it and neither can assume it ran
// first.
func (s *ArtifactStore) Delete(ctx context.Context, businessID, ref, reason string) error {
	// DATA_EXPORT retention is a maximum lifetime, not a legal minimum. Move
	// the bound to now before cancellation, expiry or checksum-failure cleanup
	// so Media's ordinary deletion semantics can actually destroy the bytes.
	if err := s.setRetention(ctx, businessID, ref, time.Now().UTC()); err != nil &&
		!strings.Contains(err.Error(), "status 404") {
		return err
	}
	body, err := json.Marshal(map[string]any{"reason": reason})
	if err != nil {
		return err
	}
	headers := map[string]string{
		"Idempotency-Key":       "crm-export-delete-" + sha256Hex([]byte(ref+"\x00"+reason)),
		"X-Request-Fingerprint": fingerprintOf(body),
	}
	raw, err := s.call(ctx, http.MethodPost,
		privateBase+"/objects/"+url.PathEscape(ref)+"/deletions", businessID, body, headers, nil)
	if err != nil && strings.Contains(err.Error(), "status 404") {
		return nil
	}
	if err != nil {
		return err
	}
	var deletion deletionEnvelope
	if err = json.Unmarshal(raw, &deletion); err != nil {
		return err
	}
	if !deletion.Success || deletion.Data.Status != "COMPLETED" {
		return fmt.Errorf("media artifact deletion did not complete")
	}
	return nil
}

// call signs and sends one private-contract request.
//
// Every request carries a fresh single-use nonce, which is what makes a
// captured request unreplayable even by something that holds the transcript.
func (s *ArtifactStore) call(ctx context.Context, method, path, businessID string, body []byte,
	extra map[string]string, headers map[string]string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	request.Header.Set("X-Organization-ID", businessID)
	request.Header.Set("X-Service-Nonce", randomNonce())
	for key, value := range extra {
		if value != "" {
			request.Header.Set(key, value)
		}
	}
	if err = serviceauth.SignRequest(request, "vendor-service", s.secret, body, time.Now()); err != nil {
		return nil, err
	}
	response, err := s.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// The store's internal detail is not echoed onward; the caller learns
		// that storage refused, never what its backend said.
		return nil, fmt.Errorf("media artifact store returned status %d", response.StatusCode)
	}
	return raw, nil
}

func randomNonce() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buffer)
}

var _ crm.ArtifactStore = (*ArtifactStore)(nil)

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// fingerprintOf is the store's request-fingerprint header: a stable digest of
// the grant request, so replaying an idempotency key with a different request
// is detected by the store rather than silently accepted.
func fingerprintOf(body []byte) string { return sha256Hex(body) }
