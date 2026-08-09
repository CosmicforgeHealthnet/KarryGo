// Package authority exposes the private, authenticated read contract for the
// facts this service is authoritative about: vehicles and providers.
//
// It exists as its own package because it is a different kind of surface from
// the feature packages around it. Those serve the people who own the data —
// riders managing their own vehicles, admins reviewing them. This one serves
// other services, answers only in verdicts, and returns nothing that
// identifies a person or an asset in the physical world.
package authority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// The vehicle authority contract.
//
// This service owns vehicles. Other services must not read the `bikes` table,
// must not keep their own copy of what a vehicle is, and must not decide for
// themselves whether one may be used. They ask here.
//
// What the contract deliberately does NOT return: plate number, chassis
// number, engine capacity, brand/model/colour, document URLs, provider contact
// details or anything else identifying a person or an asset in the physical
// world. A fleet operator asking "may this vehicle work?" needs a set of
// verdicts, not a vehicle registration record. Everything below is either a
// state name, a boolean, or an opaque identifier the caller already holds.

// Authority states. These are contract values: fleet stores them in durable
// eligibility decisions, so renaming one is a breaking change.
const (
	VerificationStateVerified   = "VERIFIED"
	VerificationStatePending    = "PENDING"
	VerificationStateUnverified = "UNVERIFIED"
	VerificationStateRejected   = "REJECTED"
	VerificationStateSuspended  = "SUSPENDED"

	AvailabilityOnline  = "ONLINE"
	AvailabilityOffline = "OFFLINE"
	AvailabilityBusy    = "BUSY"
	AvailabilityUnknown = "UNKNOWN"

	RestrictionNone      = "NONE"
	RestrictionInactive  = "INACTIVE"
	RestrictionSuspended = "SUSPENDED"

	// ControlProvider says the vehicle is controlled by the provider named in
	// providerId. This service does not model organization ownership of
	// vehicles at all, and says so rather than inventing an answer: the caller
	// establishes the organization link from its own provider membership.
	ControlProvider = "PROVIDER_CONTROLLED"

	// ConflictBasisProviderLiveTrip states what an assignment conflict was
	// derived from. Dispatch trips do not name a vehicle, so the honest
	// granularity is the owning provider's live work. Saying which basis was
	// used stops the caller from reading more precision into the answer than
	// exists.
	ConflictBasisProviderLiveTrip = "PROVIDER_LIVE_TRIP"
)

// requiredDocumentTypes are the document types a dispatch vehicle must hold.
// Absence and expiry are reported separately: a missing insurance document and
// an expired one are different problems with different remedies.
var requiredDocumentTypes = []string{"registration", "insurance"}

