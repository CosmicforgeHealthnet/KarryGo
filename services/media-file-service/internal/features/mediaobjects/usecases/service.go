// Package usecases implements the authoritative media object contract.
//
// The contract is private: only allowlisted, HMAC-authenticated platform
// services reach it. What it guarantees to those callers is narrow and total —
// media decides what a file is, whether anything vouched for it, and whether
// the bytes still exist. A caller may disagree with that answer only by
// refusing to use the file; it may never overwrite it.
//
// Three rules run through every method here:
//
//  1. the client's claims about a file are compared against the bytes, never
//     substituted for them;
//  2. nothing is ever reported SAFE that was not actually confirmed safe, and
//     an unavailable scanner produces UNKNOWN rather than an assumption;
//  3. content leaves this service only through a short-lived, object-specific
//     grant that is returned once and never stored.
package usecases

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/media-file-service/internal/database"
	"cosmicforge/logistics/services/media-file-service/internal/events"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/validation"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	"cosmicforge/logistics/services/media-file-service/internal/platform/audit"
	"cosmicforge/logistics/services/media-file-service/internal/platform/ids"
)

// Errors the HTTP surface maps onto status codes.
var (
	ErrInvalid           = errors.New("media: request is invalid")
	ErrNotFound          = errors.New("media: object not found")
	ErrConflict          = errors.New("media: conflicting request")
	ErrAuthorizationUsed = errors.New("media: upload authorization has already been used")
	ErrAuthorizationGone = errors.New("media: upload authorization is expired or invalid")
	ErrUnavailable       = errors.New("media: storage is temporarily unavailable")
	ErrNotDownloadable   = errors.New("media: object cannot be released for download")
	ErrDeletionBlocked   = errors.New("media: deletion is blocked by retention policy")
)

// ValidationError carries a controlled failure code alongside the sentinel, so
// the caller learns which check refused the file rather than only that one did.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string { return e.Code + ": " + e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

func invalid(code string, message string) error {
	return &ValidationError{Code: code, Message: message}
}

// Scanner is the malware/safety port.
//
// There is no default implementation and no fake that returns SAFE. A
// deployment with no scanner leaves this nil, and every object it accepts
// stays PENDING or becomes UNKNOWN — which is what fails closed downstream.
type Scanner interface {
	// Scan reports a safety verdict for the supplied content. Returning an
	// error means the scanner could not answer, which becomes UNKNOWN.
	Scan(ctx context.Context, mediaObjectID string, content []byte) (ScanResult, error)
	// Name identifies the scanner in history and audit records.
	Name() string
}

// ScanResult is a scanner's verdict.
type ScanResult struct {
	// Safe is only honoured when the scanner actually inspected the content.
	Safe bool
	// Reason is a controlled code recorded on the object when Safe is false.
	Reason string
}

// Policy is the deployment's media policy.
type Policy struct {
	// MaxSizeBytes caps every upload regardless of what a grant asked for.
	MaxSizeBytes int64
	// AllowedMIMETypes is the service-wide allowlist. A grant may narrow it
	// but never widen it.
	AllowedMIMETypes []string
	// UploadAuthorizationTTL bounds how long a grant may be redeemed.
	UploadAuthorizationTTL time.Duration
	// DownloadTTL bounds a content grant.
	DownloadTTL time.Duration
	// ProhibitExecutableContent refuses recognised executable signatures.
	ProhibitExecutableContent bool
	// RequireSafeForDownload refuses content grants for anything not confirmed
	// SAFE. A deployment without a scanner that sets this true can serve no
	// private content at all, which is the correct fail-closed posture when
	// policy demands a clean verdict.
	RequireSafeForDownload bool
	// DefaultRetention is applied to new objects when the caller names none.
	DefaultRetention time.Duration
	// DataExportMaxBytes is the purpose-specific ceiling for DATA_EXPORT.
	// It is separate from MaxSizeBytes and deliberately not larger: a bulk
	// customer export should not be able to borrow a document allowance, and
	// raising the global limit for images should not silently raise this one.
	DataExportMaxBytes int64
}

func (p Policy) normalized() Policy {
	if p.MaxSizeBytes <= 0 {
		p.MaxSizeBytes = 25 * 1024 * 1024
	}
	if p.UploadAuthorizationTTL <= 0 {
		p.UploadAuthorizationTTL = 15 * time.Minute
	}
	if p.DownloadTTL <= 0 {
		p.DownloadTTL = 5 * time.Minute
	}
	if len(p.AllowedMIMETypes) == 0 {
		p.AllowedMIMETypes = []string{"application/pdf", "image/jpeg", "image/png", "image/webp"}
	}
	if p.DataExportMaxBytes <= 0 {
		p.DataExportMaxBytes = 20 * 1024 * 1024
	}
	if p.DataExportMaxBytes > p.MaxSizeBytes {
		// The purpose ceiling can be lower than the global one but never
		// higher, so no configuration mistake turns the export path into the
		// widest upload this service accepts.
		p.DataExportMaxBytes = p.MaxSizeBytes
	}
	return p
}

func (p Policy) allowsMIME(contentType string) bool {
	for _, allowed := range p.AllowedMIMETypes {
		if allowed == contentType {
			return true
		}
	}
	return false
}

// Options configure the service.
type Options struct {
	Pool    *pgxpool.Pool
	Objects *repositories.Repository
	Storage uploadclients.PrivateStorage
	Events  *events.DomainPublisher
	Audit   *audit.Recorder
	// Scanner is nil when the deployment has no malware provider.
	Scanner Scanner
	// DownloadSecret signs content grants. Mandatory: without it no grant can
	// be minted and private download is refused rather than made open.
	DownloadSecret []byte
	StorageBucket  string
	Policy         Policy
	Now            func() time.Time
}

// Service is the authoritative media object service.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	opts.Policy = opts.Policy.normalized()
	return &Service{opts: opts}
}

// Caller is the authenticated platform service making a private call.
type Caller struct {
	Service        string
	OrganizationID string
	CorrelationID  string
	CausationID    string
	RequestID      string
}

// AuthorizeUploadInput is the versioned upload-authorization request.
type AuthorizeUploadInput struct {
	Caller             Caller
	OwnerType          string
	OwnerReference     string
	DocumentCategory   string
	UploadPurpose      string
	Visibility         string
	FileName           string
	AllowedMIMETypes   []string
	MaxSizeBytes       int64
	RetentionClass     string
	IdempotencyKey     string
	RequestFingerprint string
	PayloadVersion     int

	// BindingType and BindingID say what this file is evidence FOR. They are
	// fixed here, at authorisation, because the object row is created here:
	// no later request can re-point a file at a different job.
	BindingType       string
	BindingID         string
	SecondaryBindings []models.Binding
}

// UploadGrant is everything the caller learns.
//
// Deliberately absent: the storage bucket, the object key, any credential and
// any permanent URL. The caller uploads by presenting UploadToken to this
// service's own completion route, so it never touches the storage backend.
type UploadGrant struct {
	AuthorizationID  string
	MediaObjectID    string
	UploadToken      string
	UploadPath       string
	ExpiresAt        time.Time
	MaxSizeBytes     int64
	AllowedMIMETypes []string
	Replayed         bool
}

// AuthorizeUpload issues a single-use grant to upload one file.
func (s *Service) AuthorizeUpload(ctx context.Context, input AuthorizeUploadInput) (*UploadGrant, error) {
	if err := s.validateAuthorizeInput(&input); err != nil {
		return nil, err
	}

	// A retry replays; a reused key describing a different request is refused,
	// because silently returning the first grant would hand the caller an
	// authorization for constraints it did not ask for.
	existing, err := s.opts.Objects.AuthorizationByIdempotencyKey(ctx, nil,
		input.Caller.Service, input.Caller.OrganizationID, input.IdempotencyKey)
	switch {
	case err == nil:
		if existing.RequestFingerprint != input.RequestFingerprint {
			return nil, fmt.Errorf("%w: idempotency key reused for a different request", ErrConflict)
		}
		// DATA_EXPORT upload retries have one additional safe outcome. If Media
		// durably completed the object but the caller lost the completion
		// response, the single-use authorization is already consumed. Return the
		// same opaque object identity without minting another upload token; the
		// caller can reconcile checksum, size, purpose and visibility through the
		// ordinary scoped object read. This is purpose-scoped so existing document
		// upload replay semantics remain unchanged.
		if existing.UploadPurpose == models.PurposeDataExport &&
			existing.State == models.AuthorizationConsumed {
			object, objectErr := s.opts.Objects.GetObjectForOwner(ctx, nil,
				input.Caller.Service, input.Caller.OrganizationID, existing.MediaObjectID)
			if objectErr == nil && object.UploadStatus == models.UploadCompleted &&
				object.DeletionState == models.DeletionActive &&
				object.UploadPurpose == models.PurposeDataExport &&
				object.Visibility == models.VisibilityPrivate &&
				object.ChecksumSHA256 != "" {
				return &UploadGrant{
					AuthorizationID:  existing.ID,
					MediaObjectID:    existing.MediaObjectID,
					UploadPath:       uploadPathFor(existing.MediaObjectID),
					ExpiresAt:        existing.ExpiresAt.UTC(),
					MaxSizeBytes:     existing.MaxSizeBytes,
					AllowedMIMETypes: existing.AllowedMIMETypes,
					Replayed:         true,
				}, nil
			}
			if objectErr != nil && !errors.Is(objectErr, repositories.ErrNotFound) {
				return nil, objectErr
			}
		}
		if !existing.Usable(s.opts.Now().UTC()) {
			return nil, fmt.Errorf("%w: authorization is no longer usable", ErrAuthorizationGone)
		}
		// The token is not stored, so a replay cannot return it. Re-mint one
		// bound to the same authorization: the grant is the row, the token is
		// only the bearer proof for it.
		token, tokenHash, mintErr := s.mintUploadToken()
		if mintErr != nil {
			return nil, mintErr
		}
		if _, updateErr := s.opts.Pool.Exec(ctx,
			`UPDATE media_upload_authorizations SET upload_token_hash = $1, updated_at = now()
			 WHERE id = $2 AND state = $3`, tokenHash, existing.ID, models.AuthorizationIssued); updateErr != nil {
			return nil, updateErr
		}
		return &UploadGrant{
			AuthorizationID:  existing.ID,
			MediaObjectID:    existing.MediaObjectID,
			UploadToken:      token,
			UploadPath:       uploadPathFor(existing.MediaObjectID),
			ExpiresAt:        existing.ExpiresAt.UTC(),
			MaxSizeBytes:     existing.MaxSizeBytes,
			AllowedMIMETypes: existing.AllowedMIMETypes,
			Replayed:         true,
		}, nil
	case !errors.Is(err, repositories.ErrNotFound):
		return nil, err
	}

	now := s.opts.Now().UTC()
	objectID := ids.New(ids.PrefixMediaObject)
	token, tokenHash, err := s.mintUploadToken()
	if err != nil {
		return nil, err
	}

	object := &models.Object{
		ID:             objectID,
		OrganizationID: input.Caller.OrganizationID,
		OwnerService:   input.Caller.Service,
		OwnerType:      input.OwnerType,
		OwnerReference: input.OwnerReference,
		UploadPurpose:  input.UploadPurpose,
		Visibility:     input.Visibility,
		FileName:       input.FileName,
		UploadStatus:   models.UploadAuthorized,
		SafetyStatus:   models.SafetyPending,
		StorageBucket:  s.opts.StorageBucket,
		// The key is minted here and never negotiated. It embeds the tenant and
		// the object id so two tenants cannot collide and an operator reading a
		// bucket listing can tell what they are looking at.
		StorageObjectKey: storageKeyFor(input.Caller.OrganizationID, input.UploadPurpose, objectID),
		RetentionClass:   input.RetentionClass,
		CorrelationID:    input.Caller.CorrelationID,
		CausationID:      input.Caller.CausationID,
		BindingType:      input.BindingType,
		BindingID:        input.BindingID,
		EvidenceState:    models.EvidenceActive,
	}
	if s.opts.Policy.DefaultRetention > 0 {
		until := now.Add(s.opts.Policy.DefaultRetention)
		object.RetentionUntil = &until
	}

	authorization := &models.UploadAuthorization{
		ID:                 ids.New(ids.PrefixUploadAuth),
		MediaObjectID:      objectID,
		OrganizationID:     input.Caller.OrganizationID,
		CallerService:      input.Caller.Service,
		OwnerService:       input.Caller.Service,
		OwnerType:          input.OwnerType,
		OwnerReference:     input.OwnerReference,
		DocumentCategory:   input.DocumentCategory,
		UploadPurpose:      input.UploadPurpose,
		Visibility:         input.Visibility,
		FileName:           input.FileName,
		AllowedMIMETypes:   input.AllowedMIMETypes,
		MaxSizeBytes:       input.MaxSizeBytes,
		UploadTokenHash:    tokenHash,
		State:              models.AuthorizationIssued,
		ExpiresAt:          now.Add(s.opts.Policy.UploadAuthorizationTTL),
		IdempotencyKey:     input.IdempotencyKey,
		RequestFingerprint: input.RequestFingerprint,
		CorrelationID:      input.Caller.CorrelationID,
		CausationID:        input.Caller.CausationID,
		PayloadVersion:     input.PayloadVersion,
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if createErr := s.opts.Objects.CreateObject(ctx, tx, object); createErr != nil {
			return createErr
		}
		// Written in the same transaction as the object, so the complete set
		// of bindings exists before a single byte is accepted.
		if bindErr := s.opts.Objects.AddSecondaryBindings(ctx, tx, object.ID,
			input.SecondaryBindings); bindErr != nil {
			return bindErr
		}
		if createErr := s.opts.Objects.CreateAuthorization(ctx, tx, authorization); createErr != nil {
			if errors.Is(createErr, repositories.ErrConflict) {
				return fmt.Errorf("%w: authorization already exists", ErrConflict)
			}
			return createErr
		}
		if histErr := s.opts.Objects.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: object.OrganizationID,
			MediaObjectID:  object.ID,
			Action:         models.ActionAuthorized,
			ToState:        models.UploadAuthorized,
			ActorType:      "SERVICE",
			ActorID:        input.Caller.Service,
			Metadata: map[string]any{
				"purpose":      object.UploadPurpose,
				"maxSizeBytes": authorization.MaxSizeBytes,
				"category":     authorization.DocumentCategory,
			},
			CorrelationID: input.Caller.CorrelationID,
			CausationID:   input.Caller.CausationID,
		}); histErr != nil {
			return histErr
		}
		return s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: object.OrganizationID,
			ActorType:      "SERVICE",
			ActorID:        input.Caller.Service,
			Action:         "media.upload.authorized",
			ResourceType:   "media_object",
			ResourceID:     object.ID,
			NewValues: map[string]any{
				"purpose":      object.UploadPurpose,
				"ownerType":    object.OwnerType,
				"maxSizeBytes": authorization.MaxSizeBytes,
			},
			RequestID:     input.Caller.RequestID,
			CorrelationID: input.Caller.CorrelationID,
		})
	})
	if err != nil {
		return nil, err
	}

	return &UploadGrant{
		AuthorizationID:  authorization.ID,
		MediaObjectID:    object.ID,
		UploadToken:      token,
		UploadPath:       uploadPathFor(object.ID),
		ExpiresAt:        authorization.ExpiresAt,
		MaxSizeBytes:     authorization.MaxSizeBytes,
		AllowedMIMETypes: authorization.AllowedMIMETypes,
	}, nil
}

