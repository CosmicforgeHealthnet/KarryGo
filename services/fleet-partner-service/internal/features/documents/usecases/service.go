// Package usecases implements the fleet-side document domain.
//
// The division of authority this package exists to hold:
//
//	media-file-service owns    fleet owns
//	-----------------------    ----------------------------------
//	the bytes                  which company or vehicle it is for
//	the storage location       the category and purpose
//	the detected type          the verification decision
//	the checksum               the expiry date
//	the safety verdict         replacement and revocation
//	whether it still exists    retention policy and legal hold
//
// Fleet never contradicts the left column. When it has not heard from media
// recently enough, or has heard something it cannot reconcile, the document is
// STALE or UNKNOWN and every gate refuses — because a stale "safe" is not a
// weaker yes, it is not a yes at all.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Corporate events emitted by this domain.
const (
	EventDocumentRegistered   = "organization.document.registered.v1"
	EventDocumentStateChanged = "organization.document.state_changed.v1"
	EventDocumentVerified     = "organization.document.verified.v1"
	EventDocumentRejected     = "organization.document.rejected.v1"
	EventDocumentReplaced     = "organization.document.replaced.v1"
	EventDocumentRevoked      = "organization.document.revoked.v1"
	EventDocumentExpired      = "organization.document.expired.v1"
	EventDocumentDeleted      = "organization.document.deleted.v1"
	EventDocumentReconciling  = "organization.document.reconciliation_required.v1"
	EventDocumentArchived     = "organization.document.archived.v1"
)

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string,
		aggregateID string, aggregateVersion int64, organizationID string,
		correlationID string, payload any) error
}

// DecisionInvalidator supersedes eligibility decisions whose evidence changed.
//
// A narrow port rather than the whole eligibility service: this domain needs
// exactly one capability from it, and depending on the rest would couple two
// features that have no other reason to know about each other.
type DecisionInvalidator interface {
	InvalidateSubjectDecisions(ctx context.Context, q database.Querier,
		organizationID string, subjectType string, subjectID string, reasonCode string,
		correlationID string) (int64, error)
}

// MembershipLookup resolves a provider identity to the live fleet membership
// it controls, and the tenant that membership belongs to.
//
// A narrow port rather than the whole providers service: the provider-facing
// document surface needs exactly one fact — which live membership, on which
// organization, corresponds to a (providerService, providerID) pair asserted
// by a driver service that has already authenticated that provider's own
// session. Depending on the whole providers package would couple two
// features that have no other reason to know about each other.
type MembershipLookup interface {
	FindLiveMembership(ctx context.Context, providerService string, providerID string) (
		organizationID string, membershipID string, err error)
}

// ErrMembershipNotFound is what a MembershipLookup returns when the asserted
// provider identity resolves to no live membership on any tenant.
var ErrMembershipNotFound = errors.New("documents: no live membership for that provider identity")

// Flags gate the write and worker paths. Every one defaults to false.
type Flags struct {
	// WritesEnabled gates document creation, replacement and revocation.
	WritesEnabled bool
	// DownloadEnabled gates private content grants.
	DownloadEnabled bool
	// EventIngestionEnabled gates the media event route.
	EventIngestionEnabled bool
	// KillSwitch stops every write and worker without a redeploy.
	KillSwitch bool
}

// Options configure the document service.
type Options struct {
	Pool      *pgxpool.Pool
	Documents *repositories.Repository
	// Media is the authoritative file service. A nil or unconfigured client
	// makes every path that needs an authoritative answer fail closed; it
	// never makes the check disappear.
	Media       platformclients.MediaAuthority
	Events      EventPublisher
	Audit       *audit.Recorder
	Eligibility DecisionInvalidator
	// Memberships resolves a provider identity for the provider-facing
	// document surface. A nil value makes that surface fail closed rather
	// than making the identity check disappear.
	Memberships MembershipLookup
	Metrics     *observability.Metrics
	Flags       Flags
	// StaleAfter is how long a media projection may go unconfirmed before it
	// is treated as STALE.
	StaleAfter time.Duration
	// MaxDownloadBytes bounds a proxied content read.
	MaxDownloadBytes int64
	// SelfVerificationAllowed permits the uploader to verify their own
	// document. Default false: the whole value of a verification step is that
	// somebody other than the submitter looked.
	SelfVerificationAllowed bool
	Now                     func() time.Time
}