// DocumentStatus is one required document's standing.
type DocumentStatus struct {
	Type      string     `json:"type"`
	Present   bool       `json:"present"`
	Expired   bool       `json:"expired"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// AuthorityView is the complete answer to "what is the authoritative state of
// this vehicle?".
type AuthorityView struct {
	VehicleID  string `json:"vehicleId"`
	ProviderID string `json:"providerId"`

	ControlState string `json:"controlState"`
	// ProviderMatch answers the ownership question the caller actually asked,
	// rather than making it re-derive one from providerId. It is only
	// meaningful when the caller supplied an expected provider.
	ProviderMatch *bool `json:"providerMatch,omitempty"`

	ServiceType string `json:"serviceType"`
	// ServiceTypeMatch is likewise only present when the caller stated one.
	ServiceTypeMatch *bool `json:"serviceTypeMatch,omitempty"`

	VerificationState string `json:"verificationState"`
	Available         bool   `json:"available"`
	AvailabilityState string `json:"availabilityState"`
	Restricted        bool   `json:"restricted"`
	RestrictionState  string `json:"restrictionState"`

	RequiredDocuments  []DocumentStatus `json:"requiredDocuments"`
	DocumentsSatisfied bool             `json:"documentsSatisfied"`

	ActiveAssignmentConflict bool   `json:"activeAssignmentConflict"`
	AssignmentConflictBasis  string `json:"assignmentConflictBasis"`

	// SourceVersion is the authoritative version of this answer, derived from
	// the most recent change to any row it was built from. A caller that has
	// cached an older answer can tell it is older.
	SourceVersion int64     `json:"sourceVersion"`
	ObservedAt    time.Time `json:"observedAt"`
}

// ErrVehicleNotFound means no such vehicle exists here.
var ErrVehicleNotFound = errors.New("vehicle: not found")

// AuthorityQuery is one authority lookup.
type AuthorityQuery struct {
	VehicleID           string
	ExpectedProviderID  string
	ExpectedServiceType string
}

// AuthorityReader reads the authoritative vehicle state.
type AuthorityReader struct {
	db *pgxpool.Pool
}

// NewAuthorityReader builds the reader.
func NewAuthorityReader(db *pgxpool.Pool) *AuthorityReader {
	return &AuthorityReader{db: db}
}

// authorityRow is the raw slice of authoritative state a lookup needs.
type authorityRow struct {
	id                  string
	providerID          string
	bikeType            string
	verificationStatus  string
	isActive            bool
	updatedAt           time.Time
	providerActive      bool
	providerVerified    string
	availabilityStatus  *string
	availabilityChanged *time.Time
	liveTrip            bool
}

// Lookup answers one authority question.
//
// Every field is read inside a single snapshot so the answer is internally
// consistent: a vehicle reported available and unconflicted must have been
// both at the same instant, not at two instants a few milliseconds apart.
func (r *AuthorityReader) Lookup(ctx context.Context, query AuthorityQuery) (*AuthorityView, error) {
	vehicleID := strings.TrimSpace(query.VehicleID)
	if vehicleID == "" {
		return nil, ErrVehicleNotFound
	}

	transaction, err := r.db.BeginTx(ctx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	var row authorityRow
	err = transaction.QueryRow(ctx, `
		SELECT b.id::text, b.provider_id::text, b.bike_type, b.verification_status,
		       b.is_active, b.updated_at,
		       p.is_active, p.verification_status,
		       a.status, a.last_changed_at,
		       EXISTS (
		           SELECT 1 FROM trips t
		           WHERE t.provider_id = b.provider_id
		             AND t.status NOT IN ('completed', 'cancelled', 'failed')
		       )
		FROM bikes b
		JOIN providers p ON p.id = b.provider_id
		LEFT JOIN provider_availability a ON a.provider_id = b.provider_id
		WHERE b.id = $1::uuid`, vehicleID).
		Scan(&row.id, &row.providerID, &row.bikeType, &row.verificationStatus,
			&row.isActive, &row.updatedAt, &row.providerActive, &row.providerVerified,
			&row.availabilityStatus, &row.availabilityChanged, &row.liveTrip)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrVehicleNotFound
	}
	if err != nil {
		// A malformed identifier is a lookup miss, not an internal error: the
		// caller supplied an opaque string and it does not name a vehicle.
		if strings.Contains(err.Error(), "invalid input syntax") {
			return nil, ErrVehicleNotFound
		}
		return nil, err
	}

	documents, err := r.readDocuments(ctx, transaction, vehicleID)
	if err != nil {
		return nil, err
	}

	return buildAuthorityView(row, documents, query), nil
}

// readDocuments reports the standing of each required document type.
func (r *AuthorityReader) readDocuments(ctx context.Context, q pgx.Tx,
	vehicleID string) (map[string]DocumentStatus, error) {
	rows, err := q.Query(ctx, `
		SELECT document_type, max(expiry_date)
		FROM bike_documents
		WHERE bike_id = $1::uuid
		GROUP BY document_type`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	held := map[string]DocumentStatus{}
	for rows.Next() {
		var documentType string
		var expiry *time.Time
		if scanErr := rows.Scan(&documentType, &expiry); scanErr != nil {
			return nil, scanErr
		}
		status := DocumentStatus{Type: documentType, Present: true}
		if expiry != nil {
			expiresAt := expiry.UTC()
			status.ExpiresAt = &expiresAt
			status.Expired = !expiresAt.After(time.Now().UTC())
		}
		held[documentType] = status
	}
	return held, rows.Err()
}

// buildAuthorityView turns raw state into the contract answer.
//
// Provider state is folded into the vehicle's answer on purpose. A vehicle
// belonging to a deactivated or unverified provider is not usable, and a
// caller that had to ask two questions and combine them itself would sooner or
// later ask only one.
func buildAuthorityView(row authorityRow, documents map[string]DocumentStatus,
	query AuthorityQuery) *AuthorityView {
	view := &AuthorityView{
		VehicleID:               row.id,
		ProviderID:              row.providerID,
		ControlState:            ControlProvider,
		ServiceType:             row.bikeType,
		VerificationState:       verificationStateFor(row),
		AssignmentConflictBasis: ConflictBasisProviderLiveTrip,
		ObservedAt:              time.Now().UTC(),
	}

	switch {
	case !row.isActive:
		view.Restricted = true
		view.RestrictionState = RestrictionInactive
	case row.verificationStatus == "suspended" || !row.providerActive:
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
	// Unknown availability is not availability. A provider whose availability
	// row has never been written has not gone online, and treating the absence
	// as "probably fine" is how an unverified rider ends up on a job.
	view.Available = view.AvailabilityState == AvailabilityOnline && !view.Restricted

	view.RequiredDocuments = make([]DocumentStatus, 0, len(requiredDocumentTypes))
	view.DocumentsSatisfied = true
	for _, documentType := range requiredDocumentTypes {
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

	if expected := strings.TrimSpace(query.ExpectedProviderID); expected != "" {
		match := expected == row.providerID
		view.ProviderMatch = &match
	}
	if expected := strings.TrimSpace(query.ExpectedServiceType); expected != "" {
		match := strings.EqualFold(expected, row.bikeType)
		view.ServiceTypeMatch = &match
	}

	view.SourceVersion = sourceVersionFor(row)
	return view
}

// verificationStateFor maps the stored status onto the contract vocabulary.
// A vehicle owned by an unverified provider is reported UNVERIFIED regardless
// of its own row: the vehicle cannot be more verified than whoever rides it.
func verificationStateFor(row authorityRow) string {
	providerVerified := row.providerVerified == "verified" || row.providerVerified == "fully_approved"
	switch row.verificationStatus {
	case "verified":
		if !providerVerified {
			return VerificationStateUnverified
		}
		return VerificationStateVerified
	case "pending":
		return VerificationStatePending
	case "rejected":
		return VerificationStateRejected
	case "suspended":
		return VerificationStateSuspended
	default:
		return VerificationStateUnverified
	}
}

// sourceVersionFor derives a monotonic version from the newest input row.
//
// Milliseconds rather than seconds because two updates inside the same second
// are ordinary, and a version that does not move is worse than no version: it
// makes a stale answer look current.
func sourceVersionFor(row authorityRow) int64 {
	newest := row.updatedAt
	if row.availabilityChanged != nil && row.availabilityChanged.After(newest) {
		newest = *row.availabilityChanged
	}
	return newest.UTC().UnixMilli()
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

// HeaderServiceNonce carries the single-use nonce for private calls.
const HeaderServiceNonce = "X-Service-Nonce"

// NonceClaimer records a single-use private-call nonce.
type NonceClaimer interface {
	Claim(ctx context.Context, serviceName, nonce, fingerprint string, expiresAt time.Time) error
}

// ErrNonceReplayed means this caller already used this nonce.
var ErrNonceReplayed = errors.New("vehicle: service nonce replayed")

// PostgresNonceClaimer stores nonces in the dispatch database.
type PostgresNonceClaimer struct {
	db *pgxpool.Pool
}

// NewPostgresNonceClaimer builds the claimer.
func NewPostgresNonceClaimer(db *pgxpool.Pool) *PostgresNonceClaimer {
	return &PostgresNonceClaimer{db: db}
}

// Claim records one nonce, failing when it was already used.
func (c *PostgresNonceClaimer) Claim(ctx context.Context, serviceName, nonce,
	fingerprint string, expiresAt time.Time) error {
	if serviceName == "" || nonce == "" {
		return ErrNonceReplayed
	}
	tag, err := c.db.Exec(ctx, `
		INSERT INTO dispatch_service_request_nonces
			(service_name, nonce, request_fingerprint, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, serviceName, nonce, fingerprint, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNonceReplayed
	}
	return nil
}