func (s *Service) validateAuthorizeInput(input *AuthorizeUploadInput) error {
	input.UploadPurpose = strings.ToUpper(strings.TrimSpace(input.UploadPurpose))
	input.OwnerType = strings.ToUpper(strings.TrimSpace(input.OwnerType))
	input.RetentionClass = strings.ToUpper(strings.TrimSpace(input.RetentionClass))
	input.Visibility = strings.ToUpper(strings.TrimSpace(input.Visibility))
	input.FileName = strings.TrimSpace(input.FileName)
	if input.Visibility == "" {
		input.Visibility = models.VisibilityPrivate
	}
	if input.RetentionClass == "" {
		// The default follows the purpose, so a caller that forgot to say
		// "this is a safety incident" does not have it expire on the ordinary
		// schedule.
		input.RetentionClass = models.DefaultRetentionFor(input.UploadPurpose)
	}
	input.BindingType = strings.ToUpper(strings.TrimSpace(input.BindingType))
	input.BindingID = strings.TrimSpace(input.BindingID)
	if input.PayloadVersion <= 0 {
		input.PayloadVersion = 1
	}

	switch {
	case input.Caller.Service == "":
		return invalid("CALLER_REQUIRED", "An authenticated caller is required.")
	case input.Caller.OrganizationID == "":
		return invalid("ORGANIZATION_REQUIRED", "An organization id is required.")
	case !models.ValidPurpose(input.UploadPurpose):
		return invalid("PURPOSE_UNSUPPORTED", "That upload purpose is not supported.")
	case input.OwnerReference == "":
		return invalid("OWNER_REQUIRED", "An owner reference is required.")
	case input.IdempotencyKey == "":
		return invalid("IDEMPOTENCY_KEY_REQUIRED", "An idempotency key is required.")
	case input.RequestFingerprint == "":
		return invalid("FINGERPRINT_REQUIRED", "A request fingerprint is required.")
	case !models.ValidVisibility(input.Visibility):
		return invalid("VISIBILITY_UNSUPPORTED", "That visibility is not supported.")
	case !models.ValidRetentionClass(input.RetentionClass):
		return invalid("RETENTION_UNSUPPORTED", "That retention class is not supported.")
	}
	if input.UploadPurpose == models.PurposeProductImage || input.UploadPurpose == models.PurposeBrandImage || input.UploadPurpose == models.PurposePastryReference {
		if input.Caller.Service != "vendor-service" || input.OwnerType != "VENDOR_BUSINESS" || input.OwnerReference != input.Caller.OrganizationID || input.BindingType != models.BindingAsset || input.BindingID == "" {
			return invalid("VENDOR_ASSET_SCOPE_REQUIRED", "Vendor assets require a scoped record binding.")
		}
		if input.MaxSizeBytes <= 0 || input.MaxSizeBytes > 10<<20 {
			return invalid("VENDOR_ASSET_SIZE", "Vendor assets must be at most 10 MB.")
		}
		for _, mime := range input.AllowedMIMETypes {
			if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" && !(input.UploadPurpose == models.PurposePastryReference && mime == "application/pdf") {
				return invalid("VENDOR_ASSET_MIME", "Unsupported vendor asset type.")
			}
		}
	}
	if err := validateBindings(input); err != nil {
		return err
	}

	// A generated data export is customer data in bulk. It is refused here as
	// well as by a database CHECK, so neither a caller nor a future endpoint
	// can make one publicly reachable — and the refusal does not depend on the
	// requesting service having been careful.
	if models.RequiresPrivateVisibility(input.UploadPurpose) &&
		input.Visibility != models.VisibilityPrivate {
		return invalid("VISIBILITY_MUST_BE_PRIVATE",
			"That upload purpose may only be stored privately.")
	}
	// A binding type that exists only for an administrative source case is
	// case evidence and may never be publicly reachable.
	if input.Visibility != models.VisibilityPrivate {
		caseBound := models.CaseOnlyBindingType(input.BindingType)
		for _, binding := range input.SecondaryBindings {
			caseBound = caseBound || models.CaseOnlyBindingType(binding.Type)
		}
		if caseBound {
			return invalid("VISIBILITY_MUST_BE_PRIVATE", "Case evidence may only be stored privately.")
		}
	}
	// The export format is settled before any byte is accepted: the declared
	// type must be one of exactly two, and the file name must carry the one
	// extension that pairs with it. Deciding this at authorization time means
	// an unusable grant is never issued.
	if input.UploadPurpose == models.PurposeDataExport {
		canonical, failure := validation.ExportPairing(firstMIME(input.AllowedMIMETypes), input.FileName)
		if failure != "" {
			return invalid(failure, "That export type, content type and file name do not agree.")
		}
		input.AllowedMIMETypes = []string{canonical}
		if input.MaxSizeBytes <= 0 || input.MaxSizeBytes > s.opts.Policy.DataExportMaxBytes {
			input.MaxSizeBytes = s.opts.Policy.DataExportMaxBytes
		}
		return nil
	}

	// The grant may narrow the service allowlist; it may never widen it.
	normalized := make([]string, 0, len(input.AllowedMIMETypes))
	for _, candidate := range input.AllowedMIMETypes {
		value := validation.NormalizeMIME(candidate)
		if value == "" {
			continue
		}
		if !s.opts.Policy.allowsMIME(value) {
			return invalid("MIME_NOT_PERMITTED",
				"One of the requested content types is not permitted by policy.")
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return invalid("MIME_REQUIRED", "At least one permitted content type is required.")
	}
	input.AllowedMIMETypes = normalized

	if input.MaxSizeBytes <= 0 || input.MaxSizeBytes > s.opts.Policy.MaxSizeBytes {
		input.MaxSizeBytes = s.opts.Policy.MaxSizeBytes
	}
	return nil
}

// CompleteUploadInput carries the bytes and the caller's claims about them.
type CompleteUploadInput struct {
	Caller          Caller
	MediaObjectID   string
	AuthorizationID string
	UploadToken     string
	DeclaredMIME    string
	DeclaredSize    int64
	// DeclaredChecksum is the caller's SHA-256 of what it believes it sent.
	// It is compared against the stored bytes, never trusted in their place.
	DeclaredChecksum string
	Content          []byte
}