// Service is the fleet-side document service.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	if opts.StaleAfter <= 0 {
		opts.StaleAfter = 15 * time.Minute
	}
	if opts.MaxDownloadBytes <= 0 {
		opts.MaxDownloadBytes = 32 << 20
	}
	return &Service{opts: opts}
}

// Actor is the authenticated caller.
type Actor struct {
	OrganizationID string
	ActorType      string
	ActorID        string
}

// writesAllowed refuses every state-changing path when the domain is switched
// off or the kill switch is engaged.
func (s *Service) writesAllowed() error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable(errorcodes.FleetServiceUnavailable,
			"Document changes are temporarily unavailable.")
	}
	if !s.opts.Flags.WritesEnabled {
		return errorcodes.Unavailable(errorcodes.FleetServiceUnavailable,
			"Document management is not enabled for this deployment.")
	}
	return nil
}

func (s *Service) mediaReady() error {
	if s.opts.Media == nil || !s.opts.Media.Configured() {
		// An unreachable authority is not evidence of anything. Refusing is the
		// only safe answer: proceeding would mean fleet inventing a file state.
		return errorcodes.Unavailable(errorcodes.MediaAuthorityUnavailable,
			"The document service is temporarily unavailable. Please try again.")
	}
	return nil
}

// RequestUploadInput asks media for an upload grant and records the fleet-side
// document that will point at it.
type RequestUploadInput struct {
	Actor            Actor
	OwnerType        string
	OwnerID          string
	BranchID         string
	Category         string
	DocumentType     string
	IssueDate        *time.Time
	ExpiryDate       *time.Time
	AllowedMIMETypes []string
	MaxSizeBytes     int64
	RetentionClass   string
	// ReplacesDocumentID makes this document the successor of an existing one.
	ReplacesDocumentID string
	// Visibility defaults to INTERNAL when empty.
	Visibility     string
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	RequestID      string
}

// UploadTicket is what the caller receives.
//
// UploadToken is a bearer credential returned once. It is not stored on the
// fleet side, does not appear in the document row, and is never logged.
type UploadTicket struct {
	Document     *models.Document
	UploadToken  string
	UploadPath   string
	ExpiresAt    time.Time
	MaxSizeBytes int64
	AllowedMIME  []string
	Replayed     bool
}

