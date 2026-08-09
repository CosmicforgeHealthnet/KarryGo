package authority

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// The provider half of the authority contract.
//
// A fleet organization holds the RELATIONSHIP with a provider: it invited
// them, it records their branch, their employment type and whether they
// accepted. It does not hold the provider's platform standing — whether the
// account is active, whether identity verification passed, what service they
// are approved for, whether they are online, whether they are already on a
// job. Those are this service's facts, and an organization that could decide
// them for itself could sponsor an unverified rider onto a customer's job.
//
// As with vehicles, the answer carries no name, no phone number, no email, no
// photo and no rating. It carries verdicts.

// Provider account states in the contract.
const (
	AccountActive     = "ACTIVE"
	AccountInactive   = "INACTIVE"
	AccountIncomplete = "ONBOARDING_INCOMPLETE"
)

// ProviderAuthorityView is the authoritative standing of one provider.
type ProviderAuthorityView struct {
	ProviderID string `json:"providerId"`

	AccountState      string `json:"accountState"`
	VerificationState string `json:"verificationState"`

	// ServiceType is the provider's approved operation type as this service
	// records it. Empty means the provider has none recorded, which is not the
	// same as matching everything.
	ServiceType      string `json:"serviceType"`
	ServiceTypeMatch *bool  `json:"serviceTypeMatch,omitempty"`

	Available         bool   `json:"available"`
	AvailabilityState string `json:"availabilityState"`
	// ShiftOpen reports whether an availability session is currently open.
	// Distinct from Available: a provider can be inside a shift and still be
	// busy on a job.
	ShiftOpen bool `json:"shiftOpen"`

	Restricted       bool   `json:"restricted"`
	RestrictionState string `json:"restrictionState"`

	RequiredDocuments  []DocumentStatus `json:"requiredDocuments"`
	DocumentsSatisfied bool             `json:"documentsSatisfied"`

	ActiveAssignmentConflict bool   `json:"activeAssignmentConflict"`
	AssignmentConflictBasis  string `json:"assignmentConflictBasis"`

	SourceVersion int64     `json:"sourceVersion"`
	ObservedAt    time.Time `json:"observedAt"`
}

// ErrProviderNotFound means no such provider exists here.
var ErrProviderNotFound = errors.New("authority: provider not found")

// ProviderQuery is one provider authority lookup.
type ProviderQuery struct {
	ProviderID          string
	ExpectedServiceType string
}

// providerRow is the raw slice of authoritative provider state.
type providerRow struct {
	id                  string
	operationType       *string
	verificationStatus  string
	isActive            bool
	onboardingComplete  bool
	updatedAt           time.Time
	availabilityStatus  *string
	availabilityChanged *time.Time
	verifiedToGoOnline  *bool
	sessionStart        *time.Time
	liveTrip            bool
}

// LookupProvider answers one provider authority question.
func (r *AuthorityReader) LookupProvider(ctx context.Context,
	query ProviderQuery) (*ProviderAuthorityView, error) {
	providerID := strings.TrimSpace(query.ProviderID)
	if providerID == "" {
		return nil, ErrProviderNotFound
	}

	transaction, err := r.db.BeginTx(ctx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	var row providerRow
	err = transaction.QueryRow(ctx, `
		SELECT p.id::text, p.operation_type, p.verification_status, p.is_active,
		       p.onboarding_complete, p.updated_at,
		       a.status, a.last_changed_at, a.verified_to_go_online, a.session_start,
		       EXISTS (
		           SELECT 1 FROM trips t
		           WHERE t.provider_id = p.id
		             AND t.status NOT IN ('completed', 'cancelled', 'failed')
		       )
		FROM providers p
		LEFT JOIN provider_availability a ON a.provider_id = p.id
		WHERE p.id = $1::uuid`, providerID).
		Scan(&row.id, &row.operationType, &row.verificationStatus, &row.isActive,
			&row.onboardingComplete, &row.updatedAt, &row.availabilityStatus,
			&row.availabilityChanged, &row.verifiedToGoOnline, &row.sessionStart,
			&row.liveTrip)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProviderNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "invalid input syntax") {
			return nil, ErrProviderNotFound
		}
		return nil, err
	}

	documents, err := r.readProviderDocuments(ctx, transaction, providerID)
	if err != nil {
		return nil, err
	}

	return buildProviderView(row, documents, query), nil
}

// providerRequiredDocuments are the verification steps a provider must have
// passed before organization-sponsored work. Identity proves who they are;
// licence proves they may legally drive.
var providerRequiredDocuments = []string{"identity", "licence"}