// CompleteCustomerUpload permits a phone to redeem only an already-minted,
// customer-owned grant. The bearer token remains the only upload capability;
// it cannot name another service, tenant, object, or business binding.
func (s *Service) CompleteCustomerUpload(ctx context.Context, input CompleteUploadInput) (*models.Object, error) {
	authorization, err := s.opts.Objects.AuthorizationByID(ctx, input.AuthorizationID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if authorization.CallerService != "customer-service" ||
		authorization.OwnerService != "customer-service" ||
		authorization.OwnerType != "CUSTOMER" ||
		authorization.UploadPurpose != models.PurposeDisputeAttachment ||
		!strings.HasPrefix(authorization.OrganizationID, "customer_") ||
		strings.TrimPrefix(authorization.OrganizationID, "customer_") != authorization.OwnerReference {
		return nil, ErrNotFound
	}
	input.Caller = Caller{Service: "customer-service", OrganizationID: authorization.OrganizationID}
	return s.CompleteUpload(ctx, input)
}

func (s *Service) CompleteVendorUpload(ctx context.Context, input CompleteUploadInput) (*models.Object, error) {
	authorization, err := s.opts.Objects.AuthorizationByID(ctx, input.AuthorizationID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !vendorUploadAuthorization(authorization) {
		return nil, ErrNotFound
	}
	if authorization.UploadPurpose != models.PurposeComplianceDocument {
		object, readErr := s.GetObject(ctx, Caller{Service: "vendor-service", OrganizationID: authorization.OrganizationID}, authorization.MediaObjectID)
		if readErr != nil || object.BindingType != models.BindingAsset || object.BindingID == "" || object.UploadPurpose != authorization.UploadPurpose {
			return nil, ErrNotFound
		}
	}
	input.Caller = Caller{Service: "vendor-service", OrganizationID: authorization.OrganizationID}
	return s.CompleteUpload(ctx, input)
}

func vendorUploadAuthorization(a *models.UploadAuthorization) bool {
	if a == nil || a.CallerService != "vendor-service" || a.OwnerService != "vendor-service" || a.OwnerType != "VENDOR_BUSINESS" || a.OwnerReference == "" || a.OrganizationID != a.OwnerReference || a.Visibility != models.VisibilityPrivate {
		return false
	}
	if a.UploadPurpose == models.PurposeComplianceDocument {
		return true
	}
	return (a.UploadPurpose == models.PurposeProductImage || a.UploadPurpose == models.PurposeBrandImage || a.UploadPurpose == models.PurposePastryReference)
}

// CompleteUpload stores and validates the bytes for one authorized upload.
//
// Order matters. The grant is redeemed first (single-use, under a row lock),
// then the bytes are written, then everything about them is re-derived from
// what was actually stored. A validation failure marks the object FAILED,
// records why, removes the bytes and emits media.validation_failed.v1 — the
// object is never left in a state where a consumer could mistake it for usable.
func (s *Service) CompleteUpload(ctx context.Context, input CompleteUploadInput) (*models.Object, error) {
	if input.MediaObjectID == "" || input.AuthorizationID == "" || input.UploadToken == "" {
		return nil, invalid("AUTHORIZATION_REQUIRED", "An upload authorization is required.")
	}

	now := s.opts.Now().UTC()

	// Phase one: redeem the grant. Committing this separately is deliberate —
	// once it commits the authorization is spent, so a crash mid-upload cannot
	// leave a grant that a second caller could redeem with different bytes.
	var authorization *models.UploadAuthorization
	var object *models.Object
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		found, lockErr := s.opts.Objects.LockAuthorization(ctx, tx, input.AuthorizationID)
		if errors.Is(lockErr, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if lockErr != nil {
			return lockErr
		}
		if found.MediaObjectID != input.MediaObjectID ||
			found.CallerService != input.Caller.Service ||
			found.OrganizationID != input.Caller.OrganizationID {
			// A grant that belongs to a different caller, tenant or object is
			// reported as absent, not as forbidden.
			return ErrNotFound
		}
		if found.State == models.AuthorizationConsumed {
			return ErrAuthorizationUsed
		}
		if !found.Usable(now) {
			return ErrAuthorizationGone
		}
		if !verifyUploadToken(input.UploadToken, found.UploadTokenHash) {
			return ErrNotFound
		}

		locked, objectErr := s.opts.Objects.LockObject(ctx, tx, found.MediaObjectID)
		if objectErr != nil {
			return objectErr
		}
		if locked.UploadStatus == models.UploadCompleted {
			return ErrAuthorizationUsed
		}

		if stateErr := s.opts.Objects.SetAuthorizationState(ctx, tx, found.ID,
			models.AuthorizationConsumed, &now); stateErr != nil {
			return stateErr
		}
		locked.UploadStatus = models.UploadUploading
		if updateErr := s.opts.Objects.UpdateObject(ctx, tx, locked, locked.Version); updateErr != nil {
			return updateErr
		}
		if histErr := s.opts.Objects.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: locked.OrganizationID,
			MediaObjectID:  locked.ID,
			Action:         models.ActionAuthorizationUsed,
			FromState:      models.UploadAuthorized,
			ToState:        models.UploadUploading,
			ActorType:      "SERVICE",
			ActorID:        input.Caller.Service,
			CorrelationID:  input.Caller.CorrelationID,
			CausationID:    input.Caller.CausationID,
		}); histErr != nil {
			return histErr
		}
		authorization = found
		object = locked
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Phase two: validate against what was actually received, before anything
	// is written to storage. Nothing here consults the filename or extension.
	failure := s.validateContent(authorization, input)
	if failure != "" {
		if failErr := s.failUpload(ctx, object, failure, input.Caller); failErr != nil {
			return nil, failErr
		}
		return nil, invalid(failure, validationMessage(failure))
	}

	detection := s.detect(input)
	if authorization.UploadPurpose == models.PurposeDataExport {
		detection, _ = s.validateDataExport(authorization, input)
	}
	checksum := validation.Checksum(input.Content)

	if err := s.opts.Storage.Put(ctx, object.StorageObjectKey, detection.DetectedMIME, input.Content); err != nil {
		if failErr := s.failUpload(ctx, object, models.FailureIncompleteUpload, input.Caller); failErr != nil {
			return nil, failErr
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	// Re-read what storage actually holds. Comparing the request against
	// itself would prove nothing; comparing it against the stored object is
	// what catches a truncated write.
	storedSize, err := s.opts.Storage.Stat(ctx, object.StorageObjectKey)
	if err != nil || storedSize != int64(len(input.Content)) {
		_ = s.opts.Storage.Delete(ctx, object.StorageObjectKey)
		if failErr := s.failUpload(ctx, object, models.FailureIncompleteUpload, input.Caller); failErr != nil {
			return nil, failErr
		}
		return nil, invalid(models.FailureIncompleteUpload, validationMessage(models.FailureIncompleteUpload))
	}

	safetyStatus, safetyReason, scannedAt := s.scan(ctx, object.ID, input.Content)

	object.DetectedMIME = detection.DetectedMIME
	object.DeclaredMIME = validation.NormalizeMIME(input.DeclaredMIME)
	object.SizeBytes = int64(len(input.Content))
	object.ChecksumSHA256 = checksum
	object.CapturedAt = &now
	object.SourceTimestamp = &now
	object.CaptureMethod = "MEDIA_VALIDATED_UPLOAD"
	// Media does not accept capture metadata from the Fleet caller. The actor
	// is the authenticated service identity and no client device identifier is
	// stored because this private upload contract cannot attest one.
	object.ActorReference = input.Caller.Service
	object.DeviceReference = ""
	object.OriginalMIMEType = object.DeclaredMIME
	if object.OriginalMIMEType == "" {
		// The use-case API permits callers that do not make a MIME claim. Media
		// still has an authoritative value after signature detection, and the
		// capture-metadata invariant requires that value to be preserved.
		object.OriginalMIMEType = object.DetectedMIME
	}
	object.OriginalSizeBytes = storedSize
	object.IntegrityHash = checksum
	object.SignatureValidation = models.SignaturePassed
	object.UploadStatus = models.UploadCompleted
	object.SafetyStatus = safetyStatus
	object.SafetyReason = safetyReason
	object.ScannedAt = scannedAt
	object.ValidationFailureCode = ""

	quarantined := safetyStatus == models.SafetyUnsafe
	if quarantined {
		// Unsafe content is quarantined rather than left retrievable. The bytes
		// are removed here because there is no legitimate consumer for them and
		// keeping them only creates a way to serve malware later.
		_ = s.opts.Storage.Delete(ctx, object.StorageObjectKey)
		object.SafetyStatus = models.SafetyQuarantined
	}

	expectedVersion := object.Version
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if updateErr := s.opts.Objects.UpdateObject(ctx, tx, object, expectedVersion); updateErr != nil {
			return updateErr
		}
		if histErr := s.opts.Objects.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: object.OrganizationID,
			MediaObjectID:  object.ID,
			Action:         models.ActionUploadCompleted,
			FromState:      models.UploadUploading,
			ToState:        models.UploadCompleted,
			ActorType:      "SERVICE",
			ActorID:        input.Caller.Service,
			Metadata: map[string]any{
				"detectedMime": object.DetectedMIME,
				"sizeBytes":    object.SizeBytes,
				"safetyStatus": object.SafetyStatus,
			},
			CorrelationID: input.Caller.CorrelationID,
			CausationID:   input.Caller.CausationID,
		}); histErr != nil {
			return histErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: object.OrganizationID,
			ActorType:      "SERVICE",
			ActorID:        input.Caller.Service,
			Action:         "media.upload.completed",
			ResourceType:   "media_object",
			ResourceID:     object.ID,
			NewValues: map[string]any{
				"detectedMime": object.DetectedMIME,
				"checksum":     object.ChecksumSHA256,
				"sizeBytes":    object.SizeBytes,
				"safetyStatus": object.SafetyStatus,
			},
			RequestID:     input.Caller.RequestID,
			CorrelationID: input.Caller.CorrelationID,
		}); auditErr != nil {
			return auditErr
		}

		if publishErr := s.publish(ctx, tx, events.EventUploadCompleted, object, input.Caller, nil); publishErr != nil {
			return publishErr
		}
		if publishErr := s.publish(ctx, tx, events.EventAssetStatusChanged, object, input.Caller, nil); publishErr != nil {
			return publishErr
		}
		if publishErr := s.publish(ctx, tx, events.EventValidationPassed, object, input.Caller, nil); publishErr != nil {
			return publishErr
		}
		if object.ScannedAt != nil {
			if publishErr := s.publish(ctx, tx, events.EventScanCompleted, object, input.Caller, nil); publishErr != nil {
				return publishErr
			}
		}
		if quarantined {
			if publishErr := s.publish(ctx, tx, events.EventUnsafeDetected, object, input.Caller, nil); publishErr != nil {
				return publishErr
			}
			return s.publish(ctx, tx, events.EventQuarantined, object, input.Caller, nil)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return object, nil
}

// detect classifies the received bytes for a document purpose.
//
// Signature detection is the rule everywhere except DATA_EXPORT, which has its
// own path below because CSV and JSONL carry no signature to match.
func (s *Service) detect(input CompleteUploadInput) validation.Detection {
	return validation.Detect(validation.Head(input.Content))
}

// validateDataExport is the narrow, purpose-scoped content gate for generated
// data exports.
//
// It is an ADDITION to the safety model, not a relaxation of it. Nothing here
// trusts the declared content type: the declaration selects which claim to
// verify, and then the bytes have to actually be that thing. In order:
//
//   - the declared type must be exactly text/csv or newline-delimited JSON,
//     and the file name must carry the one extension that pairs with it;
//   - the artifact must be within the purpose-specific ceiling, which is
//     separate from and never larger than the global one;
//   - anything carrying a recognised signature is refused outright, because a
//     file with a signature is by definition not a sniff-less text format —
//     this is what stops a PDF, PNG or ZIP arriving as text/csv;
//   - executable and archive signatures are refused with their real reason;
//   - and the content is then streamed through the format's own parser, so a
//     CSV must actually parse as CSV with a consistent column count and every
//     JSONL line must actually be one JSON value.
//
// HTML, SVG, JavaScript and Markdown fail at the first step: none of them is
// an accepted declared type, and none would parse as CSV or JSONL if it were.
func (s *Service) validateDataExport(authorization *models.UploadAuthorization,
	input CompleteUploadInput) (validation.Detection, string) {
	canonical, failure := validation.ExportPairing(input.DeclaredMIME, authorization.FileName)
	if failure != "" {
		return validation.Detection{}, failure
	}
	ceiling := s.opts.Policy.DataExportMaxBytes
	if authorization.MaxSizeBytes > 0 && authorization.MaxSizeBytes < ceiling {
		ceiling = authorization.MaxSizeBytes
	}
	if int64(len(input.Content)) > ceiling {
		return validation.Detection{}, validation.ExportFailureTooLarge
	}
	head := validation.Head(input.Content)
	if validation.ExportExecutableSignature(head) {
		return validation.Detection{Executable: true}, validation.ExportFailureExecutable
	}
	if validation.ExportBinarySignature(head) {
		return validation.Detection{}, validation.ExportFailureUnsupportedType
	}
	if validation.ExportMarkupSignature(head) {
		// Structural parsing alone would accept a one-line HTML page as a
		// single-column CSV. An export never opens with markup.
		return validation.Detection{}, validation.ExportFailureUnsupportedType
	}
	// Streamed over the received bytes with a bounded line buffer: the parser
	// and the character rules see the content once, and no second full copy is
	// made to check it.
	if failure = validation.ValidateExportContent(bytes.NewReader(input.Content),
		canonical, ceiling); failure != "" {
		return validation.Detection{}, failure
	}
	// The stored type is the canonical one, so an accepted alias spelling is
	// never what gets served back.
	return validation.Detection{DetectedMIME: canonical}, ""
}

// validateContent compares the received bytes against the grant and policy.
//
// Returns a controlled failure code, or "" when everything holds.
func (s *Service) validateContent(authorization *models.UploadAuthorization,
	input CompleteUploadInput) string {
	if len(input.Content) == 0 {
		return models.FailureEmptyUpload
	}
	size := int64(len(input.Content))
	if size > authorization.MaxSizeBytes || size > s.opts.Policy.MaxSizeBytes {
		return models.FailureSizeExceeded
	}
	// A declared size that disagrees with the bytes means the transfer did not
	// deliver what the caller sent.
	if input.DeclaredSize > 0 && input.DeclaredSize != size {
		return models.FailureIncompleteUpload
	}
	if input.DeclaredChecksum != "" &&
		subtle.ConstantTimeCompare([]byte(strings.ToLower(input.DeclaredChecksum)),
			[]byte(validation.Checksum(input.Content))) != 1 {
		return models.FailureChecksumMismatch
	}

	if authorization.UploadPurpose == models.PurposeDataExport {
		detection, failure := s.validateDataExport(authorization, input)
		if detection.Executable && s.opts.Policy.ProhibitExecutableContent {
			return models.FailureExecutableContent
		}
		return failure
	}

	detection := s.detect(input)
	if detection.Executable && s.opts.Policy.ProhibitExecutableContent {
		return models.FailureExecutableContent
	}
	if detection.DetectedMIME == "" {
		// Unrecognised leading bytes. Accepting on the strength of the declared
		// type is exactly the hole this check exists to close.
		return models.FailureSignatureMismatch
	}
	if !s.opts.Policy.allowsMIME(detection.DetectedMIME) ||
		!authorization.AllowsMIME(detection.DetectedMIME) {
		return models.FailureUnsupportedType
	}
	declared := validation.NormalizeMIME(input.DeclaredMIME)
	if declared != "" && declared != detection.DetectedMIME {
		return models.FailureMIMEMismatch
	}
	return ""
}

func validationMessage(code string) string {
	switch code {
	case models.FailureSizeExceeded:
		return "The file is larger than this upload permits."
	case models.FailureMIMEMismatch:
		return "The file's content does not match the declared content type."
	case models.FailureSignatureMismatch:
		return "The file's content could not be recognised."
	case models.FailureChecksumMismatch:
		return "The file's checksum does not match the uploaded bytes."
	case models.FailureIncompleteUpload:
		return "The upload did not complete."
	case models.FailureUnsupportedType:
		return "That file type is not supported for this upload."
	case models.FailureExecutableContent:
		return "Executable content is not permitted."
	case models.FailureEmptyUpload:
		return "The file is empty."
	default:
		return "The file could not be accepted."
	}
}

// failUpload records a validation failure and emits the failure event.
func (s *Service) failUpload(ctx context.Context, object *models.Object, code string, caller Caller) error {
	expectedVersion := object.Version
	object.UploadStatus = models.UploadFailed
	object.SignatureValidation = models.SignatureFailed
	object.ValidationFailureCode = code
	object.SafetyStatus = models.SafetyUnknown

	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Objects.UpdateObject(ctx, tx, object, expectedVersion); err != nil {
			return err
		}
		if err := s.opts.Objects.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: object.OrganizationID,
			MediaObjectID:  object.ID,
			Action:         models.ActionValidationFailed,
			FromState:      models.UploadUploading,
			ToState:        models.UploadFailed,
			ActorType:      "SERVICE",
			ActorID:        caller.Service,
			Metadata:       map[string]any{"failureCode": code},
			CorrelationID:  caller.CorrelationID,
			CausationID:    caller.CausationID,
		}); err != nil {
			return err
		}
		if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: object.OrganizationID,
			ActorType:      "SERVICE",
			ActorID:        caller.Service,
			Action:         "media.upload.validation_failed",
			ResourceType:   "media_object",
			ResourceID:     object.ID,
			NewValues:      map[string]any{"failureCode": code},
			RequestID:      caller.RequestID,
			CorrelationID:  caller.CorrelationID,
		}); err != nil {
			return err
		}
		if err := s.publish(ctx, tx, events.EventValidationFailed, object, caller,
			map[string]any{"failureCode": code}); err != nil {
			return err
		}
		return s.publish(ctx, tx, events.EventAssetStatusChanged, object, caller,
			map[string]any{"failureCode": code})
	})
}

// scan asks the configured scanner for a verdict.
//
// No scanner means PENDING: nothing looked, and the object says so. A scanner
// that errors means UNKNOWN: something looked and could not answer. Neither is
// ever reported SAFE.
func (s *Service) scan(ctx context.Context, mediaObjectID string, content []byte) (string, string, *time.Time) {
	if s.opts.Scanner == nil {
		return models.SafetyPending, "", nil
	}
	result, err := s.opts.Scanner.Scan(ctx, mediaObjectID, content)
	scannedAt := s.opts.Now().UTC()
	if err != nil {
		return models.SafetyUnknown, "SCANNER_UNAVAILABLE", &scannedAt
	}
	if !result.Safe {
		reason := result.Reason
		if reason == "" {
			reason = "SCANNER_FLAGGED"
		}
		return models.SafetyUnsafe, reason, &scannedAt
	}
	return models.SafetySafe, "", &scannedAt
}

// GetObject reads one object inside the caller's service and tenant scope.
func (s *Service) GetObject(ctx context.Context, caller Caller, id string) (*models.Object, error) {
	object, err := s.opts.Objects.GetObjectForOwner(ctx, nil, caller.Service, caller.OrganizationID, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = s.loadBindings(ctx, object); err != nil {
		return nil, err
	}
	return object, nil
}

func (s *Service) loadBindings(ctx context.Context, object *models.Object) error {
	bindings, err := s.opts.Objects.SecondaryBindings(ctx, nil, object.ID)
	if err != nil {
		return err
	}
	object.SecondaryBindings = bindings
	return nil
}

// History returns an object's append-only history inside the caller's scope.
func (s *Service) History(ctx context.Context, caller Caller, id string) ([]models.HistoryEntry, error) {
	if _, err := s.GetObject(ctx, caller, id); err != nil {
		return nil, err
	}
	return s.opts.Objects.ListHistory(ctx, nil, id)
}

// DownloadGrant is a short-lived, object-specific content grant.
//
// The token exists only in this struct and in the response that carries it. It
// is not written to the database, not written to history, and not logged.
type DownloadGrant struct {
	MediaObjectID string
	Token         string
	Path          string
	ExpiresAt     time.Time
	ContentType   string
	SizeBytes     int64
}

// AuthorizeDownload mints a content grant for one object.
//
// Media's own floor applies first and cannot be waived by the caller: the
// upload must be COMPLETED, the bytes must still exist, and the object must
// not be UNSAFE or QUARANTINED. When policy requires a clean verdict, anything
// not confirmed SAFE — including PENDING and UNKNOWN — is refused.
func (s *Service) AuthorizeDownload(ctx context.Context, caller Caller, id string) (*DownloadGrant, error) {
	if len(s.opts.DownloadSecret) == 0 {
		// Without a signing secret there is no way to mint a bounded grant.
		// Refusing is the only safe answer; serving content unsigned would
		// replace a short-lived credential with none at all.
		return nil, fmt.Errorf("%w: download signing is not configured", ErrNotDownloadable)
	}
	object, err := s.GetObject(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	if !object.Downloadable() {
		return nil, fmt.Errorf("%w: object state %s/%s/%s", ErrNotDownloadable,
			object.UploadStatus, object.SafetyStatus, object.DeletionState)
	}
	now := s.opts.Now().UTC()
	if object.AccessExpired(now) {
		return nil, fmt.Errorf("%w: export retention elapsed", ErrNotDownloadable)
	}
	// High-risk purposes fail closed regardless of the deployment flag.
	if models.RequiresSafeVerdict(object.UploadPurpose, s.opts.Policy.RequireSafeForDownload) && !object.SafetyConfirmed() {
		return nil, fmt.Errorf("%w: safety status %s", ErrNotDownloadable, object.SafetyStatus)
	}

	expiresAt := now.Add(s.opts.Policy.DownloadTTL)
	if object.UploadPurpose == models.PurposeDataExport && object.RetentionUntil != nil &&
		object.RetentionUntil.Before(expiresAt) {
		expiresAt = object.RetentionUntil.UTC()
	}
	token := s.signDownloadToken(object.ID, caller.Service, expiresAt)

	// The audit record names the grant without recording the token. That is the
	// difference between evidence that access was authorized and a stored
	// credential anybody with database access could replay.
	if err := s.opts.Audit.Record(ctx, s.opts.Pool, audit.Entry{
		OrganizationID: object.OrganizationID,
		ActorType:      "SERVICE",
		ActorID:        caller.Service,
		Action:         "media.download.authorized",
		ResourceType:   "media_object",
		ResourceID:     object.ID,
		NewValues: map[string]any{
			"expiresAt":    expiresAt.Format(time.RFC3339),
			"safetyStatus": object.SafetyStatus,
		},
		RequestID:     caller.RequestID,
		CorrelationID: caller.CorrelationID,
	}); err != nil {
		return nil, err
	}

	return &DownloadGrant{
		MediaObjectID: object.ID,
		Token:         token,
		Path:          contentPathFor(object.ID),
		ExpiresAt:     expiresAt,
		ContentType:   object.DetectedMIME,
		SizeBytes:     object.SizeBytes,
	}, nil
}

// ReadContent serves object bytes against a grant token.
func (s *Service) ReadContent(ctx context.Context, id string, token string) ([]byte, *models.Object, error) {
	object, err := s.opts.Objects.GetObject(ctx, nil, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if !s.verifyDownloadToken(object, token) {
		// An invalid or expired token answers not-found: a caller holding a
		// lapsed grant must not be able to confirm the object still exists.
		return nil, nil, ErrNotFound
	}
	if !object.Downloadable() {
		return nil, nil, ErrNotDownloadable
	}
	// A previously issued token does not freeze the scanner verdict or the
	// current download policy. Recheck the same SAFE floor before reading bytes.
	if models.RequiresSafeVerdict(object.UploadPurpose, s.opts.Policy.RequireSafeForDownload) && !object.SafetyConfirmed() {
		return nil, nil, ErrNotDownloadable
	}
	if object.AccessExpired(s.opts.Now().UTC()) {
		return nil, nil, ErrNotDownloadable
	}
	content, err := s.opts.Storage.Get(ctx, object.StorageObjectKey)
	if errors.Is(err, uploadclients.ErrObjectMissing) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return content, object, nil
}

// RequestDeletionInput is the durable deletion request.
type RequestDeletionInput struct {
	Caller             Caller
	MediaObjectID      string
	Reason             string
	IdempotencyKey     string
	RequestFingerprint string
}

// RequestDeletion schedules and, when policy permits, performs physical
// deletion.
//
// Legal hold and an unexpired retention window both block. A blocked request
// is recorded as BLOCKED with its reason rather than retried forever, and the
// object stays ACTIVE — Fleet learns that the bytes were not destroyed and
// why, which is the only answer that lets it keep an honest projection.
func (s *Service) RequestDeletion(ctx context.Context, input RequestDeletionInput) (*models.DeletionRequest, error) {
	if input.MediaObjectID == "" || input.IdempotencyKey == "" || input.RequestFingerprint == "" {
		return nil, invalid("DELETION_REQUEST_INVALID", "A deletion request requires an object, key and fingerprint.")
	}

	existing, err := s.opts.Objects.DeletionRequestByIdempotencyKey(ctx, nil,
		input.Caller.Service, input.Caller.OrganizationID, input.IdempotencyKey)
	switch {
	case err == nil:
		if existing.RequestFingerprint != input.RequestFingerprint {
			return nil, fmt.Errorf("%w: idempotency key reused for a different request", ErrConflict)
		}
		return existing, nil
	case !errors.Is(err, repositories.ErrNotFound):
		return nil, err
	}

	object, err := s.opts.Objects.GetObjectForOwner(ctx, nil, input.Caller.Service,
		input.Caller.OrganizationID, input.MediaObjectID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	request := &models.DeletionRequest{
		ID:                 ids.New(ids.PrefixDeletion),
		MediaObjectID:      object.ID,
		OrganizationID:     object.OrganizationID,
		RequestedBy:        input.Caller.Service,
		Reason:             input.Reason,
		Status:             models.DeletionRequestPending,
		IdempotencyKey:     input.IdempotencyKey,
		RequestFingerprint: input.RequestFingerprint,
		CorrelationID:      input.Caller.CorrelationID,
		CausationID:        input.Caller.CausationID,
	}

	blocked, blockReason := object.PhysicalDeletionBlocked(s.opts.Now().UTC())
	if blocked {
		request.Status = models.DeletionRequestBlocked
		request.BlockReason = blockReason
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if createErr := s.opts.Objects.CreateDeletionRequest(ctx, tx, request); createErr != nil {
			if errors.Is(createErr, repositories.ErrConflict) {
				return fmt.Errorf("%w: deletion already requested", ErrConflict)
			}
			return createErr
		}
		action := models.ActionDeletionRequested
		if blocked {
			action = models.ActionDeletionBlocked
		} else {
			locked, lockErr := s.opts.Objects.LockObject(ctx, tx, object.ID)
			if lockErr != nil {
				return lockErr
			}
			requestedAt := s.opts.Now().UTC()
			locked.DeletionState = models.DeletionPending
			locked.DeletionRequestedAt = &requestedAt
			if updateErr := s.opts.Objects.UpdateObject(ctx, tx, locked, locked.Version); updateErr != nil {
				return updateErr
			}
			*object = *locked
		}
		if histErr := s.opts.Objects.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: object.OrganizationID,
			MediaObjectID:  object.ID,
			Action:         action,
			ToState:        object.DeletionState,
			ActorType:      "SERVICE",
			ActorID:        input.Caller.Service,
			Metadata:       map[string]any{"blockReason": blockReason, "reason": input.Reason},
			CorrelationID:  input.Caller.CorrelationID,
			CausationID:    input.Caller.CausationID,
		}); histErr != nil {
			return histErr
		}
		return s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: object.OrganizationID,
			ActorType:      "SERVICE",
			ActorID:        input.Caller.Service,
			Action:         "media.deletion.requested",
			ResourceType:   "media_object",
			ResourceID:     object.ID,
			NewValues:      map[string]any{"status": request.Status, "blockReason": blockReason},
			Reason:         input.Reason,
			RequestID:      input.Caller.RequestID,
			CorrelationID:  input.Caller.CorrelationID,
		})
	})
	if err != nil {
		return nil, err
	}

	if blocked {
		return request, nil
	}
	if err := s.executeDeletion(ctx, request.ID, object.ID); err != nil {
		return nil, err
	}
	return s.opts.Objects.LatestDeletionRequest(ctx, nil, object.ID)
}