// RequestUpload orchestrates a new document.
//
// Order of operations, and why:
//  1. shape and tenant ownership are checked first, so a malformed or
//     cross-tenant request never reaches the network;
//  2. the idempotent replay is resolved before anything else, so a retry
//     cannot create a second document;
//  3. media is asked for the grant BEFORE the transaction, because an external
//     call must not hold a database lock;
//  4. the document, its history, the audit row and the outbox event commit
//     together.
//
// The media object id is minted by media and carried into the row. Fleet never
// invents one, because a document pointing at an object media does not know
// about would be unverifiable forever.
func (s *Service) RequestUpload(ctx context.Context, input RequestUploadInput) (*UploadTicket, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	if err := s.validateUploadInput(&input); err != nil {
		return nil, err
	}
	if err := s.mediaReady(); err != nil {
		return nil, err
	}

	fingerprint := uploadFingerprint(input)

	if replay, err := s.findReplay(ctx, input.Actor.OrganizationID, input.IdempotencyKey, fingerprint); err != nil {
		return nil, err
	} else if replay != nil {
		// The upload token is not stored, so a replay cannot return one. The
		// caller receives the document it already created and asks for a fresh
		// grant with a new key if it still needs to upload.
		return &UploadTicket{Document: replay, Replayed: true}, nil
	}

	if err := s.assertOwnership(ctx, input); err != nil {
		return nil, err
	}

	var replaces *models.Document
	if input.ReplacesDocumentID != "" {
		existing, err := s.opts.Documents.GetByID(ctx, nil, input.Actor.OrganizationID, input.ReplacesDocumentID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
		}
		if err != nil {
			return nil, err
		}
		if existing.LifecycleState == models.StateDeleted {
			return nil, errorcodes.Conflict(errorcodes.DocumentDeleted,
				"A deleted document cannot be replaced.")
		}
		if existing.ReplacementDocumentID != nil {
			return nil, errorcodes.Conflict(errorcodes.DocumentReplaced,
				"That document has already been replaced.")
		}
		replaces = existing
	}

	grant, err := s.opts.Media.AuthorizeUpload(ctx, platformclients.CallMeta{
		OrganizationID: input.Actor.OrganizationID,
		RequestID:      input.RequestID,
		CorrelationID:  input.CorrelationID,
		CausationID:    input.CausationID,
		IdempotencyKey: input.IdempotencyKey,
	}, platformclients.MediaUploadRequest{
		OwnerType:        input.OwnerType,
		OwnerReference:   input.OwnerID,
		DocumentCategory: input.Category,
		UploadPurpose:    uploadPurposeFor(input.OwnerType, input.Category),
		AllowedMIMETypes: input.AllowedMIMETypes,
		MaxSizeBytes:     input.MaxSizeBytes,
		RetentionClass:   input.RetentionClass,
	})
	if err != nil {
		return nil, s.mediaError(err)
	}

	document := &models.Document{
		OrganizationID:      input.Actor.OrganizationID,
		OwnerType:           input.OwnerType,
		OwnerID:             input.OwnerID,
		MediaObjectID:       grant.MediaObjectID,
		Category:            input.Category,
		DocumentType:        input.DocumentType,
		IssueDate:           input.IssueDate,
		ExpiryDate:          input.ExpiryDate,
		LifecycleState:      models.StateUploadPending,
		VerificationState:   models.VerificationPending,
		SafetyProjection:    models.SafetyPending,
		MediaUploadStatus:   models.MediaUploadAuthorized,
		MediaDeletionState:  models.MediaDeletionActive,
		ProjectionFreshness: models.FreshnessCurrent,
		RetentionClass:      input.RetentionClass,
		Visibility:          input.Visibility,
		CreatedByActorType:  input.Actor.ActorType,
		CreatedByActorID:    input.Actor.ActorID,
		CorrelationID:       input.CorrelationID,
		CausationID:         input.CausationID,
		IdempotencyKey:      input.IdempotencyKey,
		RequestFingerprint:  fingerprint,
	}
	if input.BranchID != "" {
		branch := input.BranchID
		document.BranchID = &branch
	}
	if replaces != nil {
		predecessor := replaces.ID
		document.ReplacesDocumentID = &predecessor
	}
	syncedAt := s.opts.Now().UTC()
	document.SyncedAt = &syncedAt

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if createErr := s.opts.Documents.Create(ctx, tx, document); createErr != nil {
			if errors.Is(createErr, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.DocumentConflict,
					"A document already exists for this request.")
			}
			return createErr
		}
		if histErr := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			Action:         models.ActionCreated,
			ToState:        document.LifecycleState,
			ActorType:      input.Actor.ActorType,
			ActorID:        input.Actor.ActorID,
			Metadata: map[string]any{
				"category":     document.Category,
				"documentType": document.DocumentType,
				"ownerType":    document.OwnerType,
			},
			CorrelationID: input.CorrelationID,
			CausationID:   input.CausationID,
		}); histErr != nil {
			return histErr
		}
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: document.OrganizationID,
			ActorType:      strings.ToLower(input.Actor.ActorType),
			ActorID:        input.Actor.ActorID,
			Action:         "document.registered",
			ResourceType:   "document",
			ResourceID:     document.ID,
			NewValues: map[string]any{
				"category":      document.Category,
				"documentType":  document.DocumentType,
				"ownerType":     document.OwnerType,
				"mediaObjectId": document.MediaObjectID,
			},
			RequestID:     input.RequestID,
			CorrelationID: input.CorrelationID,
		}); auditErr != nil {
			return auditErr
		}
		return s.publish(ctx, tx, EventDocumentRegistered, document, input.CorrelationID, nil)
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_documents_registered_total",
		map[string]string{"category": document.Category, "ownerType": document.OwnerType}, 1)

	expiresAt, _ := time.Parse(time.RFC3339, grant.ExpiresAt)
	return &UploadTicket{
		Document:     document,
		UploadToken:  grant.UploadToken,
		UploadPath:   grant.UploadPath,
		ExpiresAt:    expiresAt,
		MaxSizeBytes: grant.MaxSizeBytes,
		AllowedMIME:  grant.AllowedMIMETypes,
	}, nil
}

func (s *Service) validateUploadInput(input *RequestUploadInput) error {
	input.OwnerType = strings.ToUpper(strings.TrimSpace(input.OwnerType))
	input.Category = strings.ToUpper(strings.TrimSpace(input.Category))
	input.DocumentType = strings.ToUpper(strings.TrimSpace(input.DocumentType))
	input.RetentionClass = strings.ToUpper(strings.TrimSpace(input.RetentionClass))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Visibility = strings.ToUpper(strings.TrimSpace(input.Visibility))
	if input.RetentionClass == "" {
		input.RetentionClass = models.RetentionStandard
	}
	if input.Visibility == "" {
		input.Visibility = models.VisibilityInternal
	}

	switch {
	case input.Actor.OrganizationID == "":
		return errorcodes.Forbidden(errorcodes.ActorNotAuthorized, "Organization scope is required.")
	case !models.ValidOwnerType(input.OwnerType):
		return errorcodes.BadRequest(errorcodes.DocumentInvalid, "ownerType is not supported.")
	case strings.TrimSpace(input.OwnerID) == "":
		return errorcodes.BadRequest(errorcodes.DocumentInvalid, "An owner id is required.")
	case !models.ValidCategory(input.Category):
		return errorcodes.BadRequest(errorcodes.DocumentInvalid, "category is not supported.")
	case !models.ValidVisibility(input.Visibility):
		return errorcodes.BadRequest(errorcodes.DocumentInvalid, "visibility is not supported.")
	case input.DocumentType == "":
		return errorcodes.BadRequest(errorcodes.DocumentInvalid, "A document type is required.")
	case len(input.DocumentType) > 100:
		return errorcodes.BadRequest(errorcodes.DocumentInvalid, "The document type is too long.")
	case input.IdempotencyKey == "":
		return errorcodes.BadRequest(errorcodes.ValidationFailed, "An idempotency key is required.")
	case len(input.AllowedMIMETypes) == 0:
		return errorcodes.BadRequest(errorcodes.DocumentInvalid,
			"At least one permitted content type is required.")
	case input.OwnerType == models.OwnerBranch && strings.TrimSpace(input.BranchID) == "":
		return errorcodes.BadRequest(errorcodes.DocumentInvalid,
			"A branch document must name its branch.")
	}

	if input.ExpiryDate != nil && input.IssueDate != nil && !input.ExpiryDate.After(*input.IssueDate) {
		return errorcodes.BadRequest(errorcodes.DocumentInvalid,
			"The expiry date must be after the issue date.")
	}
	return nil
}

// assertOwnership proves the organization owns everything the document points
// at. This is the tenant boundary for writes and the IDOR defence.
func (s *Service) assertOwnership(ctx context.Context, input RequestUploadInput) error {
	owned, err := s.opts.Documents.OwnerBelongsToOrganization(ctx, nil,
		input.Actor.OrganizationID, input.OwnerType, input.OwnerID)
	if err != nil {
		return err
	}
	if !owned {
		// Never disclose that the owner exists in another tenant.
		return errorcodes.NotFound(errorcodes.DocumentNotFound, "Owner not found.")
	}
	if input.BranchID == "" {
		return nil
	}
	branchOwned, err := s.opts.Documents.BranchBelongsToOrganization(ctx, nil,
		input.Actor.OrganizationID, input.BranchID)
	if err != nil {
		return err
	}
	if !branchOwned {
		return errorcodes.NotFound(errorcodes.DocumentNotFound, "Branch not found.")
	}
	return nil
}

// findReplay resolves a retried write.
func (s *Service) findReplay(ctx context.Context, organizationID string, key string,
	fingerprint string) (*models.Document, error) {
	existing, err := s.opts.Documents.FindByIdempotencyKey(ctx, nil, organizationID, key)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if existing.RequestFingerprint != fingerprint {
		return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
			"This idempotency key was already used for a different request.")
	}
	return existing, nil
}

// Get returns one document inside the tenant scope.
func (s *Service) Get(ctx context.Context, organizationID string, documentID string) (*models.Document, error) {
	document, err := s.opts.Documents.GetByID(ctx, nil, organizationID, documentID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}
	return document, err
}

// List returns the organization's documents inside the caller's branch scope.
func (s *Service) List(ctx context.Context, query tenant.Query,
	filter repositories.ListFilter) ([]models.Document, error) {
	if query.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized, "Organization scope is required.")
	}
	return s.opts.Documents.List(ctx, query, filter)
}

