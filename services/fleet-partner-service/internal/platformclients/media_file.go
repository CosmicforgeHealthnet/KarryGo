package platformclients

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The fleet side of the media authority contract.
//
// Fleet does not own files. It owns what a document MEANS — whose it is, what
// category it is, whether a human verified it, when it expires. Every question
// about the file itself — does it exist, what type is it really, did anything
// vouch for its safety, are the bytes still there — is answered by
// media-file-service over its private HMAC contract.
//
// There is deliberately no cache. A cached "safe" would outlive a quarantine,
// and a cached upload status would let fleet believe a file exists after media
// destroyed it.
//
// What this client will never do: log an upload token, log a download grant,
// persist either, or accept a permanent URL as a substitute for asking.

// MediaFileAuthorityBase is the prefix media mounts its private contract on.
const MediaFileAuthorityBase = "/api/v1/media-files/internal/media"

// Media upload states as fleet observes them.
const (
	MediaUploadPending    = "PENDING"
	MediaUploadAuthorized = "AUTHORIZED"
	MediaUploadUploading  = "UPLOADING"
	MediaUploadCompleted  = "COMPLETED"
	MediaUploadFailed     = "FAILED"
	MediaUploadExpired    = "EXPIRED"
	MediaUploadUnknown    = "UNKNOWN"
)

// Media safety states as fleet observes them.
const (
	MediaSafetyPending     = "PENDING"
	MediaSafetySafe        = "SAFE"
	MediaSafetyUnsafe      = "UNSAFE"
	MediaSafetyQuarantined = "QUARANTINED"
	MediaSafetyUnknown     = "UNKNOWN"
)

// Media deletion states as fleet observes them.
const (
	MediaDeletionActive  = "ACTIVE"
	MediaDeletionPending = "DELETION_PENDING"
	MediaDeletionDeleted = "DELETED"
	MediaDeletionUnknown = "DELETION_UNKNOWN"
	MediaDeletionFailed  = "DELETION_FAILED"
)

// ErrMediaObjectNotFound means the authority does not know this object.
//
// Distinct from a transport failure on purpose: "media says no such object" is
// a definitive answer fleet records, while "media could not be reached" is an
// UNKNOWN that must fail closed without claiming the document is bad.
var ErrMediaObjectNotFound = &Error{Service: "media-file-service", Outcome: OutcomeNotFound,
	Status: http.StatusNotFound, Message: "media object not found"}

// ErrMediaNotDownloadable means media refuses to release the content. It is a
// definitive answer about the file, not a transport problem.
var ErrMediaNotDownloadable = errors.New("platformclients: media refused to release content")

// MediaObjectState is the authoritative answer about one media object.
//
// Field names mirror media's contract exactly. Renaming them here would make a
// contract change look like a local refactor.
type MediaObjectState struct {
	MediaObjectID         string `json:"mediaObjectId"`
	OrganizationID        string `json:"organizationId"`
	OwnerService          string `json:"ownerService"`
	OwnerType             string `json:"ownerType"`
	OwnerReference        string `json:"ownerReference"`
	UploadPurpose         string `json:"uploadPurpose"`
	DeclaredMIME          string `json:"declaredMime"`
	DetectedMIME          string `json:"detectedMime"`
	SizeBytes             int64  `json:"sizeBytes"`
	ChecksumSHA256        string `json:"checksumSha256"`
	CapturedAt            string `json:"capturedAt"`
	SourceTimestamp       string `json:"sourceTimestamp"`
	CaptureMethod         string `json:"captureMethod"`
	DeviceReference       string `json:"deviceReference"`
	ActorReference        string `json:"actorReference"`
	OriginalMIMEType      string `json:"originalMimeType"`
	OriginalSizeBytes     int64  `json:"originalSizeBytes"`
	IntegrityHash         string `json:"integrityHash"`
	SignatureValidation   string `json:"signatureValidation"`
	UploadStatus          string `json:"uploadStatus"`
	SafetyStatus          string `json:"safetyStatus"`
	SafetyReason          string `json:"safetyReason"`
	DeletionState         string `json:"deletionState"`
	RetentionClass        string `json:"retentionClass"`
	LegalHold             bool   `json:"legalHold"`
	ValidationFailureCode string `json:"validationFailureCode"`
	SourceVersion         int64  `json:"sourceVersion"`
}