// readProviderDocuments reports the standing of each required verification
// step.
//
// The verification STEP is read rather than the uploaded file, because a file
// that exists is not a check that passed. Only an approved step counts as
// present, and the licence step's own expiry date is what decides expiry — a
// licence that lapsed last month is not satisfied by the document still being
// on file.
func (r *AuthorityReader) readProviderDocuments(ctx context.Context, q pgx.Tx,
	providerID string) (map[string]DocumentStatus, error) {
	rows, err := q.Query(ctx, `
		SELECT step, status, licence_expiry_date
		FROM verification_steps
		WHERE provider_id = $1::uuid AND step = ANY($2::text[])`,
		providerID, providerRequiredDocuments)
	if err != nil {
		// A deployment that cannot read the verification tables cannot answer
		// the document question. Reporting "no documents" would read as a
		// clean negative; the caller must see the check as unanswerable.
		return nil, err
	}
	defer rows.Close()

	held := map[string]DocumentStatus{}
	for rows.Next() {
		var step, status string
		var expiry *time.Time
		if scanErr := rows.Scan(&step, &status, &expiry); scanErr != nil {
			return nil, scanErr
		}
		documentStatus := DocumentStatus{Type: step, Present: status == "approved"}
		if expiry != nil {
			expiresAt := expiry.UTC()
			documentStatus.ExpiresAt = &expiresAt
			documentStatus.Expired = !expiresAt.After(time.Now().UTC())
		}
		held[step] = documentStatus
	}
	return held, rows.Err()
}

// buildProviderView turns raw state into the contract answer.
func buildProviderView(row providerRow, documents map[string]DocumentStatus,
	query ProviderQuery) *ProviderAuthorityView {
	view := &ProviderAuthorityView{
		ProviderID:              row.id,
		AssignmentConflictBasis: ConflictBasisProviderLiveTrip,
		ObservedAt:              time.Now().UTC(),
	}

	switch {
	case !row.isActive:
		view.AccountState = AccountInactive
	case !row.onboardingComplete:
		view.AccountState = AccountIncomplete
	default:
		view.AccountState = AccountActive
	}

	switch row.verificationStatus {
	case "verified", "fully_approved":
		view.VerificationState = VerificationStateVerified
	case "pending_review", "pending", "under_review":
		view.VerificationState = VerificationStatePending
	case "rejected":
		view.VerificationState = VerificationStateRejected
	case "suspended":
		view.VerificationState = VerificationStateSuspended
	default:
		view.VerificationState = VerificationStateUnverified
	}

	if row.operationType != nil {
		view.ServiceType = *row.operationType
	}
	if expected := strings.TrimSpace(query.ExpectedServiceType); expected != "" {
		// An empty recorded service type never matches. "We do not know what
		// this provider is approved for" is not evidence that they are
		// approved for what you asked.
		match := view.ServiceType != "" && strings.EqualFold(expected, view.ServiceType)
		view.ServiceTypeMatch = &match
	}

	switch {
	case !row.isActive:
		view.Restricted = true
		view.RestrictionState = RestrictionInactive
	case view.VerificationState == VerificationStateSuspended:
		view.Restricted = true
		view.RestrictionState = RestrictionSuspended
	default:
		view.RestrictionState = RestrictionNone
	}

	view.AvailabilityState = AvailabilityUnknown
	if row.availabilityStatus != nil {
		switch strings.ToLower(*row.availabilityStatus) {
		case "online":
			view.AvailabilityState = AvailabilityOnline
		case "busy":
			view.AvailabilityState = AvailabilityBusy
		case "offline":
			view.AvailabilityState = AvailabilityOffline
		}
	}
	// Going online requires the platform's own gate to have passed; a row that
	// says online but was never cleared to go online is a contradiction, and
	// the safe reading of a contradiction is "not available".
	clearedToWork := row.verifiedToGoOnline != nil && *row.verifiedToGoOnline
	view.Available = view.AvailabilityState == AvailabilityOnline &&
		clearedToWork && !view.Restricted
	view.ShiftOpen = row.sessionStart != nil

	view.RequiredDocuments = make([]DocumentStatus, 0, len(providerRequiredDocuments))
	view.DocumentsSatisfied = true
	for _, documentType := range providerRequiredDocuments {
		status, ok := documents[documentType]
		if !ok {
			status = DocumentStatus{Type: documentType}
		}
		if !status.Present || status.Expired {
			view.DocumentsSatisfied = false
		}
		view.RequiredDocuments = append(view.RequiredDocuments, status)
	}

	view.ActiveAssignmentConflict = row.liveTrip

	newest := row.updatedAt
	if row.availabilityChanged != nil && row.availabilityChanged.After(newest) {
		newest = *row.availabilityChanged
	}
	view.SourceVersion = newest.UTC().UnixMilli()
	return view
}

// LookupProvider answers one provider authority question over HTTP.
func (h *AuthorityHandler) LookupProvider(c *gin.Context) {
	view, err := h.reader.LookupProvider(c.Request.Context(), ProviderQuery{
		ProviderID:          c.Param("providerId"),
		ExpectedServiceType: c.Query("serviceType"),
	})
	if errors.Is(err, ErrProviderNotFound) {
		// Same response for "no such provider" and "malformed identifier", so
		// the endpoint cannot be used to enumerate who exists.
		httpx.Abort(c, apperrors.NotFound("Provider not found.", nil))
		return
	}
	if err != nil {
		httpx.Abort(c, apperrors.Internal("Provider lookup failed.", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": view})
}