// History returns the append-only history of one document.
func (s *Service) History(ctx context.Context, organizationID string,
	documentID string) ([]models.HistoryEntry, error) {
	if _, err := s.Get(ctx, organizationID, documentID); err != nil {
		return nil, err
	}
	return s.opts.Documents.ListHistory(ctx, nil, organizationID, documentID)
}

// Verifications returns the immutable verification decisions.
func (s *Service) Verifications(ctx context.Context, organizationID string,
	documentID string) ([]models.VerificationDecision, error) {
	if _, err := s.Get(ctx, organizationID, documentID); err != nil {
		return nil, err
	}
	return s.opts.Documents.ListVerifications(ctx, nil, organizationID, documentID)
}

// Cases returns the reconciliation cases for one organization.
func (s *Service) Cases(ctx context.Context, organizationID string,
	documentID string) ([]models.ReconciliationCase, error) {
	return s.opts.Documents.ListCases(ctx, nil, organizationID, documentID)
}

// DownloadGrant is what a caller receives for private content.
//
// Token is short-lived and object-specific. It is returned once, never stored
// and never logged.
type DownloadGrant struct {
	DocumentID    string
	MediaObjectID string
	Token         string
	Path          string
	ExpiresAt     time.Time
	ContentType   string
	SizeBytes     int64
}

// AuthorizeDownload issues a short-lived grant for one document's content.
//
// Fleet's rules are applied first: the tenant, the owner relationship, the
// branch scope, the RBAC permission (checked by the HTTP layer), the lifecycle
// state, the safety projection, the deletion and revocation state and the
// projection freshness. Media applies its own floor on top and may still
// refuse.
func (s *Service) AuthorizeDownload(ctx context.Context, actor Actor, documentID string,
	correlationID string, causationID string, requestID string) (*DownloadGrant, error) {
	if s.opts.Flags.KillSwitch || !s.opts.Flags.DownloadEnabled {
		return nil, errorcodes.Unavailable(errorcodes.FleetServiceUnavailable,
			"Document download is not enabled for this deployment.")
	}
	document, err := s.Get(ctx, actor.OrganizationID, documentID)
	if err != nil {
		return nil, err
	}
	if err := s.mediaReady(); err != nil {
		return nil, err
	}

	if !document.Downloadable() {
		return nil, s.downloadRefusal(document)
	}
	// An unresolved reconciliation case means fleet's copy is known to
	// disagree with the authority. Serving content on that basis would be
	// serving a file nobody can currently vouch for.
	reconciling, err := s.opts.Documents.OpenCaseFor(ctx, nil, document.ID)
	if err != nil {
		return nil, err
	}
	if reconciling {
		return nil, errorcodes.Forbidden(errorcodes.DocumentNotDownloadable,
			"This document is being reconciled and cannot be released right now.")
	}
	if document.LegalHold && document.LifecycleState == models.StateDeleted {
		return nil, errorcodes.Forbidden(errorcodes.DocumentDeleted, "This document has been deleted.")
	}

	grant, err := s.opts.Media.AuthorizeDownload(ctx, platformclients.CallMeta{
		OrganizationID: actor.OrganizationID,
		RequestID:      requestID,
		CorrelationID:  correlationID,
		CausationID:    causationID,
	}, document.MediaObjectID)
	if err != nil {
		if errors.Is(err, platformclients.ErrMediaNotDownloadable) {
			return nil, errorcodes.Forbidden(errorcodes.DocumentNotDownloadable,
				"This document cannot be released right now.")
		}
		if errors.Is(err, platformclients.ErrMediaObjectNotFound) {
			return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
		}
		return nil, s.mediaError(err)
	}

	// The audit record names the authorization without recording the grant.
	// That is the difference between evidence that access was authorized and a
	// stored credential anybody with database access could replay.
	if auditErr := s.opts.Audit.Record(ctx, s.opts.Pool, audit.Entry{
		OrganizationID: document.OrganizationID,
		ActorType:      strings.ToLower(actor.ActorType),
		ActorID:        actor.ActorID,
		Action:         "document.download.authorized",
		ResourceType:   "document",
		ResourceID:     document.ID,
		NewValues: map[string]any{
			"mediaObjectId":    document.MediaObjectID,
			"expiresAt":        grant.ExpiresAt,
			"safetyProjection": document.SafetyProjection,
		},
		RequestID:     requestID,
		CorrelationID: correlationID,
	}); auditErr != nil {
		return nil, auditErr
	}
	if histErr := s.opts.Documents.AppendHistory(ctx, s.opts.Pool, &models.HistoryEntry{
		OrganizationID: document.OrganizationID,
		DocumentID:     document.ID,
		Action:         models.ActionDownloadAuthorized,
		ToState:        document.LifecycleState,
		ActorType:      actor.ActorType,
		ActorID:        actor.ActorID,
		CorrelationID:  correlationID,
		CausationID:    causationID,
	}); histErr != nil {
		return nil, histErr
	}

	expiresAt, _ := time.Parse(time.RFC3339, grant.ExpiresAt)
	s.opts.Metrics.IncCounter("fleet_partner_document_downloads_total",
		map[string]string{"category": document.Category}, 1)

	return &DownloadGrant{
		DocumentID:    document.ID,
		MediaObjectID: document.MediaObjectID,
		Token:         grant.Token,
		Path:          grant.Path,
		ExpiresAt:     expiresAt,
		ContentType:   grant.ContentType,
		SizeBytes:     grant.SizeBytes,
	}, nil
}