// executeDeletion destroys the bytes and records the authoritative outcome.
func (s *Service) executeDeletion(ctx context.Context, requestID string, objectID string) error {
	object, err := s.opts.Objects.GetObject(ctx, nil, objectID)
	if err != nil {
		return err
	}

	storageErr := s.opts.Storage.Delete(ctx, object.StorageObjectKey)
	now := s.opts.Now().UTC()

	expectedVersion := object.Version
	caller := Caller{Service: "media-file-service", CorrelationID: object.CorrelationID}

	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		status := models.DeletionRequestCompleted
		eventType := events.EventDeletionCompleted
		action := models.ActionDeletionCompleted
		lastError := ""

		if storageErr != nil {
			// The storage call failed. Fleet must not be told the file is gone,
			// because it may not be: the state is DELETION_FAILED and a
			// read-back decides.
			status = models.DeletionRequestFailed
			eventType = events.EventDeletionFailed
			action = models.ActionDeletionFailed
			lastError = events.RedactError(storageErr)
			object.DeletionState = models.DeletionFailed
			object.DeletionFailureCode = "STORAGE_DELETE_FAILED"
		} else {
			object.DeletionState = models.DeletionDeleted
			object.DeletedAt = &now
			object.DeletionFailureCode = ""
		}

		if err := s.opts.Objects.UpdateObject(ctx, tx, object, expectedVersion); err != nil {
			return err
		}
		if err := s.opts.Objects.SetDeletionRequestStatus(ctx, tx, requestID, status, "", lastError); err != nil {
			return err
		}
		if err := s.opts.Objects.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: object.OrganizationID,
			MediaObjectID:  object.ID,
			Action:         action,
			ToState:        object.DeletionState,
			ActorType:      "SERVICE",
			ActorID:        "media-file-service",
			CorrelationID:  object.CorrelationID,
		}); err != nil {
			return err
		}
		// The audit row deliberately outlives the bytes: evidence that a file
		// existed and was destroyed is exactly what a later investigation needs.
		if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: object.OrganizationID,
			ActorType:      "SERVICE",
			ActorID:        "media-file-service",
			Action:         "media.deletion." + strings.ToLower(status),
			ResourceType:   "media_object",
			ResourceID:     object.ID,
			NewValues:      map[string]any{"deletionState": object.DeletionState},
			CorrelationID:  object.CorrelationID,
		}); err != nil {
			return err
		}
		if err := s.publish(ctx, tx, eventType, object, caller, nil); err != nil {
			return err
		}
		return s.publish(ctx, tx, events.EventAssetStatusChanged, object, caller, nil)
	})
}

// DeletionStatus reads back the authoritative deletion outcome. This is what
// resolves an UNKNOWN on the caller's side.
func (s *Service) DeletionStatus(ctx context.Context, caller Caller, objectID string) (*models.Object, *models.DeletionRequest, error) {
	object, err := s.GetObject(ctx, caller, objectID)
	if err != nil {
		return nil, nil, err
	}
	request, err := s.opts.Objects.LatestDeletionRequest(ctx, nil, objectID)
	if errors.Is(err, repositories.ErrNotFound) {
		return object, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return object, request, nil
}

// SetRetention updates retention and legal hold for one object.
func (s *Service) SetRetention(ctx context.Context, caller Caller, objectID string,
	retentionClass string, retentionUntil *time.Time, legalHold *bool) (*models.Object, error) {
	object, err := s.GetObject(ctx, caller, objectID)
	if err != nil {
		return nil, err
	}
	expectedVersion := object.Version
	if retentionClass != "" {
		object.RetentionClass = strings.ToUpper(retentionClass)
	}
	if retentionUntil != nil {
		until := retentionUntil.UTC()
		object.RetentionUntil = &until
	}
	if legalHold != nil {
		object.LegalHold = *legalHold
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if updateErr := s.opts.Objects.UpdateObject(ctx, tx, object, expectedVersion); updateErr != nil {
			return updateErr
		}
		return s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: object.OrganizationID,
			ActorType:      "SERVICE",
			ActorID:        caller.Service,
			Action:         "media.retention.updated",
			ResourceType:   "media_object",
			ResourceID:     object.ID,
			NewValues: map[string]any{
				"retentionClass": object.RetentionClass,
				"legalHold":      object.LegalHold,
			},
			RequestID:     caller.RequestID,
			CorrelationID: caller.CorrelationID,
		})
	})
	if err != nil {
		return nil, err
	}
	return object, nil
}

// RunMaintenancePass expires lapsed grants and stale uploads once.
func (s *Service) RunMaintenancePass(ctx context.Context, batch int) error {
	if _, err := s.opts.Objects.ExpireAuthorizations(ctx, s.opts.Pool, batch); err != nil {
		return err
	}
	if _, err := s.opts.Objects.ExpireStaleUploads(ctx, s.opts.Pool, batch); err != nil {
		return err
	}
	_, err := repositories.DeleteExpiredServiceNonces(ctx, s.opts.Pool, batch*10)
	return err
}

