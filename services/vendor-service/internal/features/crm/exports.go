package crm

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8H — secure CRM exports.
//
// An export is a durable job that produces a private artifact, never a file
// served from this service and never a URL that outlives a check. The design
// rests on three refusals:
//
//   - No client string ever becomes SQL. A dataset is a code-defined
//     descriptor with a fixed field allowlist; a filter is a closed typed
//     struct. There is no column parameter, no sort parameter, no expression
//     and no raw query.
//   - No artifact becomes readable until it is complete. READY has a database
//     CHECK requiring the artifact, its checksum, its size, a completion
//     instant and an expiry, so a worker that died halfway cannot leave
//     something downloadable behind.
//   - No download is a stored credential. Access is decided live, per request,
//     against the job row and a fresh step-up, and the private store issues a
//     short-lived token that is never persisted.

// Export dataset keys. A closed registry: a client selects one of these, and
// nothing else reaches the database.
const (
	ExportCustomerSummary    = "CUSTOMER_SUMMARY"
	ExportSegmentMembers     = "SEGMENT_MEMBERS"
	ExportCorporateCustomers = "CORPORATE_CUSTOMERS"
	ExportLoyaltySummary     = "LOYALTY_SUMMARY"
	ExportCampaignSummary    = "CAMPAIGN_PERFORMANCE_SUMMARY"
	ExportIntelligenceScores = "INTELLIGENCE_SCORES"
)

// Export formats. Two, both line-oriented and streamable. XLSX, PDF and ZIP
// are deliberately absent: each would add a parser or an archive format to the
// attack surface of a file built from customer data, for no capability a
// consumer of CSV or JSONL lacks.
const (
	FormatCSV   = "CSV"
	FormatJSONL = "JSONL"
)

// Export statuses. No PENDING: a job is either waiting for a worker (QUEUED),
// held by one (RUNNING), or in one of four terminal states.
const (
	ExportQueued    = "QUEUED"
	ExportRunning   = "RUNNING"
	ExportReady     = "READY"
	ExportFailed    = "FAILED"
	ExportCancelled = "CANCELLED"
	ExportExpired   = "EXPIRED"
)

// Export error codes.
const (
	CodeExportNotFound            = "CRM_EXPORT_NOT_FOUND"
	CodeExportScopeDenied         = "CRM_EXPORT_SCOPE_DENIED"
	CodeExportInvalidType         = "CRM_EXPORT_INVALID_TYPE"
	CodeExportInvalidFormat       = "CRM_EXPORT_INVALID_FORMAT"
	CodeExportTooLarge            = "CRM_EXPORT_TOO_LARGE"
	CodeExportStepUpRequired      = "CRM_EXPORT_STEP_UP_REQUIRED"
	CodeExportNotReady            = "CRM_EXPORT_NOT_READY"
	CodeExportExpired             = "CRM_EXPORT_EXPIRED"
	CodeExportCancelled           = "CRM_EXPORT_CANCELLED"
	CodeExportStorageUnavailable  = "CRM_EXPORT_STORAGE_UNAVAILABLE"
	CodeExportIdempotencyConflict = "CRM_EXPORT_IDEMPOTENCY_CONFLICT"
	CodeExportDisabled            = "CRM_EXPORT_DISABLED"
	CodeExportInvalidFilter       = "CRM_EXPORT_INVALID_FILTER"
	CodeExportVersionConflict     = "CRM_EXPORT_VERSION_CONFLICT"
)

// StepUpPurposeExportCreate and StepUpPurposeExportDownload are the two
// elevation purposes Section 8H consumes. They are distinct so a grant taken
// to request an export cannot be spent releasing a different one's bytes.
const (
	StepUpPurposeExportCreate   = "crm.export.create"
	StepUpPurposeExportDownload = "crm.export.download"
)

// StepUpConsumer spends a short-lived elevation issued by Section 5.
//
// It is a port so the CRM feature never imports the auth feature's
// repositories, and so a deployment without step-up fails closed: a nil
// consumer disables the guarded operations rather than skipping the check.
type StepUpConsumer interface {
	ConsumeGrant(ctx context.Context, grantID, sessionID, staffID, vendorID, purpose string) error
}

// ExportConfig tunes the generator.
type ExportConfig struct {
	BatchSize    int
	LeaseTTL     time.Duration
	MaxAttempts  int
	Backoff      time.Duration
	DefaultLimit int
	MaxLimit     int
	// MaxBytes bounds the encoded artifact. The row limit alone cannot, because
	// row width varies by dataset; an export that would exceed it fails with
	// CRM_EXPORT_TOO_LARGE rather than being truncated, because a silently
	// short export is a wrong answer that looks like a right one.
	MaxBytes int64
	// ArtifactTTL is how long a completed artifact remains retrievable.
	ArtifactTTL time.Duration
	// DownloadTTL is the lifetime of one access grant. It is deliberately far
	// shorter than ArtifactTTL: the artifact may live a day, the permission to
	// read it lives minutes.
	DownloadTTL time.Duration
}