// downloadRefusal names the specific rule that refused, so a caller can tell
// "this file is unsafe" from "we cannot currently vouch for it".
func (s *Service) downloadRefusal(document *models.Document) error {
	switch {
	case document.LifecycleState == models.StateDeleted || document.MediaDeletionState == models.MediaDeletionDeleted:
		return errorcodes.Forbidden(errorcodes.DocumentDeleted, "This document has been deleted.")
	case document.LifecycleState == models.StateRevoked:
		return errorcodes.Forbidden(errorcodes.DocumentRevoked, "This document has been revoked.")
	case document.SafetyProjection == models.SafetyUnsafe ||
		document.SafetyProjection == models.SafetyQuarantined:
		return errorcodes.Forbidden(errorcodes.DocumentNotDownloadable,
			"This document cannot be released.")
	case !document.ProjectionCurrent():
		return errorcodes.Forbidden(errorcodes.DocumentProjectionStale,
			"This document cannot be released right now. Please try again shortly.")
	default:
		return errorcodes.Forbidden(errorcodes.DocumentNotDownloadable,
			"This document cannot be released right now.")
	}
}

// ReadContent streams document bytes against a grant fleet just issued.
//
// Fleet does not store the bytes: they pass through and are gone. This exists
// so a browser client never needs the media service's address or a credential
// for it.
func (s *Service) ReadContent(ctx context.Context, actor Actor, grant *DownloadGrant,
	correlationID string) ([]byte, error) {
	if err := s.mediaReady(); err != nil {
		return nil, err
	}
	content, err := s.opts.Media.ReadContent(ctx, platformclients.CallMeta{
		OrganizationID: actor.OrganizationID,
		CorrelationID:  correlationID,
	}, platformclients.MediaDownloadGrant{
		MediaObjectID: grant.MediaObjectID,
		Token:         grant.Token,
	}, s.opts.MaxDownloadBytes)
	if err != nil {
		return nil, s.mediaError(err)
	}
	return content, nil
}

// publish appends one corporate document event.
//
// The payload is assembled field by field and carries no URL, no token, no
// storage reference and no file content — only the document's standing.
func (s *Service) publish(ctx context.Context, tx pgx.Tx, eventType string,
	document *models.Document, correlationID string, extra map[string]any) error {
	if s.opts.Events == nil {
		return nil
	}
	payload := map[string]any{
		"documentId":          document.ID,
		"organizationId":      document.OrganizationID,
		"ownerType":           document.OwnerType,
		"ownerId":             document.OwnerID,
		"category":            document.Category,
		"documentType":        document.DocumentType,
		"lifecycleState":      document.LifecycleState,
		"verificationState":   document.VerificationState,
		"safetyProjection":    document.SafetyProjection,
		"projectionFreshness": document.ProjectionFreshness,
		"mediaSourceVersion":  document.MediaSourceVersion,
	}
	if document.ExpiryDate != nil {
		payload["expiryDate"] = document.ExpiryDate.UTC().Format("2006-01-02")
	}
	for key, value := range extra {
		payload[key] = value
	}
	return s.opts.Events.Publish(ctx, tx, eventType, "document", document.ID,
		document.Version, document.OrganizationID, correlationID, payload)
}