// RunMaintenance loops until ctx ends, holding a single-runner lease so two
// replicas do not both sweep.
func (s *Service) RunMaintenance(ctx context.Context, owner string, interval time.Duration,
	lease time.Duration, batch int) error {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		acquired, err := repositories.AcquireWorkerLease(ctx, s.opts.Pool, "media-maintenance", owner, lease)
		if err == nil && acquired {
			_ = s.RunMaintenancePass(ctx, batch)
		}
		select {
		case <-ctx.Done():
			_ = repositories.ReleaseWorkerLease(context.WithoutCancel(ctx), s.opts.Pool, "media-maintenance", owner)
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// publish appends one media event describing the object's current standing.
//
// The payload is assembled field by field and deliberately excludes the
// storage key, the bucket and anything resembling a URL or credential.
func (s *Service) publish(ctx context.Context, tx pgx.Tx, eventType string,
	object *models.Object, caller Caller, extra map[string]any) error {
	if s.opts.Events == nil {
		return nil
	}
	payload := map[string]any{
		"mediaObjectId":       object.ID,
		"subjectReference":    object.ID,
		"status":              object.UploadStatus,
		"organizationId":      object.OrganizationID,
		"ownerService":        object.OwnerService,
		"ownerType":           object.OwnerType,
		"ownerReference":      object.OwnerReference,
		"uploadPurpose":       object.UploadPurpose,
		"uploadStatus":        object.UploadStatus,
		"safetyStatus":        object.SafetyStatus,
		"deletionState":       object.DeletionState,
		"signatureValidation": object.SignatureValidation,
		"detectedMime":        object.DetectedMIME,
		"sizeBytes":           object.SizeBytes,
		"checksumSha256":      object.ChecksumSHA256,
		"capturedAt":          formatOptionalTime(object.CapturedAt),
		"sourceTimestamp":     formatOptionalTime(object.SourceTimestamp),
		"captureMethod":       object.CaptureMethod,
		"deviceReference":     object.DeviceReference,
		"actorReference":      object.ActorReference,
		"originalMimeType":    object.OriginalMIMEType,
		"originalSizeBytes":   object.OriginalSizeBytes,
		"integrityHash":       object.IntegrityHash,
		"sourceVersion":       object.Version,
	}
	if object.ValidationFailureCode != "" {
		payload["validationFailureCode"] = object.ValidationFailureCode
	}
	if object.SafetyReason != "" {
		payload["safetyReason"] = object.SafetyReason
	}
	for key, value := range extra {
		payload[key] = value
	}
	return s.opts.Events.Publish(ctx, tx, eventType, object.ID, object.Version,
		object.OrganizationID, caller.CorrelationID, caller.CausationID, payload)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// --- token minting -------------------------------------------------------

// mintUploadToken returns a bearer token and the hash stored beside the grant.
func (s *Service) mintUploadToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", fmt.Errorf("media: mint upload token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

func verifyUploadToken(token string, storedHash string) bool {
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(storedHash)) == 1
}

// signDownloadToken binds a grant to one object and one expiry.
//
// The token is "<expiry>.<mac>" and is verified, never looked up: there is no
// row to leak and nothing to clean up when it lapses.
func (s *Service) signDownloadToken(objectID string, callerService string, expiresAt time.Time) string {
	expiry := strconv.FormatInt(expiresAt.Unix(), 10)
	mac := hmac.New(sha256.New, s.opts.DownloadSecret)
	_, _ = mac.Write([]byte(objectID + "\x00" + callerService + "\x00" + expiry))
	return expiry + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyDownloadToken checks a grant against the object it names.
//
// The MAC covers the object id, the service the grant was minted for and the
// expiry, so a grant is unusable for a different object, by a different
// service, or after it lapses. Expiry is checked before the MAC so a lapsed
// token is rejected without any comparison at all.
func (s *Service) verifyDownloadToken(object *models.Object, token string) bool {
	if len(s.opts.DownloadSecret) == 0 || token == "" {
		return false
	}
	expiry, signature, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	seconds, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil {
		return false
	}
	if s.opts.Now().UTC().Unix() >= seconds {
		return false
	}
	expected := s.signDownloadToken(object.ID, object.OwnerService, time.Unix(seconds, 0).UTC())
	_, expectedSignature, _ := strings.Cut(expected, ".")
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

// storageKeyFor mints the server-chosen object key.
func storageKeyFor(organizationID string, purpose string, objectID string) string {
	return strings.Join([]string{"private", organizationID, strings.ToLower(purpose), objectID}, "/")
}

func uploadPathFor(objectID string) string {
	return "/objects/" + objectID + "/content"
}

func contentPathFor(objectID string) string {
	return "/objects/" + objectID + "/content"
}

// firstMIME returns the single content type an export grant may name. An
// export grant declares exactly one; anything else is a caller that has not
// understood the contract, and ExportPairing refuses it.
func firstMIME(values []string) string {
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

// validateBindings enforces the binding rules before anything is written.
//
// A binding is a pair or nothing, it names a known type, and a secondary
// binding repeats neither the primary nor itself. Duplicates are refused
// rather than silently collapsed: a caller who sent the same binding twice
// has a bug worth hearing about.
func validateBindings(input *AuthorizeUploadInput) error {
	if (input.BindingType == "") != (input.BindingID == "") {
		return invalid("BINDING_INCOMPLETE", "A binding needs both a type and an id.")
	}
	if input.BindingType != "" && !models.ValidBindingType(input.BindingType) {
		return invalid("BINDING_TYPE_UNSUPPORTED", "That binding type is not supported.")
	}
	// Sensitive evidence must say what it is evidence for. An unbound damage
	// photo proves nothing about any particular job and could be attached to
	// any of them later.
	if models.SensitiveEvidencePurpose(input.UploadPurpose) && input.BindingType == "" {
		return invalid("BINDING_REQUIRED", "That evidence purpose must be bound to what it is evidence for.")
	}
	seen := map[string]bool{input.BindingType + "\x00" + input.BindingID: input.BindingType != ""}
	normalized := make([]models.Binding, 0, len(input.SecondaryBindings))
	for _, binding := range input.SecondaryBindings {
		binding.Type = strings.ToUpper(strings.TrimSpace(binding.Type))
		binding.ID = strings.TrimSpace(binding.ID)
		if binding.Type == "" || binding.ID == "" {
			return invalid("BINDING_INCOMPLETE", "A secondary binding needs both a type and an id.")
		}
		if !models.ValidBindingType(binding.Type) {
			return invalid("BINDING_TYPE_UNSUPPORTED", "That secondary binding type is not supported.")
		}
		key := binding.Type + "\x00" + binding.ID
		if seen[key] {
			return invalid("BINDING_DUPLICATE", "A binding was declared more than once.")
		}
		seen[key] = true
		normalized = append(normalized, binding)
	}
	input.SecondaryBindings = normalized
	return nil
}

// RevokeInput withdraws an object as evidence.
type RevokeInput struct {
	Caller          Caller
	MediaObjectID   string
	ExpectedVersion int64
	Reason          string
}

// RevokeEvidence withdraws a file as evidence without deleting it.
//
// Only the owning service may revoke, because revoking is a statement about
// what the owner is prepared to rely on. The bytes are kept: what was
// withdrawn, and when, may itself be evidence later.
func (s *Service) RevokeEvidence(ctx context.Context, input RevokeInput) (*models.Object, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, invalid("REASON_REQUIRED", "A revocation reason is required.")
	}
	if input.ExpectedVersion <= 0 {
		return nil, invalid("VERSION_REQUIRED", "The expected version is required.")
	}
	current, err := s.GetObject(ctx, input.Caller, input.MediaObjectID)
	if err != nil {
		return nil, err
	}
	if current.EffectiveStatus() == models.StatusRevoked {
		// Already withdrawn. A retried revocation is not an error.
		return current, nil
	}
	if current.EffectiveStatus() == models.StatusDeleted {
		return nil, fmt.Errorf("%w: a deleted object cannot be revoked", ErrConflict)
	}
	revoked, err := s.opts.Objects.RevokeObject(ctx, nil, current.ID, input.ExpectedVersion, reason)
	if errors.Is(err, repositories.ErrVersionMismatch) {
		return nil, repositories.ErrVersionMismatch
	}
	if err != nil {
		return nil, err
	}
	if err = s.loadBindings(ctx, revoked); err != nil {
		return nil, err
	}
	return revoked, nil
}

// EvidenceExpectation is what a consumer is prepared to accept.
//
// Every field is optional except the object id; a field left empty is not
// checked. The consumer states its expectations and the answer says which of
// them held, so it can never be tricked into trusting a raw id it was handed.
type EvidenceExpectation struct {
	MediaObjectID string
	OwnerService  string
	OwnerType     string
	OwnerRef      string
	Purposes      []string
	BindingType   string
	BindingID     string
}

// EvidenceVerdict is the answer to "may I rely on this file?".
//
// It carries NO bytes, no storage key and no download grant. It is safe to
// return to a service that does not own the object precisely because it says
// only whether that service's own stated expectation holds.
type EvidenceVerdict struct {
	MediaObjectID  string   `json:"mediaObjectId"`
	Usable         bool     `json:"usable"`
	Status         string   `json:"status"`
	UploadComplete bool     `json:"uploadComplete"`
	SafetyStatus   string   `json:"safetyStatus"`
	Purpose        string   `json:"purpose"`
	ChecksumSHA256 string   `json:"checksumSha256"`
	Failures       []string `json:"failures"`
	Version        int64    `json:"version"`
}

// Controlled reasons an expectation failed. A consumer branches on these.
const (
	EvidenceNotActive        = "NOT_ACTIVE"
	EvidenceUploadIncomplete = "UPLOAD_INCOMPLETE"
	EvidenceUnsafe           = "NOT_SAFE"
	EvidenceWrongOwner       = "WRONG_OWNER"
	EvidenceWrongPurpose     = "WRONG_PURPOSE"
	EvidenceWrongBinding     = "WRONG_BINDING"
)

// ValidateEvidence answers whether a file may be relied on as stated.
//
// It deliberately reads WITHOUT owner scoping: the question is often asked by
// a service that did not upload the file -- Verification checking a photo a
// provider app uploaded. That is safe because the answer is a verdict against
// the caller's own expectation, never the file or where it lives. An unknown
// id and an id outside the caller's tenant both answer NOT_ACTIVE, so the
// endpoint cannot be used to discover which ids exist.
func (s *Service) ValidateEvidence(ctx context.Context, caller Caller,
	expectation EvidenceExpectation) (EvidenceVerdict, error) {
	id := strings.TrimSpace(expectation.MediaObjectID)
	if id == "" {
		return EvidenceVerdict{}, invalid("OBJECT_REQUIRED", "A media object id is required.")
	}
	// A question must come with context. The caller names either its own
	// tenant or the binding or owner it already knows the file by. Without
	// this, the route would answer "is this id live?" for any id at all --
	// an existence oracle, which is precisely what a verdict-only contract is
	// meant not to be.
	if caller.OrganizationID == "" && strings.TrimSpace(expectation.BindingID) == "" &&
		strings.TrimSpace(expectation.OwnerRef) == "" {
		return EvidenceVerdict{}, invalid("CONTEXT_REQUIRED",
			"Name the organization, binding or owner the file is expected to belong to.")
	}
	verdict := EvidenceVerdict{MediaObjectID: id, Failures: []string{}}

	object, err := s.opts.Objects.GetObject(ctx, nil, id)
	if errors.Is(err, repositories.ErrNotFound) ||
		(err == nil && caller.OrganizationID != "" && object.OrganizationID != caller.OrganizationID) {
		verdict.Status = models.StatusDeleted
		verdict.Failures = append(verdict.Failures, EvidenceNotActive)
		return verdict, nil
	}
	if err != nil {
		return EvidenceVerdict{}, err
	}
	if err = s.loadBindings(ctx, object); err != nil {
		return EvidenceVerdict{}, err
	}

	verdict.Status = object.EffectiveStatus()
	verdict.UploadComplete = object.UploadStatus == models.UploadCompleted
	verdict.SafetyStatus = object.SafetyStatus
	verdict.Purpose = object.UploadPurpose
	verdict.ChecksumSHA256 = object.ChecksumSHA256
	verdict.Version = object.Version

	if verdict.Status != models.StatusActive {
		verdict.Failures = append(verdict.Failures, EvidenceNotActive)
	}
	if !verdict.UploadComplete {
		verdict.Failures = append(verdict.Failures, EvidenceUploadIncomplete)
	}
	// Only a positive scan result counts. PENDING and UNKNOWN are not "not
	// unsafe"; they are unanswered, and evidence nobody has checked is not
	// evidence to rely on.
	if object.SafetyStatus != models.SafetySafe {
		verdict.Failures = append(verdict.Failures, EvidenceUnsafe)
	}
	if owner := strings.TrimSpace(expectation.OwnerService); owner != "" && owner != object.OwnerService {
		verdict.Failures = append(verdict.Failures, EvidenceWrongOwner)
	}
	if ownerType := strings.ToUpper(strings.TrimSpace(expectation.OwnerType)); ownerType != "" &&
		ownerType != object.OwnerType {
		verdict.Failures = append(verdict.Failures, EvidenceWrongOwner)
	}
	if ref := strings.TrimSpace(expectation.OwnerRef); ref != "" && ref != object.OwnerReference {
		verdict.Failures = append(verdict.Failures, EvidenceWrongOwner)
	}
	if len(expectation.Purposes) > 0 {
		matched := false
		for _, purpose := range expectation.Purposes {
			if strings.ToUpper(strings.TrimSpace(purpose)) == object.UploadPurpose {
				matched = true
			}
		}
		if !matched {
			verdict.Failures = append(verdict.Failures, EvidenceWrongPurpose)
		}
	}
	bindingType := strings.ToUpper(strings.TrimSpace(expectation.BindingType))
	bindingID := strings.TrimSpace(expectation.BindingID)
	if bindingType != "" || bindingID != "" {
		if !object.BoundTo(bindingType, bindingID) {
			verdict.Failures = append(verdict.Failures, EvidenceWrongBinding)
		}
	}
	verdict.Failures = uniqueStrings(verdict.Failures)
	verdict.Usable = len(verdict.Failures) == 0
	return verdict, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