// AuthorityHandler serves the private vehicle authority routes.
type AuthorityHandler struct {
	reader *AuthorityReader
}

// NewAuthorityHandler builds the handler.
func NewAuthorityHandler(reader *AuthorityReader) *AuthorityHandler {
	return &AuthorityHandler{reader: reader}
}

// RegisterAuthorityRoutes mounts the private vehicle authority API.
//
// Three gates, in order, none optional:
//
//  1. serviceauth proves who signed the request and that the body and URI were
//     not altered, within a bounded timestamp window;
//  2. the caller allowlist decides whether that proven identity may reach THIS
//     surface — holding a valid signature for some unrelated integration must
//     not grant vehicle reads;
//  3. the nonce store rejects a replay of a still-fresh signature.
//
// There is no unauthenticated variant of any route here, and no flag that
// turns the gates off.
func RegisterAuthorityRoutes(group *gin.RouterGroup, handler *AuthorityHandler,
	verifier *serviceauth.Verifier, nonces NonceClaimer, allowedCallers []string) {
	group.Use(ServiceBoundary(verifier, nonces, allowedCallers)...)

	group.GET("/vehicles/:vehicleId", handler.LookupVehicle)
	group.GET("/providers/:providerId", handler.LookupProvider)
}

// ServiceBoundary returns the common HMAC-v2, caller-allowlist and nonce
// replay gates used by every Dispatch private service surface.
func ServiceBoundary(verifier *serviceauth.Verifier, nonces NonceClaimer,
	allowedCallers []string) []gin.HandlerFunc {
	permitted := make(map[string]bool, len(allowedCallers))
	for _, name := range allowedCallers {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			permitted[trimmed] = true
		}
	}

	return []gin.HandlerFunc{serviceauth.Middleware(verifier), func(c *gin.Context) {
		raw, _ := c.Get("service_name")
		serviceName, _ := raw.(string)
		if serviceName == "" || !permitted[serviceName] {
			httpx.Abort(c, apperrors.Forbidden("Service is not allowed to call this endpoint.", nil))
			return
		}

		nonce := strings.TrimSpace(c.GetHeader(HeaderServiceNonce))
		if nonce == "" {
			httpx.Abort(c, apperrors.Unauthorized("A request nonce is required.", nil))
			return
		}
		if nonces == nil {
			// No replay store means replay cannot be prevented. Refusing is the
			// only safe reading: a missing control must never read as a passed
			// one.
			httpx.Abort(c, apperrors.Unavailable("Replay protection is unavailable.", nil))
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
		sum := sha256.Sum256(append([]byte(c.Request.Method+"\x00"+
			c.Request.URL.RequestURI()+"\x00"), body...))
		if err := nonces.Claim(c.Request.Context(), serviceName, nonce,
			hex.EncodeToString(sum[:]), time.Now().UTC().Add(10*time.Minute)); err != nil {
			httpx.Abort(c, apperrors.New(http.StatusConflict, apperrors.Code("replayed_request"),
				"This request was already processed.", nil))
			return
		}
		c.Next()
	}}
}

// LookupVehicle answers one authority question.
func (h *AuthorityHandler) LookupVehicle(c *gin.Context) {
	view, err := h.reader.Lookup(c.Request.Context(), AuthorityQuery{
		VehicleID:           c.Param("vehicleId"),
		ExpectedProviderID:  c.Query("providerId"),
		ExpectedServiceType: c.Query("serviceType"),
	})
	if errors.Is(err, ErrVehicleNotFound) {
		// Identical response whether the vehicle does not exist or the caller
		// mistyped an identifier: a distinguishable answer would turn this
		// endpoint into a vehicle-existence oracle.
		httpx.Abort(c, apperrors.NotFound("Vehicle not found.", nil))
		return
	}
	if err != nil {
		httpx.Abort(c, apperrors.Internal("Vehicle lookup failed.", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": view})
}