// invalidateDecisions supersedes eligibility decisions that rested on this
// document, so the next gate re-evaluates rather than reusing a stale yes.
func (s *Service) invalidateDecisions(ctx context.Context, q database.Querier,
	document *models.Document, reasonCode string, correlationID string) error {
	if s.opts.Eligibility == nil {
		return nil
	}
	subjectType, subjectID := eligibilitySubjectFor(document)
	if subjectType == "" {
		return nil
	}
	count, err := s.opts.Eligibility.InvalidateSubjectDecisions(ctx, q,
		document.OrganizationID, subjectType, subjectID, reasonCode, correlationID)
	if err != nil {
		return err
	}
	if count > 0 {
		s.opts.Metrics.IncCounter("fleet_partner_document_decision_invalidations_total",
			map[string]string{"reason": reasonCode}, float64(count))
	}
	return nil
}

// eligibilitySubjectFor maps a document owner onto the eligibility subject
// whose decisions it could have supported.
func eligibilitySubjectFor(document *models.Document) (string, string) {
	switch document.OwnerType {
	case models.OwnerCompany, models.OwnerBranch:
		return "COMPANY", document.OrganizationID
	case models.OwnerProviderLink, models.OwnerVehicleLink:
		// Both are anchored to the provider membership, which is the subject
		// identity fleet's own decisions use.
		return "PROVIDER", document.OwnerID
	default:
		return "", ""
	}
}

// mediaError normalizes a downstream failure into a safe public error.
//
// Downstream detail is never echoed. An UNKNOWN outcome is reported as
// unavailable rather than as a failure, because the caller must retry or read
// back rather than conclude anything.
func (s *Service) mediaError(err error) error {
	var clientErr *platformclients.Error
	if errors.As(err, &clientErr) {
		switch clientErr.Outcome {
		case platformclients.OutcomeValidation:
			return errorcodes.BadRequest(errorcodes.DocumentInvalid,
				"The document could not be accepted.")
		case platformclients.OutcomeNotFound:
			return errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
		case platformclients.OutcomeForbidden:
			return errorcodes.Forbidden(errorcodes.DocumentNotDownloadable,
				"This document cannot be released right now.")
		case platformclients.OutcomeConflict:
			return errorcodes.Conflict(errorcodes.DocumentConflict,
				"That document request conflicts with an existing one.")
		}
	}
	s.opts.Metrics.IncCounter("fleet_partner_media_authority_failures_total", nil, 1)
	return errorcodes.Unavailable(errorcodes.MediaAuthorityUnavailable,
		"The document service is temporarily unavailable. Please try again.")
}

// uploadPurposeFor maps a fleet owner and category onto media's purpose
// vocabulary. One function rather than scattered branches: the mapping is a
// contract, and a contract implemented in six places drifts.
func uploadPurposeFor(ownerType string, category string) string {
	switch ownerType {
	case models.OwnerBranch:
		return "BRANCH_DOCUMENT"
	case models.OwnerProviderLink:
		return "PROVIDER_DOCUMENT"
	case models.OwnerVehicleLink:
		return "VEHICLE_DOCUMENT"
	case models.OwnerIncident:
		return "INCIDENT_ATTACHMENT"
	case models.OwnerDispute:
		return "DISPUTE_ATTACHMENT"
	case models.OwnerApproval:
		return "APPROVAL_ATTACHMENT"
	}
	switch category {
	case models.CategoryInsurance:
		return "INSURANCE_DOCUMENT"
	case models.CategoryCompliance:
		return "COMPLIANCE_DOCUMENT"
	case models.CategoryProof:
		return "PROOF_DOCUMENT"
	default:
		return "COMPANY_DOCUMENT"
	}
}

// uploadFingerprint hashes the meaningful inputs of an upload request.
func uploadFingerprint(input RequestUploadInput) string {
	hasher := sha256.New()
	parts := []string{
		input.Actor.OrganizationID, input.OwnerType, input.OwnerID, input.BranchID,
		input.Category, input.DocumentType, input.RetentionClass, input.ReplacesDocumentID,
		input.Visibility, strings.Join(input.AllowedMIMETypes, "|"), fmt.Sprintf("%d", input.MaxSizeBytes),
	}
	if input.IssueDate != nil {
		parts = append(parts, input.IssueDate.UTC().Format("2006-01-02"))
	}
	if input.ExpiryDate != nil {
		parts = append(parts, input.ExpiryDate.UTC().Format("2006-01-02"))
	}
	for _, part := range parts {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