// MediaUploadGrant is a single-use upload authorization.
//
// UploadToken is a bearer credential. It exists in this struct and in the
// request that redeems it, and nowhere else: it is never written to the
// database and never logged.
type MediaUploadGrant struct {
	AuthorizationID  string   `json:"uploadAuthorizationId"`
	MediaObjectID    string   `json:"mediaObjectId"`
	UploadToken      string   `json:"uploadToken"`
	UploadPath       string   `json:"uploadPath"`
	ExpiresAt        string   `json:"expiresAt"`
	MaxSizeBytes     int64    `json:"maxSizeBytes"`
	AllowedMIMETypes []string `json:"allowedMimeTypes"`
	Replayed         bool     `json:"replayed"`
}

// MediaDownloadGrant is a short-lived, object-specific content grant.
//
// Token is never persisted and never logged. Fleet hands it straight to the
// caller that asked, or uses it once to stream content, and forgets it.
type MediaDownloadGrant struct {
	MediaObjectID string `json:"mediaObjectId"`
	Token         string `json:"downloadToken"`
	Path          string `json:"downloadPath"`
	ExpiresAt     string `json:"expiresAt"`
	ContentType   string `json:"contentType"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// MediaDeletionOutcome is media's authoritative deletion answer.
type MediaDeletionOutcome struct {
	DeletionRequestID string `json:"deletionRequestId"`
	MediaObjectID     string `json:"mediaObjectId"`
	Status            string `json:"status"`
	BlockReason       string `json:"blockReason"`
	Attempts          int    `json:"attempts"`
}

// MediaDeletionState pairs the object's deletion state with the latest request.
type MediaDeletionState struct {
	MediaObjectID string                `json:"mediaObjectId"`
	DeletionState string                `json:"deletionState"`
	SourceVersion int64                 `json:"sourceVersion"`
	Request       *MediaDeletionOutcome `json:"request,omitempty"`
}

// MediaUploadRequest is fleet's versioned upload-authorization request.
type MediaUploadRequest struct {
	OwnerType        string   `json:"ownerType"`
	OwnerReference   string   `json:"ownerReference"`
	DocumentCategory string   `json:"documentCategory"`
	UploadPurpose    string   `json:"uploadPurpose"`
	AllowedMIMETypes []string `json:"allowedMimeTypes"`
	MaxSizeBytes     int64    `json:"maxSizeBytes"`
	RetentionClass   string   `json:"retentionClass"`
}

// MediaAuthority is the capability fleet needs from the media owner.
type MediaAuthority interface {
	AuthorizeUpload(ctx context.Context, meta CallMeta, request MediaUploadRequest) (MediaUploadGrant, error)
	CompleteUpload(ctx context.Context, meta CallMeta, grant MediaUploadGrant,
		declaredMIME string, checksum string, content []byte) (MediaObjectState, error)
	ReadObject(ctx context.Context, meta CallMeta, mediaObjectID string) (MediaObjectState, error)
	AuthorizeDownload(ctx context.Context, meta CallMeta, mediaObjectID string) (MediaDownloadGrant, error)
	ReadContent(ctx context.Context, meta CallMeta, grant MediaDownloadGrant, maxBytes int64) ([]byte, error)
	RequestDeletion(ctx context.Context, meta CallMeta, mediaObjectID string, reason string) (MediaDeletionOutcome, error)
	ReadDeletionState(ctx context.Context, meta CallMeta, mediaObjectID string) (MediaDeletionState, error)
	SetRetention(ctx context.Context, meta CallMeta, mediaObjectID string, retentionClass string,
		retentionUntil *time.Time, legalHold *bool) (MediaObjectState, error)
	AuthorityService() string
	Configured() bool
}

// MediaFileClient reads and commands media-file-service.
type MediaFileClient struct {
	client  *Client
	apiBase string
}

// GetFile adapts the rich Media authority state to the narrow organization
// onboarding contract. Ownership and safety remain authoritative at Media.
func (c *MediaFileClient) GetFile(ctx context.Context, meta CallMeta, fileID string) (FileMetadata, error) {
	state, err := c.ReadObject(ctx, meta, fileID)
	if err != nil {
		return FileMetadata{}, err
	}
	return FileMetadata{
		FileID: state.MediaObjectID, OwnerType: state.OwnerType, OwnerID: state.OwnerReference,
		Purpose: state.UploadPurpose, UploadState: state.UploadStatus, ScanState: state.SafetyStatus,
		ContentType: state.DetectedMIME, SizeBytes: state.SizeBytes, SourceVersion: state.SourceVersion,
	}, nil
}

var _ MediaContract = (*MediaFileClient)(nil)

// NewMediaFileClient builds the client.
func NewMediaFileClient(opts Options) *MediaFileClient {
	if opts.ServiceName == "" {
		opts.ServiceName = "media-file-service"
	}
	return &MediaFileClient{client: New(opts), apiBase: MediaFileAuthorityBase}
}

// WithAPIBase overrides the mounted prefix.
func (c *MediaFileClient) WithAPIBase(base string) *MediaFileClient {
	if base != "" {
		c.apiBase = strings.TrimSuffix(base, "/")
	}
	return c
}

// AuthorityService names the owning service.
func (c *MediaFileClient) AuthorityService() string { return "media-file-service" }

// Configured reports whether live calls are possible.
func (c *MediaFileClient) Configured() bool { return c != nil && c.client.Configured() }

type mediaEnvelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

// MediaFingerprint hashes the meaningful inputs of a request.
//
// The fingerprint is what turns an idempotency key from "return whatever I
// stored last time" into "return it only if you are asking the same thing".
// Without it a caller that reuses a key by accident silently receives a grant
// for constraints it never requested.
func MediaFingerprint(parts ...string) string {
	hasher := sha256.New()
	for _, part := range parts {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(hasher.Sum(nil))
}

// writeHeaders builds the security headers every state-changing media call
// carries. The base client adds the signature, the nonce, correlation and
// causation; these are the media contract's own requirements.
func writeHeaders(meta CallMeta, fingerprint string, payloadVersion int) map[string]string {
	if payloadVersion <= 0 {
		payloadVersion = 1
	}
	return map[string]string{
		"X-Request-Fingerprint": fingerprint,
		"X-Payload-Version":     strconv.Itoa(payloadVersion),
	}
}

// AuthorizeUpload asks media for a single-use upload grant.
//
// meta.IdempotencyKey and the fingerprint are both mandatory: a retried
// authorization must return the same grant, and a key reused for a different
// request must be refused rather than answered.
func (c *MediaFileClient) AuthorizeUpload(ctx context.Context, meta CallMeta,
	request MediaUploadRequest) (MediaUploadGrant, error) {
	if !c.Configured() {
		return MediaUploadGrant{}, ErrNotConfigured
	}
	if meta.IdempotencyKey == "" {
		return MediaUploadGrant{}, &Error{Service: c.AuthorityService(),
			Outcome: OutcomePermanent, Message: "idempotency key is required"}
	}

	fingerprint := MediaFingerprint(meta.OrganizationID, request.OwnerType, request.OwnerReference,
		request.DocumentCategory, request.UploadPurpose, strings.Join(request.AllowedMIMETypes, "|"),
		strconv.FormatInt(request.MaxSizeBytes, 10), request.RetentionClass)

	var envelope mediaEnvelope[MediaUploadGrant]
	err := c.client.Exchange(ctx, meta, Exchange{
		Method:   http.MethodPost,
		Path:     c.apiBase + "/upload-authorizations",
		JSONBody: request,
		Headers:  writeHeaders(meta, fingerprint, 1),
		Out:      &envelope,
	})
	if err != nil {
		return MediaUploadGrant{}, err
	}
	if envelope.Data.MediaObjectID == "" || envelope.Data.UploadToken == "" {
		// A 200 carrying no grant is not a usable answer. Treating it as one
		// would let a misconfigured proxy returning an empty body read as an
		// authorized upload.
		return MediaUploadGrant{}, &Error{Service: c.AuthorityService(),
			Outcome: OutcomeUnknown, Status: http.StatusOK, Message: "media returned no upload grant"}
	}
	return envelope.Data, nil
}

// CompleteUpload redeems a grant with the file bytes.
//
// Fleet does not store the bytes: they pass through this call and are gone.
// The declared type, size and checksum are the caller's claims, which media
// compares against the bytes rather than believing.
func (c *MediaFileClient) CompleteUpload(ctx context.Context, meta CallMeta, grant MediaUploadGrant,
	declaredMIME string, checksum string, content []byte) (MediaObjectState, error) {
	if !c.Configured() {
		return MediaObjectState{}, ErrNotConfigured
	}

	var envelope mediaEnvelope[MediaObjectState]
	err := c.client.Exchange(ctx, meta, Exchange{
		Method:      http.MethodPost,
		Path:        c.apiBase + "/objects/" + url.PathEscape(grant.MediaObjectID) + "/content",
		RawBody:     content,
		ContentType: "application/octet-stream",
		Headers: map[string]string{
			"X-Upload-Authorization-ID":  grant.AuthorizationID,
			"X-Upload-Token":             grant.UploadToken,
			"X-Declared-Content-Type":    declaredMIME,
			"X-Declared-Size":            strconv.Itoa(len(content)),
			"X-Declared-Checksum-SHA256": checksum,
		},
		Out: &envelope,
	})
	if err != nil {
		return MediaObjectState{}, err
	}
	return envelope.Data, nil
}

// ReadObject reads the authoritative state of one object. This is also the
// reconciliation read-back: it is the only way to resolve an UNKNOWN.
func (c *MediaFileClient) ReadObject(ctx context.Context, meta CallMeta,
	mediaObjectID string) (MediaObjectState, error) {
	if !c.Configured() {
		return MediaObjectState{}, ErrNotConfigured
	}
	if strings.TrimSpace(mediaObjectID) == "" {
		return MediaObjectState{}, ErrMediaObjectNotFound
	}

	var envelope mediaEnvelope[MediaObjectState]
	err := c.client.Exchange(ctx, meta, Exchange{
		Method: http.MethodGet,
		Path:   c.apiBase + "/objects/" + url.PathEscape(mediaObjectID),
		Out:    &envelope,
	})
	if err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
			return MediaObjectState{}, ErrMediaObjectNotFound
		}
		return MediaObjectState{}, err
	}
	if envelope.Data.MediaObjectID == "" {
		return MediaObjectState{}, &Error{Service: c.AuthorityService(),
			Outcome: OutcomeUnknown, Status: http.StatusOK, Message: "media returned no object identity"}
	}
	return envelope.Data, nil
}

// ReadUploadStatus and ReadSafetyStatus are narrow reads over the same
// authoritative record. They exist as named calls because a reader of the
// reconciliation code should not have to infer which field of a wide struct
// the caller actually cared about.
func (c *MediaFileClient) ReadUploadStatus(ctx context.Context, meta CallMeta,
	mediaObjectID string) (string, int64, error) {
	state, err := c.ReadObject(ctx, meta, mediaObjectID)
	if err != nil {
		return MediaUploadUnknown, 0, err
	}
	return state.UploadStatus, state.SourceVersion, nil
}

// ReadSafetyStatus returns media's safety verdict and the version it belongs to.
func (c *MediaFileClient) ReadSafetyStatus(ctx context.Context, meta CallMeta,
	mediaObjectID string) (string, int64, error) {
	state, err := c.ReadObject(ctx, meta, mediaObjectID)
	if err != nil {
		return MediaSafetyUnknown, 0, err
	}
	return state.SafetyStatus, state.SourceVersion, nil
}

// AuthorizeDownload asks media for a short-lived content grant.
//
// Fleet has already applied its own document rules by the time this is called.
// Media applies its own floor on top and may still refuse — an UNSAFE or
// deleted object is refused whatever fleet believes about it.
func (c *MediaFileClient) AuthorizeDownload(ctx context.Context, meta CallMeta,
	mediaObjectID string) (MediaDownloadGrant, error) {
	if !c.Configured() {
		return MediaDownloadGrant{}, ErrNotConfigured
	}

	var envelope mediaEnvelope[MediaDownloadGrant]
	err := c.client.Exchange(ctx, meta, Exchange{
		Method: http.MethodPost,
		Path:   c.apiBase + "/objects/" + url.PathEscape(mediaObjectID) + "/download-authorizations",
		Out:    &envelope,
	})
	if err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) {
			switch clientErr.Outcome {
			case OutcomeNotFound:
				return MediaDownloadGrant{}, ErrMediaObjectNotFound
			case OutcomeForbidden:
				return MediaDownloadGrant{}, ErrMediaNotDownloadable
			}
		}
		return MediaDownloadGrant{}, err
	}
	if envelope.Data.Token == "" {
		return MediaDownloadGrant{}, &Error{Service: c.AuthorityService(),
			Outcome: OutcomeUnknown, Status: http.StatusOK, Message: "media returned no download grant"}
	}
	return envelope.Data, nil
}

// ReadContent streams object bytes against a grant.
//
// maxBytes bounds the read so a compromised or misbehaving authority cannot
// make fleet allocate an arbitrary amount of memory.
func (c *MediaFileClient) ReadContent(ctx context.Context, meta CallMeta,
	grant MediaDownloadGrant, maxBytes int64) ([]byte, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	var content []byte
	err := c.client.Exchange(ctx, meta, Exchange{
		Method:           http.MethodGet,
		Path:             c.apiBase + "/objects/" + url.PathEscape(grant.MediaObjectID) + "/content",
		Headers:          map[string]string{"X-Download-Token": grant.Token},
		RawOut:           &content,
		MaxResponseBytes: maxBytes,
	})
	if err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
			return nil, ErrMediaObjectNotFound
		}
		return nil, err
	}
	return content, nil
}

// RequestDeletion asks media to destroy the bytes.
//
// A timeout here produces OutcomeUnknown, which the caller must resolve with
// ReadDeletionState rather than assuming either outcome. Fleet cannot mark a
// document physically deleted on the strength of a request it never saw
// answered.
func (c *MediaFileClient) RequestDeletion(ctx context.Context, meta CallMeta,
	mediaObjectID string, reason string) (MediaDeletionOutcome, error) {
	if !c.Configured() {
		return MediaDeletionOutcome{}, ErrNotConfigured
	}
	if meta.IdempotencyKey == "" {
		return MediaDeletionOutcome{}, &Error{Service: c.AuthorityService(),
			Outcome: OutcomePermanent, Message: "idempotency key is required"}
	}

	fingerprint := MediaFingerprint(meta.OrganizationID, mediaObjectID, reason)
	var envelope mediaEnvelope[MediaDeletionOutcome]
	err := c.client.Exchange(ctx, meta, Exchange{
		Method:   http.MethodPost,
		Path:     c.apiBase + "/objects/" + url.PathEscape(mediaObjectID) + "/deletions",
		JSONBody: map[string]string{"reason": reason},
		Headers:  writeHeaders(meta, fingerprint, 1),
		Out:      &envelope,
	})
	if err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
			return MediaDeletionOutcome{}, ErrMediaObjectNotFound
		}
		return MediaDeletionOutcome{}, err
	}
	return envelope.Data, nil
}

// ReadDeletionState is the authoritative read-back that resolves an UNKNOWN
// deletion outcome.
func (c *MediaFileClient) ReadDeletionState(ctx context.Context, meta CallMeta,
	mediaObjectID string) (MediaDeletionState, error) {
	if !c.Configured() {
		return MediaDeletionState{}, ErrNotConfigured
	}
	var envelope mediaEnvelope[MediaDeletionState]
	err := c.client.Exchange(ctx, meta, Exchange{
		Method: http.MethodGet,
		Path:   c.apiBase + "/objects/" + url.PathEscape(mediaObjectID) + "/deletion",
		Out:    &envelope,
	})
	if err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
			return MediaDeletionState{}, ErrMediaObjectNotFound
		}
		return MediaDeletionState{}, err
	}
	return envelope.Data, nil
}

// SetRetention updates media-side retention and legal hold.
//
// Fleet's own legal hold governs fleet's scheduling; this propagates it to the
// only service that can actually refuse to destroy the bytes.
func (c *MediaFileClient) SetRetention(ctx context.Context, meta CallMeta, mediaObjectID string,
	retentionClass string, retentionUntil *time.Time, legalHold *bool) (MediaObjectState, error) {
	if !c.Configured() {
		return MediaObjectState{}, ErrNotConfigured
	}
	body := map[string]any{}
	if retentionClass != "" {
		body["retentionClass"] = retentionClass
	}
	if retentionUntil != nil {
		body["retentionUntil"] = retentionUntil.UTC().Format(time.RFC3339)
	}
	if legalHold != nil {
		body["legalHold"] = *legalHold
	}

	var envelope mediaEnvelope[MediaObjectState]
	err := c.client.Exchange(ctx, meta, Exchange{
		Method:   http.MethodPost,
		Path:     c.apiBase + "/objects/" + url.PathEscape(mediaObjectID) + "/retention",
		JSONBody: body,
		Out:      &envelope,
	})
	if err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
			return MediaObjectState{}, ErrMediaObjectNotFound
		}
		return MediaObjectState{}, err
	}
	return envelope.Data, nil
}

// Compile-time proof that the adapter satisfies the port.
var _ MediaAuthority = (*MediaFileClient)(nil)