func (c ExportConfig) withDefaults() ExportConfig {
	if c.BatchSize <= 0 {
		c.BatchSize = 500
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 2 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.Backoff <= 0 {
		c.Backoff = 30 * time.Second
	}
	if c.DefaultLimit <= 0 {
		c.DefaultLimit = 100000
	}
	if c.MaxLimit <= 0 {
		c.MaxLimit = 250000
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = 20 * 1024 * 1024
	}
	if c.ArtifactTTL <= 0 {
		c.ArtifactTTL = 24 * time.Hour
	}
	if c.DownloadTTL <= 0 {
		c.DownloadTTL = 5 * time.Minute
	}
	return c
}

// ---------------------------------------------------------------------------
// Artifact store port
// ---------------------------------------------------------------------------

// StoredArtifact is what a private store returns once bytes are safely held.
type StoredArtifact struct {
	Ref      string
	Bytes    int64
	Checksum string
}

// ArtifactAccess is one short-lived, single-purpose grant to read an artifact.
//
// There is no URL field that could be persisted and reused: the token and the
// path travel together to the caller once, and the expiry is the store's, not
// this service's opinion of it.
type ArtifactAccess struct {
	Ref       string
	Path      string
	Token     string
	ExpiresAt time.Time
	MediaType string
	Bytes     int64
}

// ArtifactStore is the private storage contract Section 8H needs.
//
// It is a port, not a client, for one reason that matters: the alternative to
// a private store is not writing the file somewhere local and serving it. If
// no store is configured, or the store refuses the content, exports fail with
// CRM_EXPORT_STORAGE_UNAVAILABLE and no bytes exist anywhere. A CRM export is
// the widest customer-data disclosure this service produces; it does not get a
// fallback path.
type ArtifactStore interface {
	// Put stores bytes privately and returns their durable reference. The
	// idempotency key makes a retried generation reuse one artifact rather
	// than orphaning the first.
	Put(ctx context.Context, in PutArtifactInput) (StoredArtifact, error)
	// Access issues a short-lived read grant.
	Access(ctx context.Context, businessID, ref string) (ArtifactAccess, error)
	// Delete revokes an artifact. It is called on cancellation and expiry, and
	// must be safe to call for something already gone.
	Delete(ctx context.Context, businessID, ref, reason string) error
}

// PutArtifactInput carries one generated export to the store.
type PutArtifactInput struct {
	BusinessID     string
	ExportID       string
	Filename       string
	MediaType      string
	Content        []byte
	IdempotencyKey string
	// RetainUntil is when the store may drop the object without being asked.
	RetainUntil time.Time
}

// ---------------------------------------------------------------------------
// Export job
// ---------------------------------------------------------------------------

// Export is one export job.
type Export struct {
	ID              string
	BusinessID      string
	BranchID        *string
	Scope           string
	RequestedBy     string
	ExportType      string
	Format          string
	Status          string
	Filter          ExportFilter
	FieldSetVersion int
	RowLimit        int
	RowCount        int64
	ArtifactRef     *string
	ArtifactBytes   *int64
	Checksum        *string
	PrivacyFenceAt  time.Time

	RequestedAt time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	ExpiresAt   *time.Time
	CancelledAt *time.Time
	FailedAt    *time.Time
	ErrorCode   *string
	Attempts    int
	Version     int64
}

// Downloadable reports whether the job may release bytes right now.
func (e Export) Downloadable(now time.Time) bool {
	return e.Status == ExportReady && e.ArtifactRef != nil &&
		e.ExpiresAt != nil && e.ExpiresAt.After(now)
}

// ExportFilter is the closed, typed filter vocabulary.
//
// Every field is a reference or an enum from a controlled set. There is no
// free-text predicate, no column name, no operator and no expression: a filter
// narrows a dataset along dimensions the dataset itself declares, and cannot
// describe anything else.
type ExportFilter struct {
	SegmentRef  string   `json:"segment_ref,omitempty"`
	CampaignRef string   `json:"campaign_ref,omitempty"`
	ModelKey    string   `json:"model_key,omitempty"`
	Statuses    []string `json:"statuses,omitempty"`
	RiskBands   []string `json:"risk_bands,omitempty"`
	ScoreStates []string `json:"score_states,omitempty"`
	IncludeTags bool     `json:"include_tags,omitempty"`
}

func (f ExportFilter) document() ([]byte, error) { return json.Marshal(f) }

func errExportDisabled() error {
	return crmError(apperrors.Unavailable("CRM exports are not enabled.", nil), CodeExportDisabled)
}
func errExportNotFound() error {
	// Indistinguishable from "exists but is not yours", so an id is never a
	// probe for another business's exports.
	return crmError(apperrors.NotFound("Export not found.", nil), CodeExportNotFound)
}
func errExportNotReady() error {
	return crmError(apperrors.Conflict("That export is not ready.", nil), CodeExportNotReady)
}
func errExportExpired() error {
	return crmError(apperrors.Conflict("That export has expired.", nil), CodeExportExpired)
}
func errExportStepUpRequired() error {
	return crmError(apperrors.Forbidden("Additional verification is required.", nil), CodeExportStepUpRequired)
}
func errExportStorageUnavailable() error {
	return crmError(apperrors.Unavailable("Secure export storage is unavailable.", nil),
		CodeExportStorageUnavailable)
}
func errExportTooLarge() error {
	return crmError(apperrors.Validation("That export is too large.", []apperrors.FieldViolation{
		{Field: "row_limit", Message: "Narrow the export or lower the row limit."}}), CodeExportTooLarge)
}
func errExportVersionConflict() error {
	return crmError(apperrors.Conflict("This export changed since you loaded it. Reload and try again.", nil),
		CodeExportVersionConflict)
}

func (s *Service) assertExportsAvailable() error {
	if !s.flags.CRMEnabled || !s.flags.ExportsEnabled {
		return errExportDisabled()
	}
	return nil
}

// ValidExportFormat reports whether a format is one of the two supported.
func ValidExportFormat(format string) bool {
	return format == FormatCSV || format == FormatJSONL
}

func normalizeExportEnum(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }
